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

# 桌面端必须是 GUI 子系统：控制台子系统（默认）双击启动会先弹一个空白终端窗口。
# 这条只能靠构建参数保证（-H windowsgui），而参数很容易在改动中被丢掉，
# 所以构建完直接读 PE 头的 Subsystem 字段自检（2 = GUI，3 = 控制台）。
function Assert-WindowsGuiSubsystem([string]$Path) {
  $stream = [IO.File]::OpenRead($Path)
  try {
    $header = New-Object byte[] 4096
    [void]$stream.Read($header, 0, $header.Length)
  } finally { $stream.Dispose() }
  $peOffset = [BitConverter]::ToInt32($header, 0x3C)
  if ($peOffset -le 0 -or $peOffset + 92 -ge $header.Length) { throw "$Path 不是可解析的 PE 文件。" }
  $subsystem = [BitConverter]::ToUInt16($header, $peOffset + 4 + 20 + 68)
  if ($subsystem -ne 2) {
    throw "$Path 的 PE 子系统是 $subsystem（应为 2 = WINDOWS_GUI）：双击启动会多弹一个终端窗口，请确认 -ldflags 里带上了 -H windowsgui。"
  }
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
