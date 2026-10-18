[CmdletBinding()]
param(
    [string]$OutputDir = "",
    [string]$Proxy = "",
    [string]$GoExe = "",
    [switch]$SkipTests,
    [switch]$Force
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
$OutputDir = [System.IO.Path]::GetFullPath($OutputDir)
$outputRoot = $OutputDir.TrimEnd('\')
if ($outputRoot.Length -eq 2 -and $outputRoot[1] -eq ':') { $outputRoot += '\' }

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

function Assert-SafeDeleteTarget([string]$Target, [string]$Root) {
    $targetPath = Normalize-Path $Target
    $rootPath = Normalize-Path $Root
    $rootPrefix = if ($rootPath.EndsWith('\')) { $rootPath } else { $rootPath + '\' }
    if ($targetPath -ieq $rootPath -or -not $targetPath.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "拒绝删除输出目录之外或输出目录本身的路径：$targetPath"
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

Assert-NoReparsePointInPath $outputRoot $outputRoot
$projectPath = Normalize-Path $projectRoot
$projectPrefix = if ($outputRoot.EndsWith('\')) { $outputRoot } else { $outputRoot + '\' }
if ($outputRoot -ieq $projectPath -or $projectPath.StartsWith($projectPrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "OutputDir 不能是项目根目录或其父目录：$outputRoot"
}

$packageName = "ProxyStart-windows-amd64"
$packageRoot = Join-Path $OutputDir $packageName
$brandIconSource = Join-Path $projectRoot "assets\proxy-start.ico"
$packageZip = Join-Path $OutputDir "$packageName.zip"
$sourceName = "ProxyStart-source"
$sourceRoot = Join-Path $OutputDir ".pack-staging\$sourceName"
$sourceZip = Join-Path $OutputDir "$sourceName.zip"
$releaseChecksums = Join-Path $OutputDir "checksums.sha256"

if (-not $Force) {
    $existing = @(@($packageRoot, $packageZip, $sourceRoot, $sourceZip, $releaseChecksums) | Where-Object { Test-Path -LiteralPath $_ })
    if ($existing.Count -gt 0) {
        throw "发布目标已存在：$($existing -join ', ')。如需覆盖请明确使用 -Force。"
    }
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
if ($Force) {
    foreach ($path in @($packageRoot, $packageZip, $sourceRoot, $sourceZip, $releaseChecksums)) {
        Assert-SafeDeleteTarget $path $outputRoot
        if (Test-Path -LiteralPath $path) {
            if ((Get-Item -LiteralPath $path -Force).PSIsContainer) { Assert-TreeNoReparsePoints $path }
            Remove-Item -LiteralPath $path -Recurse -Force
        }
    }
}
New-Item -ItemType Directory -Force -Path $packageRoot | Out-Null

if (-not (Test-Path -LiteralPath $brandIconSource -PathType Leaf)) {
    throw "缺少 Proxy Start 品牌图标资源：$brandIconSource"
}
$brandIconDestination = Join-Path $packageRoot "assets\proxy-start.ico"
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $brandIconDestination) | Out-Null
Copy-Item -LiteralPath $brandIconSource -Destination $brandIconDestination -Force

$buildScript = Join-Path $PSScriptRoot "build.ps1"
$buildParams = @{ OutputDir = $packageRoot; GUI = $true }
if (-not [string]::IsNullOrWhiteSpace($GoExe)) { $buildParams.GoExe = $GoExe }
if ($SkipTests) { $buildParams.SkipTests = $true }
Write-Host "> build GUI binaries" -ForegroundColor DarkGray
& $buildScript @buildParams
if ($LASTEXITCODE -ne 0) { throw "GUI 构建失败。" }

$dependencies = Join-Path $packageRoot "dependencies"
$fetchScript = Join-Path $PSScriptRoot "fetch-engine.ps1"
$fetchParams = @{ OutputDir = $dependencies }
if (-not [string]::IsNullOrWhiteSpace($Proxy)) { $fetchParams.Proxy = $Proxy }
Write-Host "> fetch pinned ProxiFyre setup" -ForegroundColor DarkGray
& $fetchScript @fetchParams
if ($LASTEXITCODE -ne 0) { throw "ProxiFyre 依赖获取失败。" }

$manifestSource = Join-Path $projectRoot "internal\infrastructure\proxifyre\dependencies.json"
Copy-Item -LiteralPath $manifestSource -Destination (Join-Path $dependencies "dependencies.json") -Force
foreach ($file in @("PRODUCT.md", "DESIGN.md", "LICENSE-THIRD-PARTY.md")) {
    $source = Join-Path $projectRoot $file
    if (Test-Path -LiteralPath $source -PathType Leaf) {
        Copy-Item -LiteralPath $source -Destination (Join-Path $packageRoot $file) -Force
    }
}
$readme = Join-Path $projectRoot "README.md"
if (Test-Path -LiteralPath $readme -PathType Leaf) {
    Copy-Item -LiteralPath $readme -Destination (Join-Path $packageRoot "README.md") -Force
}
elseif (Test-Path -LiteralPath (Join-Path $projectRoot "PRODUCT.md") -PathType Leaf) {
    # Keep the binary package self-describing until a dedicated README exists.
    Copy-Item -LiteralPath (Join-Path $projectRoot "PRODUCT.md") -Destination (Join-Path $packageRoot "README.md") -Force
}
$docsSource = Join-Path $projectRoot "docs"
if (Test-Path -LiteralPath $docsSource -PathType Container) {
    $docsDestination = Join-Path $packageRoot "docs"
    Get-ChildItem -LiteralPath $docsSource -Recurse -File -Force | ForEach-Object {
        $relative = $_.FullName.Substring($docsSource.Length).TrimStart('\')
        $destination = Join-Path $docsDestination $relative
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination) | Out-Null
        Copy-Item -LiteralPath $_.FullName -Destination $destination -Force
    }
    $buildDocs = Join-Path $docsSource "BUILD.md"
    if (Test-Path -LiteralPath $buildDocs -PathType Leaf) {
        Copy-Item -LiteralPath $buildDocs -Destination (Join-Path $packageRoot "BUILD.md") -Force
    }
}
$runtimeScripts = Join-Path $packageRoot "scripts"
New-Item -ItemType Directory -Force -Path $runtimeScripts | Out-Null
foreach ($file in @("install.ps1", "install.cmd", "uninstall.ps1", "uninstall.cmd")) {
    $source = Join-Path $PSScriptRoot $file
    if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
        throw "缺少发布入口脚本：$source"
    }
    Copy-Item -LiteralPath $source -Destination (Join-Path $runtimeScripts $file) -Force
}

function Write-TreeChecksums([string]$Root, [string]$Path) {
    $lines = @(
        Get-ChildItem -LiteralPath $Root -Recurse -File |
            Where-Object { $_.FullName -ne $Path } |
            Sort-Object FullName |
            ForEach-Object {
                $relative = $_.FullName.Substring($Root.Length).TrimStart('\').Replace('\', '/')
                $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant()
                "$hash  $relative"
            }
    )
    Set-Content -LiteralPath $Path -Value $lines -Encoding ASCII
}

$packageChecksums = Join-Path $packageRoot "checksums.sha256"
Write-TreeChecksums $packageRoot $packageChecksums

New-Item -ItemType Directory -Force -Path $sourceRoot | Out-Null
$excludedRoots = @($outputRoot, (Normalize-Path (Join-Path $projectRoot ".tools")), (Normalize-Path (Join-Path $projectRoot "dist")))
$excludedDirectoryNames = @(".git", ".cache", ".impeccable", "__pycache__")
function Test-IsExcludedPath([string]$Path) {
    $candidate = Normalize-Path $Path
    foreach ($excluded in $excludedRoots) {
        $prefix = if ($excluded.EndsWith('\')) { $excluded } else { $excluded + '\' }
        if ($candidate -ieq $excluded -or $candidate.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
            return $true
        }
    }
    return $false
}
function Copy-SourceTree([string]$Directory) {
    foreach ($entry in Get-ChildItem -LiteralPath $Directory -Force) {
        if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { continue }
        if (Test-IsExcludedPath $entry.FullName) { continue }
        if ($entry.PSIsContainer) {
            if ($excludedDirectoryNames -contains $entry.Name) { continue }
            Copy-SourceTree $entry.FullName
            continue
        }
        if ($entry.Name -match '\.test\.exe$|\.py[cod]$') { continue }
        $relative = $entry.FullName.Substring($projectRoot.Length).TrimStart('\')
        $destination = Join-Path $sourceRoot $relative
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination) | Out-Null
        Copy-Item -LiteralPath $entry.FullName -Destination $destination -Force
    }
}
Copy-SourceTree $projectRoot
$designSource = Join-Path $projectRoot ".impeccable\design.json"
if (Test-Path -LiteralPath $designSource -PathType Leaf) {
    $designItem = Get-Item -LiteralPath $designSource -Force
    if (($designItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "拒绝复制重解析点设计文件：$designSource"
    }
    $designDestination = Join-Path $sourceRoot ".impeccable\design.json"
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $designDestination) | Out-Null
    Copy-Item -LiteralPath $designSource -Destination $designDestination -Force
}
$sourceChecksums = Join-Path $sourceRoot "checksums.sha256"
Write-TreeChecksums $sourceRoot $sourceChecksums

Compress-Archive -LiteralPath $packageRoot -DestinationPath $packageZip -CompressionLevel Optimal -Force
Compress-Archive -LiteralPath $sourceRoot -DestinationPath $sourceZip -CompressionLevel Optimal -Force

$releaseLines = @(
    "$((Get-FileHash -Algorithm SHA256 -LiteralPath $packageZip).Hash.ToLowerInvariant())  $([System.IO.Path]::GetFileName($packageZip))"
    "$((Get-FileHash -Algorithm SHA256 -LiteralPath $sourceZip).Hash.ToLowerInvariant())  $([System.IO.Path]::GetFileName($sourceZip))"
)
Set-Content -LiteralPath $releaseChecksums -Value $releaseLines -Encoding ASCII

Write-Host "发布包已生成：$packageZip" -ForegroundColor Green
Write-Host "源码包已生成（不含 .tools/）：$sourceZip" -ForegroundColor Green
Write-Host "发布校验文件：$releaseChecksums" -ForegroundColor Green
Write-Host "未执行 ProxiFyre 安装程序或驱动安装器。" -ForegroundColor DarkGray
