[CmdletBinding(SupportsShouldProcess)]
param()

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
        throw "卸载目标必须是指定用户目录下的子目录：$targetPath"
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
        throw "拒绝删除未由 Proxy Start 管理的目标：$InstallPath"
    }
    $markerPath = Join-Path $InstallPath $installationMarkerName
    if (-not (Test-Path -LiteralPath $markerPath -PathType Leaf)) {
        throw "拒绝删除未由 Proxy Start 安装脚本管理的目录（缺少所有权标记）：$InstallPath"
    }
    Assert-NoReparsePointInPath $markerPath $InstallPath
    $marker = [IO.File]::ReadAllText($markerPath).Trim()
    if ($marker -cne $installationMarkerValue) {
        throw "拒绝删除未由 Proxy Start 安装脚本管理的目录（所有权标记不匹配）：$InstallPath"
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
                throw "Proxy Start 仍在运行（PID $($process.Id)）。请先从托盘退出客户端，再重新运行卸载。脚本不会结束进程。"
            }
        }
    }
}

$programsRoot = Join-Path ([Environment]::GetFolderPath("LocalApplicationData")) "Programs"
$installRoot = Join-Path $programsRoot "Proxy Start"
Assert-ChildTarget $installRoot $programsRoot
if (-not (Test-Path -LiteralPath $installRoot -PathType Container)) {
    if (Test-Path -LiteralPath $installRoot) {
        throw "拒绝删除未由 Proxy Start 安装脚本管理的目标：$installRoot"
    }
    if ($WhatIfPreference) {
        Write-Host "WhatIf：未找到受 Proxy Start 管理的安装目录，未删除任何文件。" -ForegroundColor Yellow
    }
    else {
        Write-Host "未找到受 Proxy Start 管理的安装目录，未删除任何文件。" -ForegroundColor Yellow
    }
    return
}
Assert-ManagedInstall $installRoot
Assert-ClientStopped $installRoot

$startMenuRoot = [Environment]::GetFolderPath("Programs")
$shortcutDir = Join-Path $startMenuRoot "Proxy Start"
$desktopRoot = [Environment]::GetFolderPath("Desktop")
Assert-ChildTarget $shortcutDir $startMenuRoot

foreach ($shortcut in @(
    (Join-Path $shortcutDir "Proxy Start.lnk"),
    (Join-Path $shortcutDir "Uninstall Proxy Start.lnk"),
    (Join-Path $desktopRoot "Proxy Start.lnk")
)) {
    $parent = Split-Path -Parent $shortcut
    Assert-NoReparsePointInPath $shortcut $parent
    if (Test-Path -LiteralPath $shortcut -PathType Leaf) {
        if ($PSCmdlet.ShouldProcess($shortcut, "删除快捷方式")) {
            Remove-Item -LiteralPath $shortcut -Force
        }
    }
}
if (Test-Path -LiteralPath $shortcutDir -PathType Container) {
    $children = @(Get-ChildItem -LiteralPath $shortcutDir -Force)
    if ($children.Count -eq 0 -and $PSCmdlet.ShouldProcess($shortcutDir, "删除空的开始菜单文件夹")) {
        Remove-Item -LiteralPath $shortcutDir -Force
    }
}

if (Test-Path -LiteralPath $installRoot) {
    Assert-ChildTarget $installRoot $programsRoot
    Assert-ManagedInstall $installRoot
    Assert-TreeNoReparsePoints $installRoot
    if ($PSCmdlet.ShouldProcess($installRoot, "删除 Proxy Start 安装目录")) {
        Remove-Item -LiteralPath $installRoot -Recurse -Force
    }
}

if ($WhatIfPreference) {
    Write-Host "WhatIf：未删除任何文件。目标安装目录为 $installRoot。" -ForegroundColor Yellow
}
else {
    Write-Host "Proxy Start 已卸载。用户配置和凭据未删除。" -ForegroundColor Green
    Write-Host "ProxiFyre 服务、驱动和目标软件进程未被修改。" -ForegroundColor DarkGray
}
