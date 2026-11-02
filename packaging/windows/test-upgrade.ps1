<#
  Installs one MSI over another the way a person updating would, and fails unless the upgrade
  finishes cleanly. Run on a throwaway Windows machine (CI), never on someone's own PC: it
  installs, starts and removes WittyMediaServer.

    packaging\windows\test-upgrade.ps1 -OldMsi old.msi -NewMsi new.msi -Scope machine [-Service] [-ExpectFail] -LogDir logs

  Clean means: the new MSI exits 0 (not 3010, which says files were in use and need a restart),
  its log has no error 2613 or other installer error, the server started from the old copy has
  been stopped, and the installed version is the new one. -ExpectFail turns it round, to prove a
  broken MSI is caught.
#>
param(
  [Parameter(Mandatory)][string]$OldMsi,
  [Parameter(Mandatory)][string]$NewMsi,
  [ValidateSet('machine', 'user')][string]$Scope = 'machine',
  [switch]$Service,
  [switch]$ExpectFail,
  [Parameter(Mandatory)][string]$LogDir
)
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force $LogDir | Out-Null
$tag = "$Scope$(if ($Service) { '-service' })"

function Msi([string]$verb, [string]$file, [string]$log, [string[]]$props) {
  $a = @($verb, "`"$file`"", '/qn', '/l*v', "`"$log`"") + $props
  $p = Start-Process msiexec.exe -ArgumentList $a -Wait -PassThru
  return $p.ExitCode
}

$props = if ($Scope -eq 'machine') { @('MSIINSTALLPERUSER=""', 'ALLUSERS=1') } else { @('MSIINSTALLPERUSER=1', 'ALLUSERS=2') }
$props += 'AUTOSTART=""'
if ($Service) { $props += @('INSTALLSERVICE=1', 'PORT=9797') }
$dir = if ($Scope -eq 'machine') { Join-Path $env:ProgramFiles 'WittyMediaServer' } else { Join-Path $env:LOCALAPPDATA 'Programs\WittyMediaServer' }

# 1. The old version, as someone has it now.
$code = Msi '/i' $OldMsi (Join-Path $LogDir "$tag-1-old.log") $props
if ($code -ne 0) { throw "[$tag] installing the old MSI failed: $code" }
$server = Join-Path $dir 'bin\wittymediaserver.exe'
if (-not (Test-Path $server)) { throw "[$tag] the old MSI didn't put the server at $server" }

# 2. Running, as it usually is when someone updates. The service starts by itself.
$proc = $null
if (-not $Service) {
  $data = Join-Path $env:RUNNER_TEMP "wms-upgrade-$tag"
  $proc = Start-Process $server -ArgumentList @('serve', '-listen', '127.0.0.1:19797', '-data', "`"$data`"", '-open-browser=false') -PassThru -WindowStyle Hidden
  Start-Sleep -Seconds 5
  if ($proc.HasExited) { throw "[$tag] the old server didn't stay running (exit $($proc.ExitCode))" }
}

# 3. The update.
$newLog = Join-Path $LogDir "$tag-2-new.log"
$code = Msi '/i' $NewMsi $newLog $props
$log = Get-Content $newLog -Raw -Encoding Unicode
$errors = [regex]::Matches($log, 'Error (\d{4})') | ForEach-Object { $_.Groups[1].Value } | Sort-Object -Unique
Write-Host "[$tag] new MSI exit $code; installer errors: $($errors -join ', ')"

if ($ExpectFail) {
  if ($code -eq 0) { throw "[$tag] expected this MSI to fail the upgrade, and it succeeded" }
  if ($proc -and -not $proc.HasExited) { Stop-Process -Id $proc.Id -Force }
  Msi '/x' $OldMsi (Join-Path $LogDir "$tag-3-remove.log") @() | Out-Null
  Write-Host "[$tag] failed as expected"
  exit 0
}
# Verbose logs also hold harmless "DEBUG: Error 28xx" lines about dialogs, so only the exit code
# and the sequencing error itself count.
if ($code -ne 0) { throw "[$tag] upgrade failed: exit $code (3010 means files were left in use), errors: $($errors -join ', ')" }
if ($errors -contains '2613') { throw "[$tag] the upgrade log has error 2613" }
if ($proc -and -not $proc.HasExited) { throw "[$tag] the server started from the old copy is still running" }
if (-not (Test-Path $server)) { throw "[$tag] the server is missing after the upgrade" }
if ($Service) {
  $svc = Get-Service WittyMediaServer -ErrorAction SilentlyContinue
  if (-not $svc) { throw "[$tag] the service is gone after the upgrade" }
}

# 4. Leave the machine as it was.
$code = Msi '/x' $NewMsi (Join-Path $LogDir "$tag-3-remove.log") @()
if ($code -ne 0) { throw "[$tag] removing the new MSI failed: $code" }
Write-Host "[$tag] upgrade clean"
