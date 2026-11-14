[CmdletBinding()]
param([string]$OutputDir = "", [string]$GoExe = "", [switch]$SkipTests, [switch]$Force)
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ([string]::IsNullOrWhiteSpace($OutputDir)) { $OutputDir = Join-Path $projectRoot "dist/release-windows" }
elseif (-not [IO.Path]::IsPathRooted($OutputDir)) { $OutputDir = Join-Path $projectRoot $OutputDir }
$OutputDir = [IO.Path]::GetFullPath($OutputDir)
$destination = Join-Path $OutputDir "ProxyStart-windows-amd64.exe"
if ((Test-Path -LiteralPath $destination) -and -not $Force) { throw "发布文件已存在；覆盖需使用 -Force：$destination" }
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
if (@(Get-ChildItem -LiteralPath $OutputDir -Force | Where-Object { $_.FullName -ne $destination }).Count -ne 0) {
    throw "发布目录必须只包含目标 EXE，请使用独立目录：$OutputDir"
}
$buildDir = Join-Path $projectRoot (".cache/pack-windows/" + [guid]::NewGuid().ToString("N"))
$buildParams = @{ OutputDir = $buildDir; GUI = $true; SkipTests = $SkipTests }
if (-not [string]::IsNullOrWhiteSpace($GoExe)) { $buildParams.GoExe = $GoExe }
& (Join-Path $PSScriptRoot "build.ps1") @buildParams
if ($LASTEXITCODE -ne 0) { throw "Windows GUI 构建失败" }
Copy-Item -LiteralPath (Join-Path $buildDir "proxy-start.exe") -Destination $destination -Force
if (@(Get-ChildItem -LiteralPath $OutputDir -Force).Count -ne 1) { throw "发布目录含非预期文件" }
Write-Host "单文件 Windows 发布包：$destination"
Get-FileHash -Algorithm SHA256 -LiteralPath $destination
