<#
.SYNOPSIS
  Builds self-contained WittyMediaServer release archives (binary + bundled FFmpeg) from Windows.
.EXAMPLE
  scripts\build-release.ps1 -Version 1.0.0
  scripts\build-release.ps1 -Targets windows/amd64,linux/amd64
#>
param(
  [string]$Version = "",
  [string[]]$Targets = @("windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "freebsd/amd64"),
  [switch]$SkipWeb,
  [switch]$SkipFFmpeg,
  [string]$FFmpegFreeBSDDir = $env:FFMPEG_FREEBSD_DIR
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
if (-not $Version) { try { $Version = (git describe --tags --always --dirty 2>$null) } catch { } ; if (-not $Version) { $Version = "dev" } }
$dist = "dist"; $cache = ".tools\ffmpeg-dl"
New-Item -ItemType Directory -Force $dist, $cache | Out-Null

if (-not $SkipWeb) {
  Write-Host "==> building web client"
  Push-Location web
  & npm.cmd ci --no-audit --no-fund; if ($LASTEXITCODE -ne 0) { & npm.cmd install --no-audit --no-fund }
  & npm.cmd run build; if ($LASTEXITCODE -ne 0) { throw "web build failed" }
  Pop-Location
}

function Fetch($url, $out) {
  if ((Test-Path $out) -and (Get-Item $out).Length -gt 0) { return }
  for ($i = 1; $i -le 4; $i++) {
    try { Invoke-WebRequest -Uri $url -OutFile "$out.part" -UseBasicParsing -UserAgent wms-build; Move-Item -Force "$out.part" $out; return } catch { Start-Sleep ($i * 3) }
  }
  throw "download failed: $url"
}
function Expand($zip, $dir) { if (-not (Test-Path $dir)) { Expand-Archive -Path $zip -DestinationPath $dir -Force } }
function ExpandTarXz($tar, $dir) { if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Force $dir | Out-Null; & tar -xJf $tar -C $dir; if ($LASTEXITCODE -ne 0) { throw "tar failed" } } }

function Bundle-FFmpeg($dir, $os, $arch) {
  $bin = Join-Path $dir "bin"; New-Item -ItemType Directory -Force $bin | Out-Null
  $btbn = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest"
  switch ("$os/$arch") {
    "windows/amd64" { Fetch "$btbn/ffmpeg-master-latest-win64-gpl.zip" "$cache\win64.zip"; Expand "$cache\win64.zip" "$cache\win64"; Copy-Item "$cache\win64\*\bin\ffmpeg.exe", "$cache\win64\*\bin\ffprobe.exe" $bin }
    "windows/arm64" { Fetch "$btbn/ffmpeg-master-latest-winarm64-gpl.zip" "$cache\winarm64.zip"; Expand "$cache\winarm64.zip" "$cache\winarm64"; Copy-Item "$cache\winarm64\*\bin\ffmpeg.exe", "$cache\winarm64\*\bin\ffprobe.exe" $bin }
    "linux/amd64" { Fetch "$btbn/ffmpeg-master-latest-linux64-gpl.tar.xz" "$cache\linux64.tar.xz"; ExpandTarXz "$cache\linux64.tar.xz" "$cache\linux64"; Copy-Item "$cache\linux64\*\bin\ffmpeg", "$cache\linux64\*\bin\ffprobe" $bin }
    "linux/arm64" { Fetch "$btbn/ffmpeg-master-latest-linuxarm64-gpl.tar.xz" "$cache\linuxarm64.tar.xz"; ExpandTarXz "$cache\linuxarm64.tar.xz" "$cache\linuxarm64"; Copy-Item "$cache\linuxarm64\*\bin\ffmpeg", "$cache\linuxarm64\*\bin\ffprobe" $bin }
    { $_ -like "darwin/*" } {
      Fetch "https://ffmpeg.martin-riedl.de/redirect/latest/macos/$arch/release/ffmpeg.zip" "$cache\mac-$arch-ffmpeg.zip"
      Fetch "https://ffmpeg.martin-riedl.de/redirect/latest/macos/$arch/release/ffprobe.zip" "$cache\mac-$arch-ffprobe.zip"
      Expand-Archive "$cache\mac-$arch-ffmpeg.zip" $bin -Force; Expand-Archive "$cache\mac-$arch-ffprobe.zip" $bin -Force
    }
    { $_ -like "freebsd/*" } {
      if ($FFmpegFreeBSDDir -and (Test-Path "$FFmpegFreeBSDDir\ffmpeg")) { Copy-Item "$FFmpegFreeBSDDir\ffmpeg", "$FFmpegFreeBSDDir\ffprobe" $bin }
      else { Write-Warning "no FreeBSD ffmpeg provided (see scripts/build-ffmpeg-static.sh); wms will look for ffmpeg on PATH" }
    }
  }
}

foreach ($t in $Targets) {
  $os, $arch = $t.Split("/")
  $name = "wittymediaserver-$Version-$os-$arch"; $out = Join-Path $dist $name
  if (Test-Path $out) { Remove-Item -Recurse -Force $out }
  New-Item -ItemType Directory -Force $out | Out-Null
  Write-Host "==> $t"
  $ext = if ($os -eq "windows") { ".exe" } else { "" }
  $env:CGO_ENABLED = "0"; $env:GOOS = $os; $env:GOARCH = $arch
  & go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $out "wms$ext") ./cmd/wms
  if ($LASTEXITCODE -ne 0) { throw "go build failed for $t" }
  if (-not $SkipFFmpeg) { Bundle-FFmpeg $out $os $arch }
  foreach ($f in @("README.md", "LICENSE", "THIRD_PARTY_LICENSES.md")) { if (Test-Path $f) { Copy-Item $f $out } }
  if ($os -eq "windows") { Copy-Item scripts\install.ps1, scripts\uninstall.ps1, scripts\start-wms.cmd $out } else { Copy-Item scripts\install.sh $out }
  if ($os -eq "windows") { Compress-Archive -Path $out -DestinationPath "$out.zip" -Force; Write-Host "   -> $out.zip" }
  else { & tar -czf "$out.tar.gz" -C $dist $name; Write-Host "   -> $out.tar.gz" }
}
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
Write-Host "done."
