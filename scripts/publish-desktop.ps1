# Publishes the Avalonia desktop app as a self-contained build and bundles the
# Go sidecar next to it (in a sidecar/ subfolder, matching DesktopPaths lookup).
#
#   powershell -ExecutionPolicy Bypass -File scripts/publish-desktop.ps1
#   powershell -ExecutionPolicy Bypass -File scripts/publish-desktop.ps1 -Rid osx-arm64 -Zip
#
# Note: cross-RID publish is supported for the .NET app; the Go sidecar is built
# with CGO disabled so it cross-compiles cleanly. Only win-x64 is validated in CI today.
param(
  [string]$Rid = "win-x64",
  [string]$Configuration = "Release",
  [string]$Version = "0.1.0",
  [switch]$Zip,
  [switch]$SkipSidecar
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$proj = Join-Path $root "desktop\src\TavernAgent.Desktop\TavernAgent.Desktop.csproj"
$outDir = Join-Path $root "dist\desktop-$Rid"

$ridMap = @{
  "win-x64"    = @{ GOOS = "windows"; GOARCH = "amd64"; Name = "tavernagent.exe" }
  "win-arm64"  = @{ GOOS = "windows"; GOARCH = "arm64"; Name = "tavernagent.exe" }
  "osx-x64"    = @{ GOOS = "darwin";  GOARCH = "amd64"; Name = "tavernagent" }
  "osx-arm64"  = @{ GOOS = "darwin";  GOARCH = "arm64"; Name = "tavernagent" }
  "linux-x64"  = @{ GOOS = "linux";   GOARCH = "amd64"; Name = "tavernagent" }
  "linux-arm64"= @{ GOOS = "linux";   GOARCH = "arm64"; Name = "tavernagent" }
}
if (-not $ridMap.ContainsKey($Rid)) { throw "Unsupported -Rid '$Rid'. Known: $($ridMap.Keys -join ', ')" }
$map = $ridMap[$Rid]

if (Test-Path $outDir) { Remove-Item -Recurse -Force $outDir }
New-Item -ItemType Directory -Path $outDir | Out-Null

Write-Host "==> dotnet publish ($Rid, self-contained, v$Version)"
& dotnet publish $proj `
  -c $Configuration `
  -r $Rid `
  --self-contained true `
  -p:Version=$Version `
  -p:PublishTrimmed=false `
  -p:DebugType=none `
  -o $outDir
if ($LASTEXITCODE -ne 0) { throw "dotnet publish failed" }

if (-not $SkipSidecar) {
  Write-Host "==> building Go sidecar ($($map.GOOS)/$($map.GOARCH))"
  $sidecarDir = Join-Path $outDir "sidecar"
  New-Item -ItemType Directory -Path $sidecarDir | Out-Null
  $sidecarOut = Join-Path $sidecarDir $map.Name

  $prevGoos = $env:GOOS; $prevGoarch = $env:GOARCH; $prevCgo = $env:CGO_ENABLED
  try {
    $env:GOOS = $map.GOOS
    $env:GOARCH = $map.GOARCH
    $env:CGO_ENABLED = "0"
    & go build -trimpath -ldflags "-s -w" -o $sidecarOut (Join-Path $root "cmd\tavernagent")
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
  }
  finally {
    $env:GOOS = $prevGoos; $env:GOARCH = $prevGoarch; $env:CGO_ENABLED = $prevCgo
  }
  Write-Host "    sidecar -> $sidecarOut"
}

# Copy license / notice files that must travel with the release.
foreach ($doc in @("THIRD_PARTY_NOTICES.md", "LICENSE", "LICENSE.md")) {
  $src = Join-Path $root $doc
  if (Test-Path $src) { Copy-Item $src $outDir -Force }
}

if ($Zip) {
  $zipPath = Join-Path $root "dist\TavernAgent-desktop-$Rid-$Version.zip"
  if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
  Write-Host "==> zipping -> $zipPath"
  Compress-Archive -Path (Join-Path $outDir "*") -DestinationPath $zipPath
}

Write-Host ""
Write-Host "Published to: $outDir"
$exeName = if ($Rid -like "win-*") { "TavernAgent.Desktop.exe" } else { "TavernAgent.Desktop" }
Write-Host "Run:          $(Join-Path $outDir $exeName)"
