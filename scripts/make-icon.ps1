# Generates the project-drawn app icon (MIT) as a multi-size PNG-compressed .ico.
# No external assets are used; the mark is drawn procedurally.
#
#   powershell -ExecutionPolicy Bypass -File scripts/make-icon.ps1
#
param(
  [string]$Out = (Join-Path $PSScriptRoot "..\desktop\src\TavernAgent.Desktop\Assets\app.ico")
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

function New-RoundedPath([int]$w, [int]$h, [double]$radius) {
  $p = New-Object System.Drawing.Drawing2D.GraphicsPath
  $d = $radius * 2
  $p.AddArc(0, 0, $d, $d, 180, 90)
  $p.AddArc($w - $d, 0, $d, $d, 270, 90)
  $p.AddArc($w - $d, $h - $d, $d, $d, 0, 90)
  $p.AddArc(0, $h - $d, $d, $d, 90, 90)
  $p.CloseFigure()
  return $p
}

function New-IconPng([int]$size) {
  $bmp = New-Object System.Drawing.Bitmap($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
  $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
  $g.Clear([System.Drawing.Color]::Transparent)

  $pad = [Math]::Max(1, [int]($size * 0.06))
  $inner = $size - ($pad * 2)
  $path = New-RoundedPath $inner $inner ($inner * 0.28)

  # Dark base with a subtle vertical gradient.
  $rect = New-Object System.Drawing.Rectangle(0, 0, $inner, $inner)
  $brush = New-Object System.Drawing.Drawing2D.LinearGradientBrush(
    $rect,
    [System.Drawing.Color]::FromArgb(255, 26, 28, 36),
    [System.Drawing.Color]::FromArgb(255, 12, 13, 18),
    [System.Drawing.Drawing2D.LinearGradientMode]::Vertical)
  $g.TranslateTransform($pad, $pad)
  $g.FillPath($brush, $path)

  # Accent ring
  $ringPen = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(255, 120, 150, 255), [Math]::Max(1.0, $inner * 0.045))
  $ringRect = New-Object System.Drawing.RectangleF(($inner * 0.20), ($inner * 0.20), ($inner * 0.60), ($inner * 0.60))
  $g.DrawEllipse($ringPen, $ringRect)

  # Center spark (diamond)
  $cx = $inner / 2.0
  $r = $inner * 0.16
  $pts = @(
    (New-Object System.Drawing.PointF($cx, ($cx - $r))),
    (New-Object System.Drawing.PointF(($cx + $r), $cx)),
    (New-Object System.Drawing.PointF($cx, ($cx + $r))),
    (New-Object System.Drawing.PointF(($cx - $r), $cx))
  )
  $spark = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 226, 232, 250))
  $g.FillPolygon($spark, $pts)

  # Small accent dot
  $dot = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 110, 231, 183))
  $dr = $inner * 0.07
  $g.FillEllipse($dot, ($inner * 0.66), ($inner * 0.66), ($dr * 2), ($dr * 2))

  $g.ResetTransform()
  $g.Dispose()
  return $bmp
}

$sizes = @(16, 24, 32, 48, 64, 128, 256)
$pngs = @()
foreach ($s in $sizes) {
  $bmp = New-IconPng $s
  $ms = New-Object System.IO.MemoryStream
  $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
  $pngs += ,@{ Size = $s; Bytes = $ms.ToArray() }
  $ms.Dispose()
  $bmp.Dispose()
}

$dir = Split-Path -Parent $Out
if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir | Out-Null }

$fs = [System.IO.File]::Create($Out)
$bw = New-Object System.IO.BinaryWriter($fs)
try {
  $bw.Write([UInt16]0)                 # reserved
  $bw.Write([UInt16]1)                 # type = icon
  $bw.Write([UInt16]$pngs.Count)       # count

  $offset = 6 + (16 * $pngs.Count)
  foreach ($e in $pngs) {
    $dim = if ($e.Size -ge 256) { 0 } else { $e.Size }
    $bw.Write([Byte]$dim)              # width
    $bw.Write([Byte]$dim)              # height
    $bw.Write([Byte]0)                 # color count
    $bw.Write([Byte]0)                 # reserved
    $bw.Write([UInt16]1)               # planes
    $bw.Write([UInt16]32)              # bit count
    $bw.Write([UInt32]$e.Bytes.Length) # size
    $bw.Write([UInt32]$offset)         # offset
    $offset += $e.Bytes.Length
  }
  foreach ($e in $pngs) { $bw.Write($e.Bytes) }
}
finally {
  $bw.Dispose()
  $fs.Dispose()
}

Write-Host "Wrote $Out ($((Get-Item $Out).Length) bytes, $($pngs.Count) sizes)"
