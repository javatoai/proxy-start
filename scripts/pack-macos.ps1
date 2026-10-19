[CmdletBinding()]
param(
    [string]$OutputDir = "",
    [string]$Version = "",
    [switch]$SkipTests,
    [switch]$KeepSymbols,
    [switch]$Force,
    [string]$GoExe = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# Build the native Go executable directly and create the small .app bundle
# MyGO expects. This can run on Windows because the app uses no cgo code;
# signing, notarization and DMG creation remain macOS-only operations.
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ([string]::IsNullOrWhiteSpace($Version)) {
    $versionFile = Join-Path $projectRoot "VERSION"
    if (-not (Test-Path -LiteralPath $versionFile -PathType Leaf)) {
        throw "版本文件不存在：$versionFile"
    }
    $Version = (Get-Content -LiteralPath $versionFile -Raw).Trim()
}
if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+$') {
    throw "Version 必须是三段数字版本（例如 0.1.8）。"
}
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $projectRoot "dist/release-$Version-macos"
}
elseif (-not [System.IO.Path]::IsPathRooted($OutputDir)) {
    $OutputDir = Join-Path $projectRoot $OutputDir
}
$OutputDir = [System.IO.Path]::GetFullPath($OutputDir)
$outputRoot = $OutputDir.TrimEnd([char[]]@('\', '/'))
if ($outputRoot.Length -eq 2 -and $outputRoot[1] -eq ':') { $outputRoot += '\' }
function Normalize-Path([string]$Path) {
    $full = [System.IO.Path]::GetFullPath($Path)
    $volumeRoot = [System.IO.Path]::GetPathRoot($full)
    # Keep Unix / and Windows volume roots intact while walking parents.
    # Trimming / to an empty string makes Directory.GetParent reject it.
    if ($full -eq $volumeRoot) { return $volumeRoot }
    return $full.TrimEnd([char[]]@('\', '/'))
}
function Assert-NoReparsePointInPath([string]$Path, [string]$Root) {
    $current = Normalize-Path $Path
    $rootPath = Normalize-Path $Root
    $rootExists = Test-Path -LiteralPath $rootPath
    while ($true) {
        if (Test-Path -LiteralPath $current) {
            $item = Get-Item -LiteralPath $current -Force
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "拒绝操作重解析点路径：$current"
            }
        }
        if ($current -ieq $rootPath -and $rootExists) { break }
        $parent = [IO.Directory]::GetParent($current)
        if ($null -eq $parent -or $parent.FullName -ieq $current) { break }
        $current = Normalize-Path $parent.FullName
    }
}
function Assert-TreeNoReparsePoints([string]$Directory) {
    if (-not (Test-Path -LiteralPath $Directory -PathType Container)) { return }
    foreach ($entry in Get-ChildItem -LiteralPath $Directory -Force) {
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "拒绝递归删除含重解析点的目录：$($entry.FullName)"
        }
        if ($entry.PSIsContainer) { Assert-TreeNoReparsePoints $entry.FullName }
    }
}
function Assert-SafeDeleteTarget([string]$Target, [string]$Root) {
    $targetPath = Normalize-Path $Target
    $rootPath = Normalize-Path $Root
    $rootPrefix = if ($rootPath.EndsWith('\') -or $rootPath.EndsWith('/')) { $rootPath } else { $rootPath + [IO.Path]::DirectorySeparatorChar }
    if ($targetPath -ieq $rootPath -or -not $targetPath.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "拒绝删除输出目录之外或输出目录本身的路径：$targetPath"
    }
    Assert-NoReparsePointInPath $targetPath $rootPath
}
Assert-NoReparsePointInPath $outputRoot $outputRoot
$projectPath = Normalize-Path $projectRoot
$projectPrefix = if ($outputRoot.EndsWith('\') -or $outputRoot.EndsWith('/')) { $outputRoot } else { $outputRoot + [IO.Path]::DirectorySeparatorChar }
if ($outputRoot.Length -le 3 -or $outputRoot -ieq $projectPath -or $projectPath.StartsWith($projectPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "OutputDir 不能是盘根、项目根目录或项目父目录：$outputRoot"
}
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null

$goExeResolved = $null
if (-not [string]::IsNullOrWhiteSpace($GoExe)) {
    $goExeResolved = (Resolve-Path -LiteralPath $GoExe -ErrorAction Stop).Path
}
else {
    $bundledGo = Join-Path $projectRoot ".tools/go/bin/go.exe"
    if (Test-Path -LiteralPath $bundledGo -PathType Leaf) {
        $goExeResolved = (Resolve-Path -LiteralPath $bundledGo).Path
    }
    else {
        $goCommand = Get-Command go -ErrorAction SilentlyContinue
        if ($null -eq $goCommand) { throw "找不到 Go；请使用 -GoExe 指定 Go 1.27.x，或准备项目内 .tools/go。" }
        $goExeResolved = $goCommand.Source
    }
}
if (-not (Test-Path -LiteralPath $goExeResolved -PathType Leaf)) { throw "Go 可执行文件不存在：$goExeResolved" }

function Invoke-Go([string[]]$Arguments) {
    Write-Host ("> go " + ($Arguments -join " ")) -ForegroundColor DarkGray
    & $goExeResolved @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Go 命令失败：go $($Arguments -join ' ')" }
}

function Write-U32BE([byte[]]$Buffer, [int]$Offset, [int]$Value) {
    $Buffer[$Offset] = [byte](($Value -shr 24) -band 0xff)
    $Buffer[$Offset + 1] = [byte](($Value -shr 16) -band 0xff)
    $Buffer[$Offset + 2] = [byte](($Value -shr 8) -band 0xff)
    $Buffer[$Offset + 3] = [byte]($Value -band 0xff)
}

function New-IcnsFromPngs([string[]]$PngPaths, [string]$Destination) {
    $chunkNames = @("icp4", "icp5", "icp6", "ic07", "ic08", "ic09", "ic10")
    $chunks = @()
    for ($i = 0; $i -lt $PngPaths.Count; $i++) {
        if (-not (Test-Path -LiteralPath $PngPaths[$i] -PathType Leaf)) { continue }
        $data = [IO.File]::ReadAllBytes($PngPaths[$i])
        if ($data.Length -lt 8 -or $data[0] -ne 0x89 -or $data[1] -ne 0x50 -or $data[2] -ne 0x4e -or $data[3] -ne 0x47) {
            throw "图标不是有效 PNG：$($PngPaths[$i])"
        }
        $chunks += @{ Name = $chunkNames[$i]; Data = $data }
    }
    if ($chunks.Count -eq 0) { throw "没有可用于 macOS 图标的 PNG。" }
    $total = 8
    foreach ($chunk in $chunks) { $total += 8 + $chunk.Data.Length }
    $result = New-Object byte[] $total
    [Array]::Copy([Text.Encoding]::ASCII.GetBytes("icns"), 0, $result, 0, 4)
    Write-U32BE $result 4 $total
    $offset = 8
    foreach ($chunk in $chunks) {
        [Array]::Copy([Text.Encoding]::ASCII.GetBytes($chunk.Name), 0, $result, $offset, 4)
        Write-U32BE $result ($offset + 4) (8 + $chunk.Data.Length)
        [Array]::Copy($chunk.Data, 0, $result, $offset + 8, $chunk.Data.Length)
        $offset += 8 + $chunk.Data.Length
    }
    [IO.File]::WriteAllBytes($Destination, $result)
}

function Write-InfoPlist([string]$Path) {
    $escapedName = [System.Security.SecurityElement]::Escape("Proxy Start")
    $escapedVersion = [System.Security.SecurityElement]::Escape($Version)
    $plist = @"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDisplayName</key><string>$escapedName</string>
    <key>CFBundleName</key><string>$escapedName</string>
    <key>CFBundleIdentifier</key><string>com.proxystart.desktop</string>
    <key>CFBundleExecutable</key><string>proxy-start</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
    <key>CFBundleVersion</key><string>$escapedVersion</string>
    <key>CFBundleShortVersionString</key><string>$escapedVersion</string>
    <key>CFBundleIconFile</key><string>proxy-start.icns</string>
    <key>LSMinimumSystemVersion</key><string>12.0</string>
    <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
"@
    [IO.File]::WriteAllText($Path, $plist.TrimStart(), [Text.UTF8Encoding]::new($false))
}

function Get-RelativeUnixPath([string]$Root, [string]$Path) {
    $relative = $Path.Substring($Root.Length)
    while ($relative.StartsWith('\') -or $relative.StartsWith('/')) {
        $relative = $relative.Substring(1)
    }
    return $relative.Replace('\', '/')
}

function Write-TreeChecksums([string]$Root, [string]$Path) {
    $lines = @(
        Get-ChildItem -LiteralPath $Root -Recurse -File -Force |
            Where-Object { $_.FullName -ne $Path } |
            Sort-Object FullName |
            ForEach-Object {
                $relative = Get-RelativeUnixPath $Root $_.FullName
                $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
                "$hash  $relative"
            }
    )
    [IO.File]::WriteAllLines($Path, $lines, [Text.Encoding]::ASCII)
}

function Write-ZipWithUnixModes([string]$Root, [string]$ZipPath) {
    $python = Get-Command python -ErrorAction SilentlyContinue
    if ($null -ne $python) {
        $zipper = @'
import os, sys, zipfile
root, output = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(output, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
    dirs = []
    for base, names, files in os.walk(root):
        for name in files:
            path = os.path.join(base, name)
            rel = os.path.relpath(path, root).replace(os.sep, "/")
            info = zipfile.ZipInfo.from_file(path, rel)
            info.create_system = 3
            mode = 0o100755 if rel.endswith("/Contents/MacOS/proxy-start") else 0o100644
            info.external_attr = mode << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            with open(path, "rb") as stream:
                archive.writestr(info, stream.read())
        for name in names:
            dirs.append(os.path.join(base, name))
    for path in sorted(dirs, key=lambda p: os.path.relpath(p, root)):
        rel = os.path.relpath(path, root).replace(os.sep, "/") + "/"
        info = zipfile.ZipInfo(rel)
        info.create_system = 3
        info.external_attr = (0o40755 << 16) | 0x10
        archive.writestr(info, b"")
'@
        & $python.Source -c $zipper $Root $ZipPath
        if ($LASTEXITCODE -eq 0) { return }
        Write-Warning "Python 压缩失败，将使用 .NET ZipArchive 回退；请检查解压后的执行权限。"
    }
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $stream = [IO.File]::Open($ZipPath, [IO.FileMode]::Create, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    try {
        $archive = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Create, $false)
        try {
            foreach ($directory in (Get-ChildItem -LiteralPath $Root -Recurse -Directory -Force | Sort-Object FullName)) {
                $entry = $archive.CreateEntry((Get-RelativeUnixPath $Root $directory.FullName) + "/")
                $entry.ExternalAttributes = (16877 -shl 16) -bor 0x10
            }
            foreach ($file in (Get-ChildItem -LiteralPath $Root -Recurse -File -Force | Sort-Object FullName)) {
                $entryName = Get-RelativeUnixPath $Root $file.FullName
                $entry = $archive.CreateEntry($entryName, [IO.Compression.CompressionLevel]::Optimal)
                $mode = if ($entryName.EndsWith("/Contents/MacOS/proxy-start")) { 33261 } else { 33188 }
                $entry.ExternalAttributes = $mode -shl 16
                $source = [IO.File]::OpenRead($file.FullName)
                $target = $entry.Open()
                try { $source.CopyTo($target) } finally { $target.Dispose(); $source.Dispose() }
            }
        } finally { $archive.Dispose() }
    } finally { $stream.Dispose() }
    Set-ZipMadeByUnix $ZipPath
}

function Set-ZipMadeByUnix([string]$ZipPath) {
    # ZipArchive on Windows writes a DOS host in the central directory even
    # when ExternalAttributes carries Unix mode bits. Patch only the central
    # directory's "version made by" host byte to Unix (3).
    $data = [IO.File]::ReadAllBytes($ZipPath)
    $offset = 0
    $found = 0
    while ($offset + 46 -le $data.Length) {
        if ($data[$offset] -eq 0x50 -and $data[$offset + 1] -eq 0x4b -and $data[$offset + 2] -eq 0x01 -and $data[$offset + 3] -eq 0x02) {
            $data[$offset + 5] = 3
            $nameLength = [BitConverter]::ToUInt16($data, $offset + 28)
            $extraLength = [BitConverter]::ToUInt16($data, $offset + 30)
            $commentLength = [BitConverter]::ToUInt16($data, $offset + 32)
            $offset += 46 + $nameLength + $extraLength + $commentLength
            $found++
            continue
        }
        $offset++
    }
    if ($found -eq 0) { throw "ZIP 没有可修补的 central directory：$ZipPath" }
    [IO.File]::WriteAllBytes($ZipPath, $data)
}

function Assert-ZipUnixModes([string]$ZipPath) {
    Add-Type -AssemblyName System.IO.Compression
    $stream = [IO.File]::OpenRead($ZipPath)
    try {
        $archive = [IO.Compression.ZipArchive]::new($stream, [IO.Compression.ZipArchiveMode]::Read, $false)
        try {
            foreach ($entry in $archive.Entries) {
                if ($entry.FullName.Contains('\')) { throw "ZIP 中出现 Windows 反斜杠路径：$($entry.FullName)" }
            }
            $expected = @{
                "Proxy Start.app/Contents/MacOS/proxy-start" = [uint32]([int64]33261 * 65536)
                "Proxy Start.app/Contents/Info.plist" = [uint32]([int64]33188 * 65536)
            }
            foreach ($name in $expected.Keys) {
                $entry = $archive.GetEntry($name)
                if ($null -eq $entry) { throw "ZIP 缺少预期文件：$name" }
                $actual = [BitConverter]::ToUInt32([BitConverter]::GetBytes([int32]$entry.ExternalAttributes), 0)
                if ($actual -ne $expected[$name]) {
                    $actualMode = [Convert]::ToString([int64]($actual -shr 16), 8)
                    $expectedMode = [Convert]::ToString([int64]($expected[$name] -shr 16), 8)
                    throw "ZIP Unix 权限错误：$name（实际 $actualMode，预期 $expectedMode）"
                }
            }
            Assert-ZipMadeByUnix $ZipPath
        } finally { $archive.Dispose() }
    } finally { $stream.Dispose() }
}

function Assert-ZipMadeByUnix([string]$ZipPath) {
    $data = [IO.File]::ReadAllBytes($ZipPath)
    $offset = 0
    $found = 0
    while ($offset + 46 -le $data.Length) {
        if ($data[$offset] -eq 0x50 -and $data[$offset + 1] -eq 0x4b -and $data[$offset + 2] -eq 0x01 -and $data[$offset + 3] -eq 0x02) {
            if ($data[$offset + 5] -ne 3) { throw "ZIP central directory 未声明 Unix host：$ZipPath" }
            $nameLength = [BitConverter]::ToUInt16($data, $offset + 28)
            $extraLength = [BitConverter]::ToUInt16($data, $offset + 30)
            $commentLength = [BitConverter]::ToUInt16($data, $offset + 32)
            $offset += 46 + $nameLength + $extraLength + $commentLength
            $found++
            continue
        }
        $offset++
    }
    if ($found -eq 0) { throw "ZIP 没有可验证的 central directory：$ZipPath" }
}

function Read-U32LE([byte[]]$Data, [int]$Offset) {
    return [BitConverter]::ToUInt32($Data, $Offset)
}
function Assert-MachOExecutable([string]$Path, [string]$Architecture) {
    $data = [IO.File]::ReadAllBytes($Path)
    if ($data.Length -lt 16) { throw "交叉编译产物过短，无法读取 Mach-O 头：$Path" }
    $magic = Read-U32LE $data 0
    if ($magic -ne [uint32]4277009103) { throw "产物不是 64 位 Mach-O（magic=0x$('{0:x8}' -f $magic)）：$Path" }
    $cpuType = Read-U32LE $data 4
    $fileType = Read-U32LE $data 12
    $expectedCPU = if ($Architecture -eq "arm64") { [uint32]0x0100000c } else { [uint32]0x01000007 }
    if ($cpuType -ne $expectedCPU) { throw "Mach-O CPU 类型不匹配：$Architecture（0x$('{0:x8}' -f $cpuType)）" }
    if ($fileType -ne 2) { throw "Mach-O 文件类型不是 MH_EXECUTE：$fileType" }
}

$environmentNames = @("GOROOT", "GO111MODULE", "GOTOOLCHAIN", "GOWORK", "GOOS", "GOARCH", "CGO_ENABLED")
$originalEnvironment = @{}
foreach ($name in $environmentNames) { $originalEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process") }
$oldLocation = Get-Location
try {
    Push-Location -LiteralPath $projectRoot
    $goRootCandidate = Split-Path (Split-Path $goExeResolved -Parent) -Parent
    if (Test-Path -LiteralPath (Join-Path $goRootCandidate "src/go.mod") -PathType Leaf) {
        $env:GOROOT = $goRootCandidate
    }
    $goVersion = (& $goExeResolved version).Trim()
    if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch "go1\.27\.") { throw "Go 版本不匹配：$goVersion；需要 Go 1.27.x。" }
    $env:GO111MODULE = "on"
    $env:GOTOOLCHAIN = "local"
    $env:GOWORK = "off"
    if (-not $SkipTests) {
        Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
        Invoke-Go @("test", "./...")
    }
    $iconSource = Join-Path $projectRoot "assets/proxy-start-64.png"
    $iconPaths = @(16, 32, 64, 128, 256, 512, 1024 | ForEach-Object { Join-Path $projectRoot "assets/proxy-start-$_.png" })
    if (-not (Test-Path -LiteralPath $iconSource -PathType Leaf)) { throw "缺少图标资源：$iconSource" }

    $releaseChecksums = Join-Path $OutputDir "checksums.sha256"
    foreach ($arch in @("arm64", "amd64")) {
        $targetRoot = Join-Path $OutputDir "ProxyStart-macos-$arch"
        $targetZip = Join-Path $OutputDir "ProxyStart-macos-$arch.zip"
        foreach ($target in @($targetRoot, $targetZip)) {
            if (-not (Test-Path -LiteralPath $target)) { continue }
            if (-not $Force) { throw "发布目标已存在：$target；如需覆盖请显式使用 -Force。" }
            Assert-SafeDeleteTarget $target $outputRoot
            if ((Get-Item -LiteralPath $target -Force).PSIsContainer) { Assert-TreeNoReparsePoints $target }
            Remove-Item -LiteralPath $target -Recurse -Force
        }
    }
    if (Test-Path -LiteralPath $releaseChecksums) {
        if (-not $Force) { throw "发布校验文件已存在：$releaseChecksums；如需覆盖请显式使用 -Force。" }
        Assert-SafeDeleteTarget $releaseChecksums $outputRoot
        Remove-Item -LiteralPath $releaseChecksums -Force
    }

    foreach ($arch in @("arm64", "amd64")) {
        $packageName = "ProxyStart-macos-$arch"
        $packageRoot = Join-Path $OutputDir $packageName
        $appRoot = Join-Path $packageRoot "Proxy Start.app"
        $contents = Join-Path $appRoot "Contents"
        $macOS = Join-Path $contents "MacOS"
        $resources = Join-Path $contents "Resources"
        New-Item -ItemType Directory -Force -Path $macOS, $resources | Out-Null
        $env:GOOS = "darwin"
        $env:GOARCH = $arch
        $env:CGO_ENABLED = "0"
        $binary = Join-Path $macOS "proxy-start"
        $args = @("build", "-trimpath", "-tags", "mygo_noinspector")
        if (-not $KeepSymbols) { $args += @("-ldflags", "-s -w") }
        $args += @("-o", $binary, "./cmd/proxy-start")
        Invoke-Go $args
        Assert-MachOExecutable $binary $arch
        Copy-Item -LiteralPath $iconSource -Destination (Join-Path $resources "proxy-start.png") -Force
        New-IcnsFromPngs $iconPaths (Join-Path $resources "proxy-start.icns")
        Write-InfoPlist (Join-Path $contents "Info.plist")
        [IO.File]::WriteAllText((Join-Path $contents "PkgInfo"), "APPL????", [Text.UTF8Encoding]::new($false))
        Copy-Item -LiteralPath (Join-Path $projectRoot "docs/MACOS.md") -Destination (Join-Path $packageRoot "MACOS.md") -Force
        Write-TreeChecksums $packageRoot (Join-Path $packageRoot "checksums.sha256")
        $zipPath = Join-Path $OutputDir "$packageName.zip"
        Write-ZipWithUnixModes $packageRoot $zipPath
        Assert-ZipUnixModes $zipPath
        Write-Host "已生成 macOS $arch 包：$zipPath" -ForegroundColor Green
    }
    $releaseChecksums = Join-Path $OutputDir "checksums.sha256"
    $releaseLines = foreach ($zip in (Get-ChildItem -LiteralPath $OutputDir -Filter "*.zip" -File | Sort-Object Name)) {
        "$((Get-FileHash -Algorithm SHA256 -LiteralPath $zip.FullName).Hash.ToLowerInvariant())  $($zip.Name)"
    }
    [IO.File]::WriteAllLines($releaseChecksums, $releaseLines, [Text.Encoding]::ASCII)
    Write-Host "macOS 发布校验文件：$releaseChecksums" -ForegroundColor Green
    Write-Host "说明：本包未签名、未公证，且未在本机 macOS 实机验证。" -ForegroundColor Yellow
}
finally {
    foreach ($name in $environmentNames) {
        $value = $originalEnvironment[$name]
        if ($null -eq $value) { Remove-Item -LiteralPath ("Env:" + $name) -ErrorAction SilentlyContinue }
        else { [Environment]::SetEnvironmentVariable($name, $value, "Process") }
    }
    Set-Location -LiteralPath $oldLocation
}
