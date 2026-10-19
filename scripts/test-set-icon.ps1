[CmdletBinding()]
param(
    [string]$GoExe = "",
    [string]$OutputDir = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$iconScript = Join-Path $PSScriptRoot "set-icon.ps1"
$iconPath = Join-Path $projectRoot "assets\proxy-start.ico"
$bundledGo = Join-Path $projectRoot ".tools\go\bin\go.exe"

if ([string]::IsNullOrWhiteSpace($GoExe)) {
    if (Test-Path -LiteralPath $bundledGo -PathType Leaf) {
        $GoExe = (Resolve-Path -LiteralPath $bundledGo).Path
    }
    else {
        $goCommand = Get-Command go -ErrorAction SilentlyContinue
        if ($null -eq $goCommand) {
            throw "找不到 Go；请使用 -GoExe 指定 go.exe。"
        }
        $GoExe = $goCommand.Source
    }
}
elseif (Test-Path -LiteralPath $GoExe -PathType Leaf) {
    $GoExe = (Resolve-Path -LiteralPath $GoExe).Path
}
else {
    $goCommand = Get-Command $GoExe -ErrorAction SilentlyContinue
    if ($null -eq $goCommand) {
        throw "指定的 Go 可执行文件不存在：$GoExe"
    }
    $GoExe = $goCommand.Source
}

foreach ($path in @($iconScript, $iconPath, $GoExe)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "缺少测试依赖：$path"
    }
}

function Get-U16([byte[]]$Bytes, [int]$Offset) {
    return [uint16]($Bytes[$Offset] -bor ($Bytes[$Offset + 1] -shl 8))
}

function Get-U32([byte[]]$Bytes, [int]$Offset) {
    $number = [uint32]$Bytes[$Offset]
    $number = $number -bor ([uint32]$Bytes[$Offset + 1] -shl 8)
    $number = $number -bor ([uint32]$Bytes[$Offset + 2] -shl 16)
    $number = $number -bor ([uint32]$Bytes[$Offset + 3] -shl 24)
    return [uint32]$number
}

function Set-U32([byte[]]$Bytes, [int]$Offset, [uint64]$Value) {
    $number = [uint32]$Value
    $Bytes[$Offset] = [byte]($number -band 0xff)
    $Bytes[$Offset + 1] = [byte](($number -shr 8) -band 0xff)
    $Bytes[$Offset + 2] = [byte](($number -shr 16) -band 0xff)
    $Bytes[$Offset + 3] = [byte](($number -shr 24) -band 0xff)
}

function Test-BytesEqual([byte[]]$Left, [byte[]]$Right) {
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
        throw "不是有效 PE 文件：$Path"
    }
    $peOffset = [int](Get-U32 $bytes 0x3c)
    if ($bytes[$peOffset] -ne 0x50 -or $bytes[$peOffset + 1] -ne 0x45 -or $bytes[$peOffset + 2] -ne 0 -or $bytes[$peOffset + 3] -ne 0) {
        throw "PE 签名无效：$Path"
    }
    $sectionCount = [int](Get-U16 $bytes ($peOffset + 6))
    $optionalSize = [int](Get-U16 $bytes ($peOffset + 20))
    $sectionTable = $peOffset + 24 + $optionalSize
    $sections = @()
    for ($index = 0; $index -lt $sectionCount; $index++) {
        $header = $sectionTable + (40 * $index)
        $nameBytes = New-Object byte[] 8
        [Array]::Copy($bytes, $header, $nameBytes, 0, 8)
        $sections += [pscustomobject]@{
            Index = $index
            HeaderOffset = $header
            NameBytes = $nameBytes
            VirtualSize = [uint32](Get-U32 $bytes ($header + 8))
            RawSize = [uint32](Get-U32 $bytes ($header + 16))
            RawPointer = [uint32](Get-U32 $bytes ($header + 20))
            Characteristics = [uint32](Get-U32 $bytes ($header + 36))
        }
    }
    return [pscustomobject]@{
        Bytes = $bytes
        PeOffset = $peOffset
        PointerOffset = $peOffset + 12
        SymbolPointer = [uint32](Get-U32 $bytes ($peOffset + 12))
        Sections = $sections
    }
}

function Get-SymbolInfo([string]$Path) {
    $layout = Read-PeLayout $Path
    if ($layout.SymbolPointer -eq 0) {
        throw "测试 fixture 没有 COFF 符号表指针：$Path"
    }
    $pointer = [uint64]$layout.SymbolPointer
    $matches = @($layout.Sections | Where-Object {
        $_.RawSize -gt 0 -and
        $pointer -ge [uint64]$_.RawPointer -and
        $pointer -lt ([uint64]$_.RawPointer + [uint64]$_.RawSize)
    })
    if ($matches.Count -ne 1) {
        throw "fixture 的 COFF 符号表不在唯一节内：$Path"
    }
    $section = $matches[0]
    return [pscustomobject]@{
        Layout = $layout
        Section = $section
        Offset = $pointer - [uint64]$section.RawPointer
    }
}

function Get-SectionBytes([string]$Path, [pscustomobject]$Section) {
    $bytes = [IO.File]::ReadAllBytes($Path)
    $result = New-Object byte[] ([int]$Section.RawSize)
    [Array]::Copy($bytes, [int]$Section.RawPointer, $result, 0, [int]$Section.RawSize)
    return $result
}

function Assert-GoBuildInfo([string]$Path, [string]$Label) {
    $output = @(& $GoExe version -m $Path 2>&1)
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        throw "$Label 的 go version -m 失败（exit $exitCode）：$($output -join "`n")"
    }
    $text = $output -join "`n"
    if ($text -notmatch "go1\.27\.2") {
        throw "$Label 的 Go build info 缺少 go1.27.2：$text"
    }
}

function Assert-GoToolReaders([string]$Path, [string]$Label) {
    $nmOutput = @(& $GoExe tool nm $Path 2>&1)
    $nmExitCode = $LASTEXITCODE
    if ($nmExitCode -ne 0 -or ($nmOutput -join "`n") -notmatch "main\.main") {
        throw "$Label 的 go tool nm 失败或未读到 main.main（exit $nmExitCode）：$($nmOutput -join "`n")"
    }

    # objdump exercises the unstripped executable's symbol/pclntab and long
    # section mappings after the resource section has been inserted.
    $objdumpOutput = @(& $GoExe tool objdump -s main.main $Path 2>&1)
    $objdumpExitCode = $LASTEXITCODE
    if ($objdumpExitCode -ne 0 -or $objdumpOutput.Count -eq 0) {
        throw "$Label 的 go tool objdump 失败（exit $objdumpExitCode）：$($objdumpOutput -join "`n")"
    }
}

function Invoke-GoBuild([string[]]$Arguments, [string]$Label) {
    Write-Host ("> go " + ($Arguments -join " ")) -ForegroundColor DarkGray
    & $GoExe @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Label 构建失败（exit $LASTEXITCODE）。"
    }
}

$environmentNames = @("GOROOT", "GO111MODULE", "GOTOOLCHAIN", "GOWORK", "GOOS", "GOARCH", "CGO_ENABLED")
$originalEnvironment = @{}
foreach ($name in $environmentNames) {
    $originalEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
}

$testRoot = if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    Join-Path ([IO.Path]::GetTempPath()) ("proxy-start-set-icon-" + [guid]::NewGuid().ToString("N"))
}
elseif ([IO.Path]::IsPathRooted($OutputDir)) {
    [IO.Path]::GetFullPath($OutputDir)
}
else {
    Join-Path $projectRoot $OutputDir
}

try {
    New-Item -ItemType Directory -Force -Path $testRoot | Out-Null
    $fixtureSource = Join-Path $testRoot "fixture"
    New-Item -ItemType Directory -Path $fixtureSource | Out-Null
    $fixtureModule = @"
module example.com/proxy-start-icon-fixture

go 1.27
"@
    $fixtureProgram = @"
package main

import "fmt"

func main() { fmt.Println("proxy-start icon fixture") }
"@
    $utf8 = New-Object Text.UTF8Encoding($false)
    [IO.File]::WriteAllText((Join-Path $fixtureSource "go.mod"), $fixtureModule, $utf8)
    [IO.File]::WriteAllText((Join-Path $fixtureSource "main.go"), $fixtureProgram, $utf8)

    $env:GOROOT = Split-Path (Split-Path $GoExe -Parent) -Parent
    $env:GO111MODULE = "on"
    $env:GOTOOLCHAIN = "local"
    $env:GOWORK = "off"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"

    $unstripped = Join-Path $testRoot "fixture-unstripped.exe"
    $stripped = Join-Path $testRoot "fixture-stripped.exe"
    Push-Location -LiteralPath $fixtureSource
    try {
        Invoke-GoBuild @("build", "-trimpath", "-o", $unstripped, ".") "unstripped fixture"
        Invoke-GoBuild @("build", "-trimpath", "-ldflags", "-s -w", "-o", $stripped, ".") "stripped fixture"
    }
    finally {
        Pop-Location
    }

    foreach ($fixture in @(
        [pscustomobject]@{ Path = $unstripped; Label = "unstripped" },
        [pscustomobject]@{ Path = $stripped; Label = "stripped" }
    )) {
        Assert-GoBuildInfo $fixture.Path "$($fixture.Label) fixture before icon injection"
        $before = Get-SymbolInfo $fixture.Path
        $beforeSymbols = Get-SectionBytes $fixture.Path $before.Section

        # The unstripped Go linker emits long section names (for example /4
        # and /19) through its COFF string table. Assert that fixture is
        # present so go version -m also covers long-name resolution.
        if ($fixture.Label -eq "unstripped") {
            $longSections = @($before.Layout.Sections | Where-Object { $_.NameBytes[0] -eq [byte][char]'/' })
            if ($longSections.Count -eq 0) {
                throw "unstripped fixture 没有 Go COFF 长节名。"
            }
        }

        & $iconScript -ExePath $fixture.Path -IconPath $iconPath
        if ($LASTEXITCODE -ne 0) {
            throw "$($fixture.Label) fixture 图标注入失败。"
        }
        Assert-GoBuildInfo $fixture.Path "$($fixture.Label) fixture after first icon injection"
        if ($fixture.Label -eq "unstripped") {
            Assert-GoToolReaders $fixture.Path "unstripped fixture after first icon injection"
        }
        $after = Get-SymbolInfo $fixture.Path
        $afterSymbols = Get-SectionBytes $fixture.Path $after.Section
        if ($after.Offset -ne $before.Offset) {
            throw "$($fixture.Label) fixture 的符号表节内偏移改变：$($before.Offset) -> $($after.Offset)"
        }
        if (-not (Test-BytesEqual $beforeSymbols $afterSymbols)) {
            throw "$($fixture.Label) fixture 的符号/字符串表内容被图标注入修改。"
        }
        if (@($after.Layout.Sections | Where-Object { [Text.Encoding]::ASCII.GetString($_.NameBytes).Trim([char]0) -eq ".rsrc" }).Count -ne 1) {
            throw "$($fixture.Label) fixture 没有唯一 .rsrc 节。"
        }

        # Updating an existing resource must remain idempotent and preserve
        # the repaired COFF pointer and symbol/string table.
        & $iconScript -ExePath $fixture.Path -IconPath $iconPath
        if ($LASTEXITCODE -ne 0) {
            throw "$($fixture.Label) fixture 二次图标注入失败。"
        }
        Assert-GoBuildInfo $fixture.Path "$($fixture.Label) fixture after second icon injection"
        if ($fixture.Label -eq "unstripped") {
            Assert-GoToolReaders $fixture.Path "unstripped fixture after second icon injection"
        }
        $second = Get-SymbolInfo $fixture.Path
        $secondSymbols = Get-SectionBytes $fixture.Path $second.Section
        if ($second.Offset -ne $before.Offset -or -not (Test-BytesEqual $beforeSymbols $secondSymbols)) {
            throw "$($fixture.Label) fixture 二次注入后符号/字符串表未保持不变。"
        }
        Write-Host "通过：$($fixture.Label) fixture 的图标、COFF 指针、符号表及二次注入验证。" -ForegroundColor Green
    }
}
finally {
    foreach ($name in $environmentNames) {
        $value = $originalEnvironment[$name]
        if ($null -eq $value) {
            Remove-Item -LiteralPath ("Env:" + $name) -ErrorAction SilentlyContinue
        }
        else {
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
    }
    if ([string]::IsNullOrWhiteSpace($OutputDir) -and (Test-Path -LiteralPath $testRoot)) {
        $cleanupTarget = [IO.Path]::GetFullPath($testRoot)
        $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\', '/')
        $cleanupParent = [IO.Directory]::GetParent($cleanupTarget).FullName.TrimEnd('\', '/')
        if ($cleanupParent -ine $tempRoot -or [IO.Path]::GetFileName($cleanupTarget) -notmatch '^proxy-start-set-icon-[0-9a-f]{32}$') {
            throw "拒绝清理测试临时目录之外的路径：$cleanupTarget"
        }
        Remove-Item -LiteralPath $cleanupTarget -Recurse -Force
    }
}

Write-Host "set-icon PE 回归测试通过。" -ForegroundColor Green
