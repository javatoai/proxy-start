//go:build windows

package windows

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"proxy-start/internal/adapters/native"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
	"strings"
)

type Resolver struct{}

var _ application.CodexResolver = (*Resolver)(nil)

func NewResolver() *Resolver { return &Resolver{} }

func (*Resolver) Discover(ctx context.Context) (domain.AppIdentity, error) {
	output, err := runPowerShellJSON(ctx, codexPackageScript, nil)
	if err != nil {
		return domain.AppIdentity{}, err
	}
	if output == "" || output == "null" {
		return domain.AppIdentity{}, application.ErrNotFound
	}
	var identity domain.AppIdentity
	if err := json.Unmarshal([]byte(output), &identity); err != nil {
		return identity, err
	}
	identity.Executable = canonicalPath(identity.Executable)
	identity.WorkingDirectory = filepath.Dir(identity.Executable)
	if !native.IsCodex(identity) {
		return domain.AppIdentity{}, errors.New("Codex 安装包未找到受支持的桌面入口")
	}
	return identity, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

const codexPackageScript = `$ErrorActionPreference = 'Stop'
$package = Get-AppxPackage -Name OpenAI.Codex | Sort-Object Version -Descending | Select-Object -First 1
if (-not $package) { 'null'; exit }
$manifest = Get-AppxPackageManifest -Package $package
foreach ($application in @($manifest.Package.Applications.Application)) {
  $relative = ([string]$application.Executable).Replace('/', [char]92)
  $entry = Join-Path $package.InstallLocation $relative
  $basename = [IO.Path]::GetFileName($entry)
  if ($basename -notin @('Codex.exe','ChatGPT.exe','Codex-Code-Mode-Host.exe')) { continue }
  if ([string]$application.VisualElements.AppListEntry -ieq 'none') { continue }
  if (-not (Test-Path -LiteralPath $entry -PathType Leaf)) { continue }
  [pscustomobject]@{
    name = 'Codex'
    executable = $entry
    packageName = [string]$package.Name
    packageFamily = [string]$package.PackageFamilyName
    applicationId = [string]$application.Id
    aumid = ([string]$package.PackageFamilyName) + '!' + [string]$application.Id
  } | ConvertTo-Json -Compress
  exit
}
'null'`
