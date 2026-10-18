[CmdletBinding()]
param(
    [string]$OutputDir = "",
    [string]$Proxy = "",
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $projectRoot "dist\dependencies"
}
elseif (-not [System.IO.Path]::IsPathRooted($OutputDir)) {
    $OutputDir = Join-Path $projectRoot $OutputDir
}

$manifestPath = Join-Path $projectRoot "internal\infrastructure\proxifyre\dependencies.json"
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
    throw "ProxiFyre 依赖清单不存在：$manifestPath"
}
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
$spec = $manifest.engine
if ($null -eq $spec -or [string]::IsNullOrWhiteSpace($spec.name) -or [string]::IsNullOrWhiteSpace($spec.source) -or [string]::IsNullOrWhiteSpace($spec.sha256)) {
    throw "ProxiFyre 依赖清单缺少 name/source/sha256。"
}
if ($spec.source -notmatch '^https://github\.com/wiresock/proxifyre/releases/download/v2\.6\.4/') {
    throw "拒绝非官方 ProxiFyre 资产地址：$($spec.source)"
}
if ($spec.sha256 -notmatch '^[0-9a-fA-F]{64}$') {
    throw "ProxiFyre SHA-256 格式无效：$($spec.sha256)"
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$destination = Join-Path $OutputDir $spec.name
$expected = $spec.sha256.ToLowerInvariant()

function Get-Sha256([string]$Path) {
    return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

if ((-not $Force) -and (Test-Path -LiteralPath $destination -PathType Leaf)) {
    $existing = Get-Sha256 $destination
    if ($existing -eq $expected) {
        Write-Host "已存在并通过校验：$destination" -ForegroundColor Green
        return
    }
    Remove-Item -LiteralPath $destination -Force
}

$temporary = "$destination.download"
if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Force }
$request = @{
    Uri = $spec.source
    OutFile = $temporary
    UseBasicParsing = $true
    TimeoutSec = 900
}
if (-not [string]::IsNullOrWhiteSpace($Proxy)) {
    $request.Proxy = $Proxy
}
try {
    Write-Host "下载官方 ProxiFyre 资产：$($spec.source)" -ForegroundColor DarkGray
    Invoke-WebRequest @request
}
catch {
    if (-not [string]::IsNullOrWhiteSpace($Proxy)) { throw }
    Write-Host "直连失败，使用本机代理 http://127.0.0.1:7897 重试。" -ForegroundColor Yellow
    $request.Proxy = "http://127.0.0.1:7897"
    Invoke-WebRequest @request
}

$actual = Get-Sha256 $temporary
if ($actual -ne $expected) {
    Remove-Item -LiteralPath $temporary -Force
    throw "ProxiFyre SHA-256 校验失败：期望 $expected，实际 $actual"
}
Move-Item -LiteralPath $temporary -Destination $destination -Force
Write-Host "已下载并通过校验：$destination" -ForegroundColor Green
Write-Host "此脚本只下载和校验安装包，不会执行安装程序或驱动安装器。" -ForegroundColor DarkGray
