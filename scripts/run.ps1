[CmdletBinding()]
param(
    [switch]$Helper,
    [switch]$Build,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Arguments
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$outputDir = Join-Path $projectRoot "dist"
$name = if ($Helper) { "proxy-start-helper.exe" } else { "proxy-start.exe" }
$executable = Join-Path $outputDir $name

if ($Build -or -not (Test-Path -LiteralPath $executable -PathType Leaf)) {
    & (Join-Path $PSScriptRoot "build.ps1")
    if ($LASTEXITCODE -ne 0) { throw "构建失败，无法启动 $name。" }
}

if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) {
    throw "启动文件不存在：$executable"
}

Write-Host ("启动：" + $executable) -ForegroundColor DarkGray
& $executable @Arguments
exit $LASTEXITCODE
