<#
.SYNOPSIS
  Builds self-contained WittyMediaServer release archives for every supported platform from Windows.
.DESCRIPTION
  A thin wrapper around scripts/build-release.sh (the single source of truth for targets, FFmpeg
  sources and packaging), run with Git for Windows' bash. Install Git for Windows, Go and Node.js.
.EXAMPLE
  scripts\build-release.ps1 -Version 0.4.0-alpha
  scripts\build-release.ps1 -Version 0.4.0-alpha -Targets windows/amd64,linux/arm64
  scripts\build-release.ps1 -SkipFFmpeg
#>
param(
  [string]$Version = "",
  [string[]]$Targets = @(),
  [switch]$SkipWeb,
  [switch]$SkipFFmpeg
)
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

# Find Git for Windows' bash (not the WSL launcher in System32).
$bash = $null
foreach ($p in @("$env:ProgramFiles\Git\bin\bash.exe", "${env:ProgramFiles(x86)}\Git\bin\bash.exe", "$env:LOCALAPPDATA\Programs\Git\bin\bash.exe")) {
  if ($p -and (Test-Path $p)) { $bash = $p; break }
}
if (-not $bash) {
  $git = (Get-Command git.exe -ErrorAction SilentlyContinue).Source
  if ($git) { $cand = Join-Path (Split-Path (Split-Path $git)) "bin\bash.exe"; if (Test-Path $cand) { $bash = $cand } }
}
if (-not $bash) { throw "Git for Windows (bash) is required: https://git-scm.com/download/win" }

if ($SkipWeb) { $env:SKIP_WEB = "1" }
if ($SkipFFmpeg) { $env:SKIP_FFMPEG = "1" }
$bashArgs = @("scripts/build-release.sh")
if ($Version -or $Targets.Count -gt 0) { $bashArgs += $(if ($Version) { $Version } else { "" }) }
$bashArgs += $Targets
& $bash @bashArgs
if ($LASTEXITCODE -ne 0) { throw "build failed" }
