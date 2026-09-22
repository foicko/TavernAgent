# Dev launcher for the standalone UI Component Showcase.
# Runs the independent Vite dev server on http://localhost:5174 showcasing
# components from beautifului.dev, beui.dev, rareui.com, transitions.dev, and ui.shadcn.com.
#
# Usage:
#   ./scripts/dev-showcase.ps1

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$showcaseDir = Join-Path $root "showcase"

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "  TavernAgent Standalone UI Component Showcase" -ForegroundColor White
Write-Host "  Libraries: beautifului, beui, rareui, transitions, shadcn" -ForegroundColor Gray
Write-Host "  URL:       http://localhost:5174" -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan

Set-Location $showcaseDir
& pnpm run dev
