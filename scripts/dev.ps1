# 一键开发启动：后端无头服务 + Vite 前端（Windows PowerShell）
# 用法：powershell -ExecutionPolicy Bypass -File scripts/dev.ps1
$ErrorActionPreference = "Stop"
$projectRoot = Split-Path -Parent $PSScriptRoot
$devDir = Join-Path $projectRoot "build/dev"
[IO.Directory]::CreateDirectory($devDir) | Out-Null
$executable = Join-Path $devDir "tavernagent.exe"
$devDataDir = Join-Path $devDir "data"

# web/dist 是生成物且被 //go:embed 嵌入：缺了它 Go 直接编译失败，所以先构建前端。
Push-Location (Join-Path $projectRoot "web")
try {
  pnpm build
  if ($LASTEXITCODE -ne 0) { throw "前端构建失败（pnpm build）" }
} finally { Pop-Location }

Push-Location $projectRoot
try {
  go build -o $executable ./cmd/tavernagent
  if ($LASTEXITCODE -ne 0) { throw "后端构建失败" }
} finally { Pop-Location }

Write-Host "==> [1/2] 启动 SillyDog 后端（127.0.0.1:8890，provider=mock）" -ForegroundColor Cyan
$backend = Start-Process -FilePath $executable -ArgumentList @("-addr", "127.0.0.1:8890", "-data", $devDataDir, "-provider", "mock", "-allow-origin", "http://localhost:5173,http://127.0.0.1:5173") `
  -WorkingDirectory $projectRoot -PassThru -WindowStyle Hidden `
  -RedirectStandardOutput (Join-Path $devDir "backend.stdout.log") -RedirectStandardError (Join-Path $devDir "backend.stderr.log")

Start-Sleep -Milliseconds 1200
if ($backend.HasExited) { throw "后端未启动，请查看 build/dev/backend.stderr.log" }

Write-Host "==> [2/2] 启动 Vite 前端（http://localhost:5173）" -ForegroundColor Cyan
Push-Location (Join-Path $projectRoot "web")
try {
  pnpm dev
} finally {
  Pop-Location
  if (-not $backend.HasExited) { Stop-Process -Id $backend.Id -Force }
}
