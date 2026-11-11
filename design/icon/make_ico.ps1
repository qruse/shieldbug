# Renders assets/icon*.svg with headless Edge and packs a multi-size assets/icon.ico.
# 16-48 px use icon-small.svg, larger sizes use icon.svg.
$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\..\.."
$edge = "C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
$tmp = Join-Path $env:TEMP "sb_ico"
New-Item -ItemType Directory -Force $tmp | Out-Null
Add-Type -AssemblyName System.Drawing

function Render([string]$svgFile, [int]$px, [string]$outPng) {
    $svg = [IO.File]::ReadAllText($svgFile) -replace '<svg ', "<svg width='$px' height='$px' style='display:block' "
    $html = Join-Path $tmp "r$px.html"
    "<html><body style='margin:0;background:transparent'>$svg</body></html>" | Out-File -Encoding utf8 $html
    $w = [Math]::Max($px, 100)
    $raw = Join-Path $tmp "raw$px.png"
    if (Test-Path $raw) { Remove-Item $raw }
    $url = "file:///" + ($html -replace '\\', '/')
    $p = Start-Process -FilePath $edge -PassThru -Wait -WindowStyle Hidden -ArgumentList @(
        "--headless=new", "--disable-gpu", "--hide-scrollbars", "--default-background-color=00000000",
        "--user-data-dir=$tmp\profile", "--window-size=$w,$w", "--screenshot=$raw", $url)
    $src = [Drawing.Bitmap]::FromFile($raw)
    $bmp = New-Object Drawing.Bitmap $px, $px, ([Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g = [Drawing.Graphics]::FromImage($bmp)
    $r = New-Object Drawing.Rectangle 0, 0, $px, $px
    $g.DrawImage($src, $r, $r, [Drawing.GraphicsUnit]::Pixel)
    $g.Dispose(); $src.Dispose()
    $bmp.Save($outPng, [Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
}

$sizes = 16, 24, 32, 48, 64, 128, 256
$pngs = @()
foreach ($px in $sizes) {
    $svgFile = if ($px -le 48) { "$root\assets\icon-small.svg" } else { "$root\assets\icon.svg" }
    $out = Join-Path $tmp "icon$px.png"
    Render $svgFile $px $out
    $pngs += , [IO.File]::ReadAllBytes($out)
}
Copy-Item (Join-Path $tmp "icon256.png") "$root\assets\icon-256.png" -Force

# ICO with PNG-compressed entries (supported since Windows Vista).
$ms = New-Object IO.MemoryStream
$bw = New-Object IO.BinaryWriter $ms
$bw.Write([UInt16]0); $bw.Write([UInt16]1); $bw.Write([UInt16]$sizes.Count)
$off = 6 + 16 * $sizes.Count
for ($i = 0; $i -lt $sizes.Count; $i++) {
    $d = if ($sizes[$i] -ge 256) { 0 } else { $sizes[$i] }
    $bw.Write([byte]$d); $bw.Write([byte]$d); $bw.Write([byte]0); $bw.Write([byte]0)
    $bw.Write([UInt16]1); $bw.Write([UInt16]32)
    $bw.Write([UInt32]$pngs[$i].Length); $bw.Write([UInt32]$off)
    $off += $pngs[$i].Length
}
foreach ($p in $pngs) { $bw.Write($p) }
$bw.Flush()
[IO.File]::WriteAllBytes("$root\assets\icon.ico", $ms.ToArray())
"wrote assets\icon.ico ($($ms.Length) bytes, sizes $($sizes -join ','))"
