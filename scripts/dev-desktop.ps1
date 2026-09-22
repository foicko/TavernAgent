# Dev launcher for the Avalonia desktop shell.
# Builds the Go headless sidecar into build/ (if missing or -Rebuild), then runs
# the Avalonia app which discovers build/tavernagent.exe via walk-up search.
#
#   ./scripts/dev-desktop.ps1              # real providers (configure in-app)
#   ./scripts/dev-desktop.ps1 -Mock        # offline mock provider
#   ./scripts/dev-desktop.ps1 -Rebuild     # force rebuild the Go sidecar
param(
  [switch]$Mock,
  [switch]$Rebuild
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$sidecar = Join-Path $root "build\tavernagent.exe"

if ($Rebuild -or -not (Test-Path $sidecar)) {
  Write-Host "Building Go sidecar -> build/tavernagent.exe"
  & go build -trimpath -o $sidecar (Join-Path $root "cmd\tavernagent")
  if ($LASTEXITCODE -ne 0) { throw "go build failed" }
}

$env:TAVERNAGENT_SIDECAR = $sidecar
$env:TAVERNAGENT_PROVIDER = if ($Mock) { "mock" } else { "" }

Write-Host "Launching Avalonia desktop (TAVERNAGENT_SIDECAR=$sidecar)"
& dotnet run --project (Join-Path $root "desktop\src\TavernAgent.Desktop\TavernAgent.Desktop.csproj")
