[CmdletBinding()]
param([string]$OutputDir = "", [string]$Version = "", [switch]$SkipTests, [switch]$Force, [string]$GoExe = "")
$ErrorActionPreference = "Stop"
if (-not $IsMacOS) { throw "DMG 必须在 macOS 上构建；请使用 GitHub Actions 的 macOS 构建任务。" }
if ([string]::IsNullOrWhiteSpace($OutputDir)) { $OutputDir = "dist/release-macos" }
if ([string]::IsNullOrWhiteSpace($Version)) { $Version = (Get-Content (Join-Path $PSScriptRoot "../VERSION") -Raw).Trim() }
$oldGo = $env:GO_BIN
$oldSkip = $env:SKIP_TESTS
try {
    if ($GoExe) { $env:GO_BIN = $GoExe }
    $env:SKIP_TESTS = if ($SkipTests) { "1" } else { "0" }
    & bash (Join-Path $PSScriptRoot "pack-macos.sh") $OutputDir $Version $Force.IsPresent.ToString().ToLowerInvariant()
    if ($LASTEXITCODE -ne 0) { throw "DMG 构建失败" }
} finally {
    $env:GO_BIN = $oldGo
    $env:SKIP_TESTS = $oldSkip
}
