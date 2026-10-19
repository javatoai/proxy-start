[CmdletBinding()]
param(
    [string]$OutputDir = "",
    [switch]$SkipTests,
    [switch]$KeepSymbols,
    [switch]$GUI,
    [string]$GoExe = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $projectRoot "dist"
}
elseif (-not [System.IO.Path]::IsPathRooted($OutputDir)) {
    $OutputDir = Join-Path $projectRoot $OutputDir
}

$bundledGoRoot = Join-Path $projectRoot ".tools\go"
$bundledGoExe = Join-Path $bundledGoRoot "bin\go.exe"
$goExe = $null
if (-not [string]::IsNullOrWhiteSpace($GoExe)) {
    $goCommand = Get-Command $GoExe -ErrorAction SilentlyContinue
    if ($null -ne $goCommand -and $goCommand.CommandType -eq "Application") {
        $goExe = $goCommand.Source
    }
    elseif (Test-Path -LiteralPath $GoExe -PathType Leaf) {
        $goExe = (Resolve-Path -LiteralPath $GoExe).Path
    }
    else {
        throw "指定的 Go 可执行文件不存在：$GoExe"
    }
}
elseif (Test-Path -LiteralPath $bundledGoExe -PathType Leaf) {
    $goExe = (Resolve-Path -LiteralPath $bundledGoExe).Path
}
else {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if ($null -ne $goCommand) { $goExe = $goCommand.Source }
}
if ([string]::IsNullOrWhiteSpace($goExe) -or -not (Test-Path -LiteralPath $goExe -PathType Leaf)) {
    throw "找不到 Go；请准备项目内 .tools/go，或使用 -GoExe 指定 Go 1.27.x。"
}

$goRoot = Split-Path (Split-Path $goExe -Parent) -Parent
$goRoot = (Resolve-Path -LiteralPath $goRoot).Path
$goVersion = $null
$originalLocation = Get-Location
$pushedLocation = $false
$environmentNames = @("GOROOT", "GO111MODULE", "GOTOOLCHAIN", "GOWORK", "GOOS", "GOARCH", "CGO_ENABLED")
$originalEnvironment = @{}
foreach ($name in $environmentNames) {
    $originalEnvironment[$name] = [Environment]::GetEnvironmentVariable($name, "Process")
}

try {
    Push-Location -LiteralPath $projectRoot
    $pushedLocation = $true
    $goVersion = (& $goExe version).Trim()
    if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch "go1\.27\.2\s+windows/amd64") {
        throw "项目工具链版本不匹配：$goVersion；需要 go1.27.2 windows/amd64。"
    }

    # Keep the build hermetic even when the host has another Go installation or
    # GO111MODULE/GOTOOLCHAIN settings configured globally.
    $env:GOROOT = $goRoot
    $env:GO111MODULE = "on"
    $env:GOTOOLCHAIN = "local"
    $env:GOWORK = "off"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "0"

    $commands = @(
        @{ Name = "go version"; Args = @("version") },
        @{ Name = "go env"; Args = @("env", "GOROOT", "GOMOD", "GOOS", "GOARCH", "CGO_ENABLED") }
    )
    foreach ($command in $commands) {
        Write-Host ("> " + $command.Name) -ForegroundColor DarkGray
        & $goExe @($command.Args)
        if ($LASTEXITCODE -ne 0) { throw "命令失败：$($command.Name)" }
    }

    if (-not $SkipTests) {
        Write-Host "> go test ./..." -ForegroundColor DarkGray
        & $goExe test ./...
        if ($LASTEXITCODE -ne 0) { throw "测试失败；使用 -SkipTests 仅构建产物。" }
    }

    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
    $linkerFlags = @()
    if (-not $KeepSymbols) { $linkerFlags += @("-s", "-w") }
    if ($GUI) { $linkerFlags += "-H=windowsgui" }

    $helperPackage = "./cmd/proxy-start-helper"
    $mainPackage = "./cmd/proxy-start"
    $iconScript = Join-Path $PSScriptRoot "set-icon.ps1"
    $iconSource = Join-Path $projectRoot "assets\proxy-start.ico"
    if (-not (Test-Path -LiteralPath $iconScript -PathType Leaf)) {
        throw "缺少 Windows 图标注入脚本：$iconScript"
    }
    if (-not (Test-Path -LiteralPath $iconSource -PathType Leaf)) {
        throw "缺少 Proxy Start Windows 图标资源：$iconSource"
    }
    foreach ($package in @($helperPackage, $mainPackage)) {
        $packageDir = Join-Path $projectRoot ($package -replace '^\./', '').Replace('/', '\')
        if (-not (Test-Path -LiteralPath $packageDir -PathType Container)) {
            throw "缺少构建入口：$packageDir"
        }
    }

    # Build the elevated helper first. The client embeds this exact digest so
    # a stale, replaced, or unrelated helper is rejected before use.
    $helperOutput = Join-Path $OutputDir "proxy-start-helper.exe"
    $helperLdflags = $linkerFlags -join " "
    $helperArgs = @("build", "-trimpath")
    if (-not [string]::IsNullOrWhiteSpace($helperLdflags)) {
        $helperArgs += @("-ldflags", $helperLdflags)
    }
    $helperArgs += @("-o", $helperOutput, $helperPackage)
    Write-Host ("> go " + ($helperArgs -join " ")) -ForegroundColor DarkGray
    & $goExe @helperArgs
    if ($LASTEXITCODE -ne 0) { throw "构建失败：$helperPackage" }
    Write-Host "> set Windows icon (helper)" -ForegroundColor DarkGray
    & $iconScript -ExePath $helperOutput -IconPath $iconSource
    if ($LASTEXITCODE -ne 0) { throw "helper 图标资源注入失败。" }
    $helperSHA256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $helperOutput).Hash.ToLowerInvariant()
    Write-Host ("已生成 helper：$helperOutput") -ForegroundColor Green
    Write-Host ("helper SHA-256：$helperSHA256") -ForegroundColor DarkGray

    $mainOutput = Join-Path $OutputDir "proxy-start.exe"
    $mainLinkerFlags = @($linkerFlags)
    $mainLinkerFlags += @("-X", "main.helperSHA256=$helperSHA256")
    $mainLdflags = $mainLinkerFlags -join " "
    $mainArgs = @("build", "-trimpath", "-ldflags", $mainLdflags, "-o", $mainOutput, $mainPackage)
    Write-Host ("> go " + ($mainArgs -join " ")) -ForegroundColor DarkGray
    & $goExe @mainArgs
    if ($LASTEXITCODE -ne 0) { throw "构建失败：$mainPackage" }
    Write-Host "> set Windows icon (client)" -ForegroundColor DarkGray
    & $iconScript -ExePath $mainOutput -IconPath $iconSource
    if ($LASTEXITCODE -ne 0) { throw "客户端图标资源注入失败。" }
    Write-Host ("已生成：$mainOutput（内嵌 helper SHA-256：$helperSHA256）") -ForegroundColor Green

    Write-Host "构建完成。" -ForegroundColor Green
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
    if ($pushedLocation) {
        Pop-Location
    }
    else {
        Set-Location -LiteralPath $originalLocation
    }
}
