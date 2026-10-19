[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ExePath,

    [Parameter(Mandatory = $true)]
    [string]$IconPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Keep the PE update implementation local and dependency-free.  The resource
# APIs are intentionally called with bDeleteExistingResources=false so an
# existing executable keeps unrelated resources (manifest, version metadata,
# and so on).  A failed transaction is discarded explicitly.
if ($null -eq ("ProxyStart.NativeResourceWriter" -as [type])) {
    Add-Type -TypeDefinition @"
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;

namespace ProxyStart {
    public static class NativeResourceWriter {
        private const string Kernel32 = "kernel32.dll";

        [DllImport(Kernel32, CharSet = CharSet.Unicode, SetLastError = true)]
        private static extern IntPtr BeginUpdateResourceW(string pFileName, [MarshalAs(UnmanagedType.Bool)] bool bDeleteExistingResources);

        [DllImport(Kernel32, CharSet = CharSet.Unicode, SetLastError = true)]
        [return: MarshalAs(UnmanagedType.Bool)]
        private static extern bool UpdateResourceW(
            IntPtr hUpdate,
            IntPtr lpType,
            IntPtr lpName,
            ushort wLanguage,
            byte[] lpData,
            uint cbData);

        [DllImport(Kernel32, SetLastError = true)]
        [return: MarshalAs(UnmanagedType.Bool)]
        private static extern bool EndUpdateResourceW(
            IntPtr hUpdate,
            [MarshalAs(UnmanagedType.Bool)] bool fDiscard);

        public static IntPtr Begin(string path) => BeginUpdateResourceW(path, false);
        public static bool Update(IntPtr handle, ushort type, ushort name, byte[] data) =>
            UpdateResourceW(handle, MakeIntResource(type), MakeIntResource(name), 0, data, checked((uint)data.Length));
        public static bool Commit(IntPtr handle) => EndUpdateResourceW(handle, false);
        public static void Discard(IntPtr handle) { EndUpdateResourceW(handle, true); }
        public static int LastError() => Marshal.GetLastWin32Error();
        public static IntPtr MakeIntResource(ushort id) => new IntPtr(id);
    }
}
"@
}

$originalLocation = Get-Location
$resourceHandle = [IntPtr]::Zero
$resourceCommitted = $false

function Get-ResolvedLeafPath([string]$Path, [string]$Kind) {
    if ([string]::IsNullOrWhiteSpace($Path)) {
        throw "$Kind 路径不能为空。"
    }
    if ($Path.IndexOfAny([char[]]@('*', '?')) -ge 0) {
        throw "$Kind 路径不允许包含通配符：$Path"
    }
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "$Kind 文件不存在：$Path"
    }
    $item = Get-Item -LiteralPath $Path -Force
    if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "拒绝使用重解析点 $Kind：$($item.FullName)"
    }
    return $item.FullName
}

function Set-U16([byte[]]$Buffer, [int]$Offset, [int]$Value) {
    $number = [uint16]$Value
    $Buffer[$Offset] = [byte]($number -band 0xff)
    $Buffer[$Offset + 1] = [byte](($number -shr 8) -band 0xff)
}

function Set-U32([byte[]]$Buffer, [int]$Offset, [UInt64]$Value) {
    $number = [uint32]$Value
    $Buffer[$Offset] = [byte]($number -band 0xff)
    $Buffer[$Offset + 1] = [byte](($number -shr 8) -band 0xff)
    $Buffer[$Offset + 2] = [byte](($number -shr 16) -band 0xff)
    $Buffer[$Offset + 3] = [byte](($number -shr 24) -band 0xff)
}

function Get-U16([byte[]]$Buffer, [int]$Offset) {
    if ($Offset -lt 0 -or $Offset + 2 -gt $Buffer.Length) {
        throw "PE 字段超出文件范围（偏移 $Offset）。"
    }
    return [uint16]($Buffer[$Offset] -bor ($Buffer[$Offset + 1] -shl 8))
}

function Get-U32([byte[]]$Buffer, [int]$Offset) {
    if ($Offset -lt 0 -or $Offset + 4 -gt $Buffer.Length) {
        throw "PE 字段超出文件范围（偏移 $Offset）。"
    }
    $number = [uint32]$Buffer[$Offset]
    $number = $number -bor ([uint32]$Buffer[$Offset + 1] -shl 8)
    $number = $number -bor ([uint32]$Buffer[$Offset + 2] -shl 16)
    $number = $number -bor ([uint32]$Buffer[$Offset + 3] -shl 24)
    return [uint32]$number
}

function Test-ByteArrayEqual([byte[]]$Left, [byte[]]$Right) {
    if ($null -eq $Left -or $null -eq $Right -or $Left.Length -ne $Right.Length) {
        return $false
    }
    for ($index = 0; $index -lt $Left.Length; $index++) {
        if ($Left[$index] -ne $Right[$index]) {
            return $false
        }
    }
    return $true
}

function Read-PeLayout([string]$Path) {
    $bytes = [IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -lt 0x40 -or $bytes[0] -ne 0x4d -or $bytes[1] -ne 0x5a) {
        throw "目标不是有效的 DOS/PE 文件：$Path"
    }
    $peOffset = [int](Get-U32 $bytes 0x3c)
    if ($peOffset -lt 0 -or $peOffset + 24 -gt $bytes.Length) {
        throw "PE 头偏移越界：$Path"
    }
    if ($bytes[$peOffset] -ne 0x50 -or $bytes[$peOffset + 1] -ne 0x45 -or $bytes[$peOffset + 2] -ne 0 -or $bytes[$peOffset + 3] -ne 0) {
        throw "PE 签名无效：$Path"
    }

    $numberOfSections = [int](Get-U16 $bytes ($peOffset + 6))
    $sizeOfOptionalHeader = [int](Get-U16 $bytes ($peOffset + 20))
    $sectionTableOffset = $peOffset + 24 + $sizeOfOptionalHeader
    $sectionTableEnd = [int64]$sectionTableOffset + (40L * $numberOfSections)
    if ($numberOfSections -lt 1 -or $sectionTableEnd -gt $bytes.Length) {
        throw "PE 节表越界：$Path"
    }

    $sections = @()
    for ($index = 0; $index -lt $numberOfSections; $index++) {
        $offset = $sectionTableOffset + (40 * $index)
        $nameBytes = New-Object byte[] 8
        [Array]::Copy($bytes, $offset, $nameBytes, 0, 8)
        $virtualSize = [uint32](Get-U32 $bytes ($offset + 8))
        $virtualAddress = [uint32](Get-U32 $bytes ($offset + 12))
        $rawSize = [uint32](Get-U32 $bytes ($offset + 16))
        $rawPointer = [uint32](Get-U32 $bytes ($offset + 20))
        $characteristics = [uint32](Get-U32 $bytes ($offset + 36))
        if ($rawSize -gt 0 -and ([uint64]$rawPointer + [uint64]$rawSize -gt [uint64]$bytes.Length)) {
            throw "PE 节的原始数据越界（节 $index）：$Path"
        }
        $sections += [pscustomobject]@{
            Index = $index
            NameBytes = $nameBytes
            VirtualSize = $virtualSize
            VirtualAddress = $virtualAddress
            RawSize = $rawSize
            RawPointer = $rawPointer
            Characteristics = $characteristics
        }
    }

    return [pscustomobject]@{
        Bytes = $bytes
        PeOffset = $peOffset
        CoffPointerOffset = $peOffset + 12
        SymbolTablePointer = [uint32](Get-U32 $bytes ($peOffset + 12))
        NumberOfSymbols = [uint32](Get-U32 $bytes ($peOffset + 16))
        Sections = $sections
    }
}

function Get-PeSymbolAnchor([string]$Path) {
    $layout = Read-PeLayout $Path
    if ($layout.SymbolTablePointer -eq 0) {
        return [pscustomobject]@{ HasSymbolTable = $false }
    }

    $symbolPointer = [uint64]$layout.SymbolTablePointer
    $matches = @($layout.Sections | Where-Object {
        $_.RawSize -gt 0 -and
        $symbolPointer -ge [uint64]$_.RawPointer -and
        $symbolPointer -lt ([uint64]$_.RawPointer + [uint64]$_.RawSize)
    })
    if ($matches.Count -ne 1) {
        throw "COFF 符号表指针不在唯一节内（指针 0x$('{0:X8}' -f $layout.SymbolTablePointer)）：$Path"
    }
    $section = $matches[0]
    return [pscustomobject]@{
        HasSymbolTable = $true
        NameBytes = [byte[]]$section.NameBytes
        VirtualSize = [uint32]$section.VirtualSize
        RawSize = [uint32]$section.RawSize
        Characteristics = [uint32]$section.Characteristics
        OffsetWithinSection = [uint64]$symbolPointer - [uint64]$section.RawPointer
    }
}

function Repair-PeSymbolTablePointer([string]$Path, [pscustomobject]$Anchor) {
    if (-not $Anchor.HasSymbolTable) {
        return
    }
    $layout = Read-PeLayout $Path
    $matches = @($layout.Sections | Where-Object {
        $_.RawSize -eq $Anchor.RawSize -and
        $_.VirtualSize -eq $Anchor.VirtualSize -and
        $_.Characteristics -eq $Anchor.Characteristics -and
        (Test-ByteArrayEqual $_.NameBytes $Anchor.NameBytes)
    })
    if ($matches.Count -ne 1) {
        throw "注入后无法唯一定位原 COFF 符号表节（匹配数 $($matches.Count)）：$Path"
    }

    $section = $matches[0]
    $newPointer = [uint64]$section.RawPointer + [uint64]$Anchor.OffsetWithinSection
    if ($newPointer -gt [uint64][uint32]::MaxValue -or $newPointer -ge [uint64]$layout.Bytes.Length) {
        throw "注入后 COFF 符号表指针越界（0x$('{0:X}' -f $newPointer)）：$Path"
    }
    if ([uint32]$newPointer -eq $layout.SymbolTablePointer) {
        return
    }

    $stream = $null
    try {
        $stream = [IO.File]::Open($Path, [IO.FileMode]::Open, [IO.FileAccess]::Write, [IO.FileShare]::Read)
        $stream.Seek($layout.CoffPointerOffset, [IO.SeekOrigin]::Begin) | Out-Null
        $patch = [BitConverter]::GetBytes([uint32]$newPointer)
        $stream.Write($patch, 0, $patch.Length)
        $stream.Flush()
    }
    finally {
        if ($null -ne $stream) {
            $stream.Dispose()
        }
    }

    $verified = Read-PeLayout $Path
    if ([uint32]$newPointer -ne $verified.SymbolTablePointer) {
        throw "COFF 符号表指针修复后校验失败：$Path"
    }
    Write-Host ("已修复 COFF 符号表指针：0x{0:X8} -> 0x{1:X8}" -f $layout.SymbolTablePointer, $newPointer) -ForegroundColor DarkGray
}

try {
    $isWindowsHost = $env:OS -eq "Windows_NT"
    $isWindowsVariable = Get-Variable -Name IsWindows -ErrorAction SilentlyContinue
    if ($null -ne $isWindowsVariable) {
        $isWindowsHost = [bool]$isWindowsVariable.Value
    }
    if (-not $isWindowsHost) {
        throw "Windows PE 图标注入只能在 Windows 上执行。"
    }

    $exe = Get-ResolvedLeafPath $ExePath "EXE"
    $icon = Get-ResolvedLeafPath $IconPath "ICO"
    if ([IO.Path]::GetExtension($exe) -ine ".exe") {
        throw "目标文件必须是 .exe：$exe"
    }
    if ([IO.Path]::GetExtension($icon) -ine ".ico") {
        throw "图标资源必须是 .ico：$icon"
    }
    if ([IO.Path]::GetFullPath($exe) -ieq [IO.Path]::GetFullPath($icon)) {
        throw "EXE 与 ICO 不能是同一个文件。"
    }

    $icoBytes = [IO.File]::ReadAllBytes($icon)
    if ($icoBytes.Length -lt 6) {
        throw "ICO 文件过短：$icon"
    }
    $reserved = [BitConverter]::ToUInt16($icoBytes, 0)
    $type = [BitConverter]::ToUInt16($icoBytes, 2)
    $count = [BitConverter]::ToUInt16($icoBytes, 4)
    if ($reserved -ne 0 -or $type -ne 1 -or $count -lt 1) {
        throw "ICO 头无效（需要 reserved=0、type=1 且至少包含一个图像）：$icon"
    }
    $entryTableBytes = 6L + (16L * $count)
    if ($entryTableBytes -gt $icoBytes.Length) {
        throw "ICO 图像目录被截断：$icon"
    }

    $entries = @()
    for ($index = 0; $index -lt $count; $index++) {
        $entryOffset = 6 + (16 * $index)
        $width = [int]$icoBytes[$entryOffset]
        $height = [int]$icoBytes[$entryOffset + 1]
        $colorCount = [int]$icoBytes[$entryOffset + 2]
        $planes = [BitConverter]::ToUInt16($icoBytes, $entryOffset + 4)
        $bitCount = [BitConverter]::ToUInt16($icoBytes, $entryOffset + 6)
        $bytesInResource = [uint64][BitConverter]::ToUInt32($icoBytes, $entryOffset + 8)
        $imageOffset = [uint64][BitConverter]::ToUInt32($icoBytes, $entryOffset + 12)
        if ($bytesInResource -lt 1 -or $imageOffset -gt [uint64]$icoBytes.Length -or $bytesInResource -gt ([uint64]$icoBytes.Length - $imageOffset)) {
            throw "ICO 图像目录项 $($index + 1) 越界：$icon"
        }
        if ($bytesInResource -gt [uint64][uint32]::MaxValue) {
            throw "ICO 图像目录项 $($index + 1) 过大：$icon"
        }
        $entries += [pscustomobject]@{
            Width = $width
            Height = $height
            ColorCount = $colorCount
            Planes = $planes
            BitCount = $bitCount
            BytesInResource = $bytesInResource
            ImageOffset = $imageOffset
        }
    }

    # BeginUpdateResourceW may insert a .rsrc section before the existing
    # relocation/symbol sections. Capture the COFF symbol table's section and
    # its in-section offset before that happens; the resource API can leave
    # PointerToSymbolTable at its old absolute file offset.
    $symbolAnchor = Get-PeSymbolAnchor $exe

    # GRPICONDIR/GRPICONDIRENTRY is the ICO directory with a 16-bit resource
    # identifier in place of each image's file offset.
    $groupBytes = New-Object byte[] (6 + (14 * $entries.Count))
    Set-U16 $groupBytes 0 0
    Set-U16 $groupBytes 2 1
    Set-U16 $groupBytes 4 $entries.Count
    for ($index = 0; $index -lt $entries.Count; $index++) {
        $entry = $entries[$index]
        $entryOffset = 6 + (14 * $index)
        $groupBytes[$entryOffset] = [byte]$entry.Width
        $groupBytes[$entryOffset + 1] = [byte]$entry.Height
        $groupBytes[$entryOffset + 2] = [byte]$entry.ColorCount
        $groupBytes[$entryOffset + 3] = 0
        Set-U16 $groupBytes ($entryOffset + 4) $entry.Planes
        Set-U16 $groupBytes ($entryOffset + 6) $entry.BitCount
        Set-U32 $groupBytes ($entryOffset + 8) $entry.BytesInResource
        Set-U16 $groupBytes ($entryOffset + 12) ($index + 1)
    }

    $resourceHandle = [ProxyStart.NativeResourceWriter]::Begin($exe)
    if ($resourceHandle -eq [IntPtr]::Zero) {
        throw "BeginUpdateResourceW 失败（Win32 $([ProxyStart.NativeResourceWriter]::LastError())）：$exe"
    }

    for ($index = 0; $index -lt $entries.Count; $index++) {
        $entry = $entries[$index]
        $imageLength = [int]$entry.BytesInResource
        $image = New-Object byte[] $imageLength
        [Array]::Copy($icoBytes, [int]$entry.ImageOffset, $image, 0, $imageLength)
        if (-not [ProxyStart.NativeResourceWriter]::Update($resourceHandle, 3, [uint16]($index + 1), $image)) {
            throw "UpdateResourceW RT_ICON #$($index + 1) 失败（Win32 $([ProxyStart.NativeResourceWriter]::LastError())）。"
        }
    }
    if (-not [ProxyStart.NativeResourceWriter]::Update($resourceHandle, 14, 1, $groupBytes)) {
        throw "UpdateResourceW RT_GROUP_ICON 失败（Win32 $([ProxyStart.NativeResourceWriter]::LastError())）。"
    }
    if (-not [ProxyStart.NativeResourceWriter]::Commit($resourceHandle)) {
        $errorCode = [ProxyStart.NativeResourceWriter]::LastError()
        [ProxyStart.NativeResourceWriter]::Discard($resourceHandle)
        $resourceHandle = [IntPtr]::Zero
        throw "EndUpdateResourceW 提交失败（Win32 $errorCode）：$exe"
    }
    $resourceHandle = [IntPtr]::Zero
    $resourceCommitted = $true
    Repair-PeSymbolTablePointer $exe $symbolAnchor
    Write-Host "已嵌入 Windows 图标：$exe（$($entries.Count) 个尺寸）" -ForegroundColor Green
}
catch {
    if ($resourceHandle -ne [IntPtr]::Zero) {
        [ProxyStart.NativeResourceWriter]::Discard($resourceHandle)
        $resourceHandle = [IntPtr]::Zero
    }
    throw
}
finally {
    if ($resourceHandle -ne [IntPtr]::Zero -and -not $resourceCommitted) {
        [ProxyStart.NativeResourceWriter]::Discard($resourceHandle)
    }
    Set-Location -LiteralPath $originalLocation
}
