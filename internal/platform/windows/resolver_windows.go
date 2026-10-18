//go:build windows

package windows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

// Resolver resolves dropped executable files, Windows shortcuts, and packaged
// applications into the platform-independent identity consumed by the
// application layer.
type Resolver struct{}

var _ application.AppResolver = (*Resolver)(nil)

// NewResolver constructs a stateless Windows application resolver.
func NewResolver() *Resolver { return &Resolver{} }

func (r *Resolver) Resolve(ctx context.Context, input string) (domain.AppIdentity, error) {
	if err := ctx.Err(); err != nil {
		return domain.AppIdentity{}, err
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return domain.AppIdentity{}, errors.New("应用路径不能为空")
	}
	if isPackageReference(input) {
		return r.resolvePackage(ctx, input)
	}

	path := canonicalPath(input)
	info, err := os.Stat(path)
	if err != nil {
		return domain.AppIdentity{}, fmt.Errorf("无法读取应用路径 %q: %w", input, err)
	}
	if info.IsDir() {
		return domain.AppIdentity{}, fmt.Errorf("应用路径必须是可执行文件或快捷方式: %s", path)
	}
	if strings.EqualFold(filepath.Ext(path), ".lnk") {
		return r.resolveShortcut(ctx, path)
	}
	if !isLaunchableFile(path) {
		return domain.AppIdentity{}, fmt.Errorf("仅支持 .exe 文件，请选择实际程序或已安装应用: %s", path)
	}
	return directIdentity(path, nil, ""), nil
}

func (r *Resolver) Installed(ctx context.Context) ([]domain.AppIdentity, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var rows []packageRow
	if output, err := runPowerShellJSON(ctx, installedPackagesScript, nil); err != nil {
		// AppX enumeration can fail on restricted Windows accounts. Known
		// desktop installs remain useful, so return them while preserving the
		// error only if nothing else can be discovered.
		rows = nil
		known := knownBrowserIdentities()
		if len(known) == 0 {
			return nil, err
		}
		return known, nil
	} else if err := decodeJSONArray(output, &rows); err != nil {
		return nil, fmt.Errorf("解析已安装应用清单失败: %w", err)
	}

	result := make([]domain.AppIdentity, 0, len(rows)+2)
	seen := make(map[string]struct{})
	appendUnique := func(identity domain.AppIdentity) {
		key := catalogIdentityKey(identity)
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		// A packaged browser and its traditional Win32 installation can expose
		// two paths for the same user-facing app. Keep one browser entry while
		// retaining distinct packaged applications with different AUMIDs.
		for _, existing := range result {
			if sameKnownBrowser(existing, identity) {
				return
			}
		}
		seen[key] = struct{}{}
		result = append(result, identity)
	}
	for _, row := range rows {
		if row.Executable == "" {
			continue
		}
		// The PowerShell catalog already removes framework packages, hidden
		// AppListEntry=none entries and non-executable manifest entries. Keep
		// the guard here as well because package metadata is user-controlled
		// and this is the boundary at which it becomes a launch identity.
		if !isLaunchableFile(row.Executable) {
			continue
		}
		if strings.TrimSpace(row.PackageFamily) != "" && strings.TrimSpace(row.ApplicationID) == "" {
			continue
		}
		appendUnique(packageRowIdentity(row))
	}
	for _, identity := range knownBrowserIdentities() {
		appendUnique(identity)
	}
	return result, nil
}

func catalogIdentityKey(identity domain.AppIdentity) string {
	if aumid := strings.TrimSpace(identity.AUMID); aumid != "" {
		return "aumid:" + strings.ToLower(strings.TrimPrefix(aumid, "package:"))
	}
	if family := strings.TrimSpace(identity.PackageFamily); family != "" {
		applicationID := strings.TrimSpace(identity.ApplicationID)
		return "package:" + strings.ToLower(family) + "!" + strings.ToLower(applicationID)
	}
	if path := canonicalPath(identity.Executable); path != "" {
		return "path:" + strings.ToLower(path)
	}
	return ""
}

func sameKnownBrowser(a, b domain.AppIdentity) bool {
	// Only collapse the known packaged/Win32 alias pair. Two packaged
	// identities with different AUMIDs are independent launch entries even if
	// they happen to share a display name and executable basename.
	if isPackagedIdentity(a) == isPackagedIdentity(b) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(a.Name), strings.TrimSpace(b.Name)) {
		return false
	}
	baseA := strings.ToLower(filepath.Base(canonicalPath(a.Executable)))
	baseB := strings.ToLower(filepath.Base(canonicalPath(b.Executable)))
	if baseA == "" || baseA != baseB {
		return false
	}
	return baseA == "chrome.exe" || baseA == "msedge.exe"
}

func isPackagedIdentity(identity domain.AppIdentity) bool {
	return strings.TrimSpace(identity.AUMID) != "" ||
		strings.TrimSpace(identity.PackageFamily) != "" ||
		strings.TrimSpace(identity.PackageName) != ""
}

type packageRow struct {
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	Publisher        string `json:"publisher"`
	Executable       string `json:"executable"`
	WorkingDirectory string `json:"workingDirectory"`
	PackageName      string `json:"packageName"`
	PackageFamily    string `json:"packageFamily"`
	ApplicationID    string `json:"applicationId"`
	AUMID            string `json:"aumid"`
	AppListEntry     string `json:"appListEntry"`
	SystemApp        bool   `json:"systemApp"`
}

func packageRowIdentity(row packageRow) domain.AppIdentity {
	return domain.AppIdentity{
		Name:             friendlyPackageName(row.DisplayName, row.Name, row.ApplicationID),
		Executable:       canonicalPath(row.Executable),
		WorkingDirectory: canonicalPath(row.WorkingDirectory),
		Publisher:        strings.TrimSpace(row.Publisher),
		SystemApp:        row.SystemApp,
		PackageName:      strings.TrimSpace(row.PackageName),
		PackageFamily:    strings.TrimSpace(row.PackageFamily),
		ApplicationID:    strings.TrimSpace(row.ApplicationID),
		AUMID:            strings.TrimSpace(row.AUMID),
	}
}

func (r *Resolver) resolveShortcut(ctx context.Context, path string) (domain.AppIdentity, error) {
	env := map[string]string{"PROXY_START_INPUT": path}
	var row shortcutRow
	output, err := runPowerShellJSON(ctx, shortcutScript, env)
	if err != nil {
		return domain.AppIdentity{}, fmt.Errorf("读取快捷方式失败: %w", err)
	}
	if err := json.Unmarshal([]byte(output), &row); err != nil {
		return domain.AppIdentity{}, fmt.Errorf("解析快捷方式失败: %w", err)
	}
	target := canonicalPath(row.TargetPath)
	if target == "" {
		return domain.AppIdentity{}, fmt.Errorf("快捷方式没有目标程序: %s", path)
	}
	if _, err := os.Stat(target); err != nil {
		return domain.AppIdentity{}, fmt.Errorf("快捷方式目标不存在: %s: %w", target, err)
	}
	if !isLaunchableFile(target) {
		return domain.AppIdentity{}, fmt.Errorf("快捷方式目标必须是 .exe 文件，请选择实际程序: %s", target)
	}
	args := splitWindowsCommandLine(row.Arguments)
	workingDir := canonicalPath(row.WorkingDirectory)
	if workingDir == "" {
		workingDir = filepath.Dir(target)
	}
	name := strings.TrimSpace(row.Description)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return domain.AppIdentity{
		Name:             name,
		Executable:       target,
		Arguments:        args,
		WorkingDirectory: workingDir,
	}, nil
}

func (r *Resolver) resolvePackage(ctx context.Context, reference string) (domain.AppIdentity, error) {
	env := map[string]string{"PROXY_START_INPUT": reference}
	output, err := runPowerShellJSON(ctx, packageScript, env)
	if err != nil {
		return domain.AppIdentity{}, fmt.Errorf("解析打包应用 %q 失败: %w", reference, err)
	}
	var row packageRow
	if err := json.Unmarshal([]byte(output), &row); err != nil {
		return domain.AppIdentity{}, fmt.Errorf("解析打包应用清单失败: %w", err)
	}
	if row.Executable == "" {
		return domain.AppIdentity{}, fmt.Errorf("打包应用 %q 没有可执行入口", reference)
	}
	if strings.TrimSpace(row.ApplicationID) == "" || !isLaunchableFile(row.Executable) {
		return domain.AppIdentity{}, fmt.Errorf("打包应用 %q 没有可执行入口", reference)
	}
	return packageRowIdentity(row), nil
}

func directIdentity(path string, args []string, packageFamily string) domain.AppIdentity {
	return domain.AppIdentity{
		Name:             strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Executable:       canonicalPath(path),
		Arguments:        append([]string(nil), args...),
		PackageFamily:    packageFamily,
		WorkingDirectory: filepath.Dir(canonicalPath(path)),
	}
}

func knownBrowserIdentities() []domain.AppIdentity {
	paths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}
	result := make([]domain.AppIdentity, 0, len(paths))
	seen := make(map[string]struct{})
	for _, path := range paths {
		path = canonicalPath(path)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if _, ok := seen[strings.ToLower(path)]; ok {
			continue
		}
		seen[strings.ToLower(path)] = struct{}{}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if strings.EqualFold(name, "chrome") {
			name = "Google Chrome"
		} else if strings.EqualFold(name, "msedge") {
			name = "Microsoft Edge"
		}
		identity := directIdentity(path, nil, "")
		identity.Name = name
		identity.Publisher = browserPublisher(name)
		identity.SystemApp = strings.EqualFold(name, "Microsoft Edge")
		result = append(result, identity)
	}
	return result
}

func browserPublisher(name string) string {
	if strings.EqualFold(strings.TrimSpace(name), "Microsoft Edge") {
		return "Microsoft Corporation"
	}
	if strings.EqualFold(strings.TrimSpace(name), "Google Chrome") {
		return "Google LLC"
	}
	return ""
}

func isPackageReference(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "package:") || strings.Contains(value, "!")
}

func isLaunchableFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe":
		return true
	default:
		return false
	}
}

func friendlyPackageName(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || isResourceDisplayName(value) {
			continue
		}
		return value
	}
	return "应用"
}

func isResourceDisplayName(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "@") || strings.HasPrefix(strings.ToLower(value), "ms-resource:")
}

// firstNonEmpty is shared by the Windows launch helpers when choosing a
// fallback package/application identifier.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "应用"
}

func decodeJSONArray(data string, destination interface{}) error {
	data = strings.TrimSpace(data)
	if data == "" || data == "null" {
		data = "[]"
	}
	if strings.HasPrefix(data, "{") {
		data = "[" + data + "]"
	}
	return json.Unmarshal([]byte(data), destination)
}

const shortcutScript = `$ErrorActionPreference = 'Stop'
$shell = New-Object -ComObject WScript.Shell
$link = $shell.CreateShortcut($env:PROXY_START_INPUT)
[pscustomobject]@{
  TargetPath = [string]$link.TargetPath
  Arguments = [string]$link.Arguments
  WorkingDirectory = [string]$link.WorkingDirectory
  Description = [string]$link.Description
} | ConvertTo-Json -Compress`

const packageScript = `$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$inputValue = [string]$env:PROXY_START_INPUT
$reference = $inputValue
if ($reference.StartsWith('package:', [System.StringComparison]::OrdinalIgnoreCase)) {
  $reference = $reference.Substring(8)
}
$reference = $reference.Trim()
if ([string]::IsNullOrWhiteSpace($reference)) { throw "Package reference is empty: $inputValue" }
$package = $null
$applicationID = $null
$parts = $reference.Split('!', 2)
$packageKey = ([string]$parts[0]).Trim()
if ($parts.Count -gt 1) {
  $applicationID = ([string]$parts[1]).Trim()
  if ([string]::IsNullOrWhiteSpace($applicationID)) { throw "Application ID is empty: $inputValue" }
}
if ([string]::IsNullOrWhiteSpace($packageKey)) { throw "Package name or family is empty: $inputValue" }

# -Name is the fast path for the short package name. The fallback also
# accepts a package family (the form emitted by AUMID) and a full package
# name without relying on a second, version-sensitive cmdlet invocation.
$package = @(Get-AppxPackage -Name $packageKey | Select-Object -First 1)
if ($package.Count -eq 0) {
  $package = @(Get-AppxPackage | Where-Object {
    $_.PackageFamilyName -ieq $packageKey -or
    $_.Name -ieq $packageKey -or
    $_.PackageFullName -ieq $packageKey
  } | Select-Object -First 1)
}
if (-not $package) { throw "Package not found: $inputValue" }
$package = $package[0]
$manifest = Get-AppxPackageManifest -Package $package
$applications = @($manifest.Package.Applications.Application)
if ($applicationID) {
  $application = $applications | Where-Object { $_.Id -ieq $applicationID } | Select-Object -First 1
  if (-not $application) { throw "Application ID not found in package: $inputValue" }
} else {
  # A package reference without !AppId has no application preference. Keep
  # manifest order and choose its first declared entry; an explicit AppId is
  # always required when the caller needs another entry.
  $application = $applications | Select-Object -First 1
}
if (-not $application) { throw "No application entry: $inputValue" }
$visual = $application.VisualElements
$appListEntry = [string]$visual.AppListEntry
if ($appListEntry -ieq 'none') { throw "Application is hidden from the app list: $inputValue" }
$relative = ([string]$application.Executable).Replace('/', [char]92)
$executable = if ([IO.Path]::IsPathRooted($relative)) { $relative } else { Join-Path $package.InstallLocation $relative }
if ([IO.Path]::GetExtension($executable) -ine '.exe' -or -not (Test-Path -LiteralPath $executable -PathType Leaf)) {
  throw "Application entry is not a launchable executable: $inputValue"
}
$packageName = [string]$package.Name
$publisher = [string]$package.Publisher
$installLocation = [string]$package.InstallLocation
$systemApp = ($publisher -match '(?i)Microsoft') -or
  ($packageName -match '^(?i:Microsoft|Windows)\.') -or
  ($installLocation -match '(?i)\\Windows\\SystemApps(?:\\|$)')
$displayName = [string]$visual.DisplayName
if ([string]::IsNullOrWhiteSpace($displayName) -or $displayName.StartsWith('@') -or $displayName.StartsWith('ms-resource:', [System.StringComparison]::OrdinalIgnoreCase)) { $displayName = $packageName }
$versionInfo = $null
try { $versionInfo = [Diagnostics.FileVersionInfo]::GetVersionInfo($executable) } catch {}
$productName = if ($versionInfo) { [string]$versionInfo.ProductName } else { '' }
$fileDescription = if ($versionInfo) { [string]$versionInfo.FileDescription } else { '' }
if (-not [string]::IsNullOrWhiteSpace($productName) -and $productName -notmatch '^(?i:application|app)$') { $displayName = $productName }
elseif (-not [string]::IsNullOrWhiteSpace($fileDescription) -and $fileDescription -notmatch '^(?i:application|app)$') { $displayName = $fileDescription }
[pscustomobject]@{
  Name = $displayName
  DisplayName = $displayName
  Publisher = $publisher
  Executable = $executable
  WorkingDirectory = (Split-Path -Parent $executable)
  PackageName = $packageName
  PackageFamily = [string]$package.PackageFamilyName
  ApplicationId = [string]$application.Id
  AUMID = (([string]$package.PackageFamilyName) + '!' + [string]$application.Id)
  AppListEntry = $appListEntry
  SystemApp = [bool]$systemApp
} | ConvertTo-Json -Compress`

const installedPackagesScript = `$ErrorActionPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
$rows = @()
$startNames = @{}
try {
  foreach ($start in @(Get-StartApps)) {
    $startID = [string]$start.AppID
    $startName = [string]$start.Name
    if (-not [string]::IsNullOrWhiteSpace($startID) -and -not [string]::IsNullOrWhiteSpace($startName)) {
      $startNames[$startID] = $startName
    }
  }
} catch {}
foreach ($package in @(Get-AppxPackage)) {
  try {
    # Framework/resource packages and packages without a user-facing app are
    # implementation details, not launchable applications.
    if ([bool]$package.IsFramework -or [bool]$package.IsResourcePackage) { continue }
    $manifest = Get-AppxPackageManifest -Package $package
    foreach ($application in @($manifest.Package.Applications.Application)) {
      $visual = $application.VisualElements
      $appListEntry = [string]$visual.AppListEntry
      if ($appListEntry -ieq 'none') { continue }
      $applicationID = [string]$application.Id
      if ([string]::IsNullOrWhiteSpace($applicationID)) { continue }
      $relative = ([string]$application.Executable).Replace('/', [char]92)
      $executable = if ([IO.Path]::IsPathRooted($relative)) { $relative } else { Join-Path $package.InstallLocation $relative }
      if ([IO.Path]::GetExtension($executable) -ine '.exe' -or -not (Test-Path -LiteralPath $executable -PathType Leaf)) { continue }
      $aumid = ([string]$package.PackageFamilyName) + '!' + $applicationID
      $packageName = [string]$package.Name
      $displayName = ''
      if ($startNames.ContainsKey($aumid)) { $displayName = [string]$startNames[$aumid] }
      if ([string]::IsNullOrWhiteSpace($displayName) -or $displayName.StartsWith('@') -or $displayName.StartsWith('ms-resource:', [System.StringComparison]::OrdinalIgnoreCase)) { $displayName = [string]$visual.DisplayName }
      if ([string]::IsNullOrWhiteSpace($displayName) -or $displayName.StartsWith('@') -or $displayName.StartsWith('ms-resource:', [System.StringComparison]::OrdinalIgnoreCase)) { $displayName = [string]$package.Name }
      # Get-StartApps is the localized shell name when available. Some
      # desktop-bridge packages are intentionally absent from that list;
      # their version resource is a better product name than Package.Name.
      if (-not $startNames.ContainsKey($aumid) -or $packageName -ieq 'OpenAI.Codex') {
        $versionInfo = $null
        try { $versionInfo = [Diagnostics.FileVersionInfo]::GetVersionInfo($executable) } catch {}
        $productName = if ($versionInfo) { [string]$versionInfo.ProductName } else { '' }
        $fileDescription = if ($versionInfo) { [string]$versionInfo.FileDescription } else { '' }
        if (-not [string]::IsNullOrWhiteSpace($productName) -and $productName -notmatch '^(?i:application|app)$') { $displayName = $productName }
        elseif (-not [string]::IsNullOrWhiteSpace($fileDescription) -and $fileDescription -notmatch '^(?i:application|app)$') { $displayName = $fileDescription }
      }
      $publisher = [string]$package.Publisher
      $installLocation = [string]$package.InstallLocation
      $systemApp = ($publisher -match '(?i)Microsoft') -or
        ($packageName -match '^(?i:Microsoft|Windows)\.') -or
        ($installLocation -match '(?i)\\Windows\\SystemApps(?:\\|$)')
        $rows += [pscustomobject]@{
          Name = $displayName
          DisplayName = $displayName
          Publisher = $publisher
          Executable = $executable
          WorkingDirectory = (Split-Path -Parent $executable)
          PackageName = $packageName
          PackageFamily = [string]$package.PackageFamilyName
          ApplicationId = $applicationID
          AUMID = $aumid
          AppListEntry = $appListEntry
          SystemApp = [bool]$systemApp
        }

    }
  } catch {}
}
# Classic desktop applications exposed by the Start menu are not represented
# by Get-AppxPackage. Keep only concrete .exe entries; URL, shell namespace,
# and GUID-backed Windows management entries are intentionally omitted.
foreach ($start in @($startNames.GetEnumerator())) {
  $startID = [string]$start.Key
  $startName = [string]$start.Value
  if ([string]::IsNullOrWhiteSpace($startID) -or [string]::IsNullOrWhiteSpace($startName)) { continue }
  if ([IO.Path]::GetExtension($startID) -ine '.exe') { continue }
  if (-not [IO.Path]::IsPathRooted($startID) -or -not (Test-Path -LiteralPath $startID -PathType Leaf)) { continue }
  $versionInfo = $null
  try { $versionInfo = [Diagnostics.FileVersionInfo]::GetVersionInfo($startID) } catch {}
  $company = if ($versionInfo) { [string]$versionInfo.CompanyName } else { '' }
  $productName = if ($versionInfo) { [string]$versionInfo.ProductName } else { '' }
  $fileDescription = if ($versionInfo) { [string]$versionInfo.FileDescription } else { '' }
  $publisher = $company
  $systemApp = ($publisher -match '(?i)Microsoft') -or ($startID -match '(?i)^[A-Za-z]:\\Windows(?:\\|$)')
  $displayName = $startName
  if ([string]::IsNullOrWhiteSpace($displayName) -and -not [string]::IsNullOrWhiteSpace($productName)) { $displayName = $productName }
  if ([string]::IsNullOrWhiteSpace($displayName) -and -not [string]::IsNullOrWhiteSpace($fileDescription)) { $displayName = $fileDescription }
  $rows += [pscustomobject]@{
    Name = $displayName
    DisplayName = $displayName
    Publisher = $publisher
    Executable = $startID
    WorkingDirectory = (Split-Path -Parent $startID)
    PackageName = ''
    PackageFamily = ''
    ApplicationId = ''
    AUMID = ''
    AppListEntry = 'start-menu'
    SystemApp = [bool]$systemApp
  }
}
@($rows) | ConvertTo-Json -Compress -Depth 8`
