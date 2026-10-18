[CmdletBinding(SupportsShouldProcess)]
param(
    [switch]$DesktopShortcut
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$installationMarkerName = ".proxy-start-install.marker"
$installationMarkerValue = "proxy-start-managed-v1"

function Normalize-Path([string]$Path) {
    $full = [System.IO.Path]::GetFullPath($Path)
    $full = $full.TrimEnd('\')
    if ($full.Length -eq 2 -and $full[1] -eq ':') { $full += '\' }
    return $full
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

function Assert-ChildTarget([string]$Target, [string]$Root) {
    $targetPath = Normalize-Path $Target
    $rootPath = Normalize-Path $Root
    $prefix = if ($rootPath.EndsWith('\')) { $rootPath } else { $rootPath + '\' }
    if ($targetPath -ieq $rootPath -or -not $targetPath.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "安装目标必须是指定用户目录下的子目录：$targetPath"
    }
    Assert-NoReparsePointInPath $targetPath $rootPath
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

function Assert-ManagedInstall([string]$InstallPath) {
    if (-not (Test-Path -LiteralPath $InstallPath -PathType Container)) {
        throw "拒绝替换未由 Proxy Start 管理的目标：$InstallPath"
    }
    $markerPath = Join-Path $InstallPath $installationMarkerName
    if (-not (Test-Path -LiteralPath $markerPath -PathType Leaf)) {
        throw "拒绝替换未由 Proxy Start 安装脚本管理的目录（缺少所有权标记）：$InstallPath"
    }
    Assert-NoReparsePointInPath $markerPath $InstallPath
    $marker = [IO.File]::ReadAllText($markerPath).Trim()
    if ($marker -cne $installationMarkerValue) {
        throw "拒绝替换未由 Proxy Start 安装脚本管理的目录（所有权标记不匹配）：$InstallPath"
    }
}

function Get-ProcessPath($Process) {
    try { return $Process.Path } catch {}
    try { return $Process.MainModule.FileName } catch { return $null }
}

function Assert-ClientStopped([string]$InstallPath) {
    foreach ($name in @("proxy-start", "proxy-start-helper")) {
        $expected = Normalize-Path (Join-Path $InstallPath ($name + ".exe"))
        foreach ($process in @(Get-Process -Name $name -ErrorAction SilentlyContinue)) {
            $path = Get-ProcessPath $process
            if ([string]::IsNullOrWhiteSpace($path) -or (Normalize-Path $path) -ieq $expected) {
                throw "Proxy Start 仍在运行（PID $($process.Id)）。请先从托盘退出客户端，再重新安装。脚本不会结束进程。"
            }
        }
    }
}

$sourceRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$sourceExe = Join-Path $sourceRoot "proxy-start.exe"
$sourceHelper = Join-Path $sourceRoot "proxy-start-helper.exe"
$sourceBrandIcon = Join-Path $sourceRoot "assets\proxy-start.ico"
if (-not (Test-Path -LiteralPath $sourceExe -PathType Leaf) -or -not (Test-Path -LiteralPath $sourceHelper -PathType Leaf)) {
    throw "安装包不完整：需要 proxy-start.exe 和 proxy-start-helper.exe。"
}
if (-not (Test-Path -LiteralPath $sourceBrandIcon -PathType Leaf)) {
    throw "安装包不完整：缺少 Proxy Start 品牌图标 $sourceBrandIcon。"
}

$programsRoot = Join-Path ([Environment]::GetFolderPath("LocalApplicationData")) "Programs"
$installRoot = Join-Path $programsRoot "Proxy Start"
Assert-ChildTarget $installRoot $programsRoot
if (Test-Path -LiteralPath $installRoot) {
    Assert-ManagedInstall $installRoot
}
Assert-ClientStopped $installRoot
if ($WhatIfPreference) {
    Write-Host "WhatIf：将安装到 $installRoot，并创建开始菜单快捷方式；未执行任何写入。" -ForegroundColor Yellow
    return
}
New-Item -ItemType Directory -Force -Path $programsRoot | Out-Null

$stage = Join-Path $programsRoot (".ProxyStart-installing-" + $PID)
Assert-ChildTarget $stage $programsRoot
try {
    if (Test-Path -LiteralPath $stage) {
        Assert-TreeNoReparsePoints $stage
        Remove-Item -LiteralPath $stage -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $stage | Out-Null
    Copy-Item -Path (Join-Path $sourceRoot "*") -Destination $stage -Recurse -Force

    if (Test-Path -LiteralPath $installRoot) {
        Assert-ChildTarget $installRoot $programsRoot
        Assert-ManagedInstall $installRoot
        Assert-TreeNoReparsePoints $installRoot
        Remove-Item -LiteralPath $installRoot -Recurse -Force
    }
    $markerPath = Join-Path $stage $installationMarkerName
    [IO.File]::WriteAllText($markerPath, $installationMarkerValue, [Text.Encoding]::ASCII)
    Move-Item -LiteralPath $stage -Destination $installRoot
}
finally {
    if (Test-Path -LiteralPath $stage) {
        Assert-ChildTarget $stage $programsRoot
        Assert-TreeNoReparsePoints $stage
        Remove-Item -LiteralPath $stage -Recurse -Force
    }
}

$startMenuRoot = [Environment]::GetFolderPath("Programs")
$shortcutDir = Join-Path $startMenuRoot "Proxy Start"
$desktopRoot = [Environment]::GetFolderPath("Desktop")
Assert-ChildTarget $shortcutDir $startMenuRoot
$shell = New-Object -ComObject WScript.Shell
New-Item -ItemType Directory -Force -Path $shortcutDir | Out-Null

function New-Shortcut([string]$Path, [string]$Target, [string]$Arguments, [string]$WorkingDirectory, [string]$Description, [string]$IconLocation = "") {
    Assert-NoReparsePointInPath $Path (Split-Path -Parent $Path)
    $shortcut = $shell.CreateShortcut($Path)
    $shortcut.TargetPath = $Target
    $shortcut.Arguments = $Arguments
    $shortcut.WorkingDirectory = $WorkingDirectory
    $shortcut.Description = $Description
    if ([string]::IsNullOrWhiteSpace($IconLocation)) {
        $shortcut.IconLocation = "$Target,0"
    }
    else {
        $shortcut.IconLocation = $IconLocation
    }
    $shortcut.Save()
}

$brandIconPath = Join-Path $installRoot "assets\proxy-start.ico"
if (-not (Test-Path -LiteralPath $brandIconPath -PathType Leaf)) {
    throw "安装包不完整：缺少 Proxy Start 品牌图标 $brandIconPath。"
}
New-Shortcut (Join-Path $shortcutDir "Proxy Start.lnk") (Join-Path $installRoot "proxy-start.exe") "" $installRoot "Proxy Start" $brandIconPath
$powershell = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"
$uninstallArgs = "-NoProfile -ExecutionPolicy Bypass -File `"$(Join-Path $installRoot 'scripts\uninstall.ps1')`""
New-Shortcut (Join-Path $shortcutDir "Uninstall Proxy Start.lnk") $powershell $uninstallArgs $installRoot "卸载 Proxy Start"
if ($DesktopShortcut) {
    New-Shortcut (Join-Path $desktopRoot "Proxy Start.lnk") (Join-Path $installRoot "proxy-start.exe") "" $installRoot "Proxy Start" $brandIconPath
}

Write-Host "已安装到：$installRoot" -ForegroundColor Green
Write-Host "配置和凭据保留在用户目录，不会被安装或卸载脚本删除。" -ForegroundColor DarkGray
Write-Host "安装脚本不会安装、启动或停止 ProxiFyre 服务/驱动。" -ForegroundColor DarkGray
