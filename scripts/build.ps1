# Build a single executable with the current frontend embedded.
[CmdletBinding()]
param(
  [string]$Version = "dev",
  [string]$OutputDirectory = "build",
  [switch]$SkipInstall,
  [switch]$SkipChecks,
  [switch]$Desktop
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

  if ($Desktop) {
    if ((& go env GOOS) -ne "windows") { throw "桌面壳（-Desktop）目前仅支持 Windows；其他平台请构建无头服务。" }
    $desktopExe = Join-Path $targetDir "tavernagent-desktop.exe"
    # 先生成图标/清单/版本资源（.syso），再构建；Wails 必须带 production/dev 标签，
    # 缺标签时它编译成“弹框报错”的变体，窗口根本不渲染。
    Invoke-Checked "go" @("run", "./cmd/winresgen", "-version", "tavernagent/$Version")
    Invoke-Checked "go" @("build", "-trimpath", "-tags", "desktop,production", "-ldflags", $flags, "-o", $desktopExe, "./cmd/tavernagent-desktop")
    Write-Host "Built $desktopExe"
  }
} finally { Pop-Location }
