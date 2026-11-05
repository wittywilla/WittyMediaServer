<#
.SYNOPSIS
  Makes the Windows installer (.msi) from a folder scripts/build-release.sh built.
.DESCRIPTION
  Needs the WiX Toolset, on the PATH or at $env:WIX_EXE, with its three extensions:
    dotnet tool install --tool-path <folder> wix --version 6.0.2
    wix extension add -g WixToolset.UI.wixext/6.0.2 WixToolset.Firewall.wixext/6.0.2 WixToolset.Util.wixext/6.0.2
.EXAMPLE
  packaging\windows\build-msi.ps1 -Version 0.6.0-alpha -PackageDir dist\wittymediaserver-0.6.0-alpha-windows-amd64 -Arch amd64 -OutDir dist
#>
param(
  [Parameter(Mandatory)][string]$Version,
  [Parameter(Mandatory)][string]$PackageDir,
  [ValidateSet("amd64", "arm64")][string]$Arch = "amd64",
  [string]$OutDir = "dist"
)
$ErrorActionPreference = "Stop"
$here = $PSScriptRoot
$root = Resolve-Path (Join-Path $here "..\..")
$wix = if ($env:WIX_EXE) { $env:WIX_EXE } elseif (Get-Command wix -ErrorAction SilentlyContinue) { "wix" } else { throw "The WiX Toolset is not installed: see the top of this script." }
$PackageDir = (Resolve-Path $PackageDir).Path
New-Item -ItemType Directory -Force $OutDir | Out-Null
$OutDir = (Resolve-Path $OutDir).Path

# Windows Installer versions are three numbers; what's after them is for people to read.
$numeric = ($Version -replace '^(\d+\.\d+\.\d+).*$', '$1')
if ($numeric -notmatch '^\d+\.\d+\.\d+$') { $numeric = "0.0.0" }

$work = Join-Path ([IO.Path]::GetTempPath()) ("wms-msi-" + [Guid]::NewGuid())
$pkg = Join-Path $work "package"
$res = Join-Path $work "resources"
New-Item -ItemType Directory -Force $pkg, $res | Out-Null
try {
  Copy-Item -Recurse -Force (Join-Path $PackageDir "*") $pkg
  # The forwarder is only for an update from 0.5.2-alpha inside an unpacked folder.
  Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $pkg "wms.exe")
  # So the server knows who installed it.
  [IO.File]::WriteAllText((Join-Path $pkg "package-info.json"), '{"product":"WittyMediaServer","kind":"msi"}' + "`n")

  Push-Location $root
  try {
    $env:GOOS = $null; $env:GOARCH = $null
    go run ./scripts/genicon (Join-Path $res "icon.ico")
    go run ./scripts/genicon -bitmaps $res
    if ($LASTEXITCODE -ne 0) { throw "making the installer's pictures failed" }
    # A package from before 0.7.0 has no settings.ini beside the program.
    if (-not (Test-Path (Join-Path $pkg "settings.ini"))) {
      go run ./scripts/settingsini -crlf (Join-Path $pkg "settings.ini")
      if ($LASTEXITCODE -ne 0) { throw "writing settings.ini failed" }
    }
  } finally { Pop-Location }

  $hasModules = if (Test-Path (Join-Path $pkg "modules")) { "1" } else { "0" }
  $wixArch = if ($Arch -eq "arm64") { "arm64" } else { "x64" }
  $out = Join-Path $OutDir "wittymediaserver-$Version-windows-$Arch.msi"
  & $wix build `
    (Join-Path $here "WittyMediaServer.wxs") (Join-Path $here "WittyUI.wxs") `
    -arch $wixArch `
    -d "Version=$numeric" -d "DisplayVersion=$Version" -d "HasModules=$hasModules" `
    -bindpath "pkg=$pkg" -bindpath "res=$res" `
    -ext WixToolset.UI.wixext -ext WixToolset.Firewall.wixext -ext WixToolset.Util.wixext `
    -o $out
  if ($LASTEXITCODE -ne 0) { throw "wix build failed" }
  Remove-Item -Force -ErrorAction SilentlyContinue ($out -replace '\.msi$', '.wixpdb')
  Write-Host "   -> $out"
} finally {
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $work
}
