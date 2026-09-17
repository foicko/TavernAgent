# Build a single executable with the current frontend embedded.
[CmdletBinding()]
param(
  [string]$Version = "dev",
  [string]$OutputDirectory = "build",
  [switch]$SkipInstall,
  [switch]$SkipChecks
)
$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
if ($Version -notmatch '^[A-Za-z0-9._+-]+$') { throw "Version must contain only letters, digits, dot, underscore, plus or dash." }

function Invoke-Checked([string]$Program, [string[]]$Arguments) {
  & $Program @Arguments
  if ($LASTEXITCODE -ne 0) { throw "$Program failed with exit code $LASTEXITCODE" }
}

Push-Location (Join-Path $projectRoot "web")
try {
  if (-not $SkipInstall) { Invoke-Checked "pnpm" @("install", "--frozen-lockfile") }
  if (-not $SkipChecks) {
    Invoke-Checked "pnpm" @("test")
    Invoke-Checked "pnpm" @("lint")
  }
  Invoke-Checked "pnpm" @("build")
} finally { Pop-Location }

Push-Location $projectRoot
try {
  if (-not $SkipChecks) {
    Invoke-Checked "go" @("test", "-count=1", "./...")
    Invoke-Checked "go" @("vet", "./...")
  }
  $commit = (& git rev-parse --short=12 HEAD).Trim()
  if ($LASTEXITCODE -ne 0) { throw "Unable to determine build commit." }
  $changes = & git status --porcelain
  if ($LASTEXITCODE -ne 0) { throw "Unable to determine working tree status." }
  if ($changes) { $commit += "-dirty" }
  $builtAt = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
  $targetDir = [IO.Path]::GetFullPath((Join-Path $projectRoot $OutputDirectory))
  [IO.Directory]::CreateDirectory($targetDir) | Out-Null
  $fileName = if ((& go env GOOS) -eq "windows") { "tavernagent.exe" } else { "tavernagent" }
  $executable = Join-Path $targetDir $fileName
  $flags = "-X main.appVersion=tavernagent/$Version -X main.buildCommit=$commit -X main.buildTime=$builtAt"
  Invoke-Checked "go" @("build", "-trimpath", "-ldflags", $flags, "-o", $executable, "./cmd/tavernagent")
  $metadata = [ordered]@{ version = $Version; commit = $commit; builtAt = $builtAt; sha256 = (Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash; executable = $fileName }
  $metadata | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $targetDir "BUILD-INFO.json") -Encoding UTF8
  Write-Host "Built $executable"
} finally { Pop-Location }
