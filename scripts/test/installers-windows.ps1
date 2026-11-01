<#
  Installs, upgrades and removes WittyMediaServer's MSI for real, with a snapshot of the file system
  and the registry before and after that must match. It refuses to run anywhere but Windows Sandbox,
  because it installs and deletes for real: start it with installers-windows-sandbox.wsb.
#>
$ErrorActionPreference = "Stop"
if ($env:USERNAME -ne "WDAGUtilityAccount") { throw "This only runs in Windows Sandbox: open installers-windows-sandbox.wsb." }

$here = $PSScriptRoot
Start-Transcript -Path (Join-Path $here "results-windows.txt") -Force | Out-Null
$msi = (Get-ChildItem (Join-Path $here "wittymediaserver-*-windows-*.msi") | Select-Object -First 1).FullName
$new = (Get-ChildItem (Join-Path $here "upgrade\wittymediaserver-*-windows-*.msi") | Select-Object -First 1).FullName
$failed = 0
function Say($t) { Write-Host "`n##### $t" -ForegroundColor Cyan }
function Check($ok, $what) {
  if ($ok) { Write-Host "ok: $what" -ForegroundColor Green } else { Write-Host "FAIL: $what" -ForegroundColor Red; $script:failed++ }
}
function Install($file, [string[]]$props) {
  $p = Start-Process msiexec.exe -Wait -PassThru -ArgumentList (@("/i", "`"$file`"", "/qn", "/norestart") + $props)
  Check ($p.ExitCode -eq 0 -or $p.ExitCode -eq 3010) "msiexec /i $($props -join ' ') (exit $($p.ExitCode))"
}
function WaitGone($path, $seconds = 60) {
  $end = (Get-Date).AddSeconds($seconds)
  while ((Test-Path $path) -and (Get-Date) -lt $end) { Start-Sleep -Milliseconds 500 }
  -not (Test-Path $path)
}
function Up($port = 9797, $seconds = 40) {
  $end = (Get-Date).AddSeconds($seconds)
  while ((Get-Date) -lt $end) {
    try { if ((Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$port/api/health" -TimeoutSec 2).StatusCode -eq 200) { return $true } } catch {}
    Start-Sleep -Seconds 1
  }
  $false
}
# Everything except what Windows changes by itself.
function Snapshot {
  $roots = @("C:\Program Files", "C:\ProgramData", $env:LOCALAPPDATA, $env:APPDATA, "$env:USERPROFILE\Desktop")
  $skip = '\\(Temp|Microsoft|Packages|ConnectedDevicesPlatform|Package Cache|USOPrivate|USOShared|regid\.|ssh|Windows Defender|WindowsHolographicDevices)(\\|$)'
  $files = foreach ($r in $roots) { Get-ChildItem $r -Recurse -Force -ErrorAction SilentlyContinue | Where-Object { $_.FullName -notmatch $skip } | ForEach-Object { $_.FullName } }
  $keys = foreach ($k in @("HKCU:\Software", "HKLM:\SOFTWARE", "HKLM:\SYSTEM\CurrentControlSet\Services")) {
    Get-ChildItem $k -Recurse -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'witty' } | ForEach-Object { $_.Name }
  }
  $run = foreach ($k in @("HKCU:\Software\Microsoft\Windows\CurrentVersion\Run", "HKLM:\Software\Microsoft\Windows\CurrentVersion\Run")) {
    (Get-Item $k -ErrorAction SilentlyContinue).Property | Where-Object { $_ -match 'witty' } | ForEach-Object { "$k\$_" }
  }
  $rules = Get-NetFirewallRule -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -match 'witty' } | ForEach-Object { "firewall: $($_.DisplayName)" }
  $svc = Get-Service -ErrorAction SilentlyContinue | Where-Object { $_.Name -match 'witty' } | ForEach-Object { "service: $($_.Name)" }
  $task = Get-ScheduledTask -ErrorAction SilentlyContinue | Where-Object { $_.TaskName -match 'witty' } | ForEach-Object { "task: $($_.TaskName)" }
  $path = foreach ($scope in "User", "Machine") { ([Environment]::GetEnvironmentVariable("PATH", $scope) -split ';') | Where-Object { $_ -match 'witty' } | ForEach-Object { "PATH ($scope): $_" } }
  @($files) + @($keys) + @($run) + @($rules) + @($svc) + @($task) + @($path) | Sort-Object -Unique
}
function Same($before, $what) {
  $after = Snapshot
  $d = Compare-Object @($before) @($after)
  if ($d) { $d | ForEach-Object { Write-Host "  $($_.SideIndicator) $($_.InputObject)" } }
  Check (-not $d) "nothing left: $what ($(@($after).Count) entries before and after are the same)"
}
function Uninstaller($root, [string[]]$a) {
  $report = Join-Path $env:TEMP ("uninstall-" + [Guid]::NewGuid() + ".txt")
  Start-Process (Join-Path $root "Uninstall WittyMediaServer.exe") -ArgumentList (@("/S") + $a + @("--report", "`"$report`"")) -Wait
  $end = (Get-Date).AddSeconds(90)
  while (-not (Test-Path $report) -and (Get-Date) -lt $end) { Start-Sleep -Seconds 1 }
  Check (Test-Path $report) "the uninstaller wrote what it did"
  if (Test-Path $report) { Get-Content $report; Get-Content $report -Raw } else { "" }
}

Say "snapshot before"
$before = Snapshot
"$(@($before).Count) entries"
$film = "$env:USERPROFILE\Videos\A Made-Up Film.mkv"
New-Item -ItemType File -Force $film | Out-Null

Say "just me: no administrator, in the account's own folder"
$perUser = "$env:LOCALAPPDATA\Programs\WittyMediaServer"
Install $msi @("MSIINSTALLPERUSER=1", "AUTOSTART=1", "DESKTOPSHORTCUT=1", "ADDTOPATH=1")
Check (Test-Path "$perUser\WittyMediaServer.exe") "the app is in %LOCALAPPDATA%\Programs"
Check (Test-Path "$perUser\bin\wittymediaserver.exe") "the server is in bin"
Check (-not (Test-Path "$perUser\wms.exe")) "the installer didn't bring the forwarder"
Check (Test-Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\WittyMediaServer") "in Installed apps, for this account"
Check ((Get-ItemProperty "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run").WittyMediaServer) "starts at sign-in"
Check (Test-Path "$env:APPDATA\Microsoft\Windows\Start Menu\Programs\WittyMediaServer.lnk") "Start menu entry"
Check (Test-Path "$env:USERPROFILE\Desktop\WittyMediaServer.lnk") "desktop shortcut"
& "$perUser\bin\wittymediaserver.exe" version

Say "the app starts the server"
Start-Process "$perUser\WittyMediaServer.exe"
Check (Up) "the server answers"
Check (Test-Path "$env:LOCALAPPDATA\WittyMediaServer\wms.db") "the library is in the usual place"

Say "upgrade in place, with it running"
"mine" | Set-Content "$perUser\my-own-file.txt"
Install $new @("MSIINSTALLPERUSER=1")
$v = & "$perUser\bin\wittymediaserver.exe" version
$v
Check ($v -match "9\.9\.9") "the new version is in place"
Check (Test-Path "$env:LOCALAPPDATA\WittyMediaServer\wms.db") "the library stayed"
Check (Test-Path "$perUser\my-own-file.txt") "a file it didn't make stayed"

Say "remove, keeping data"
$out = Uninstaller $perUser @("/KeepData")
Check (WaitGone "$perUser\bin") "the program went"
Check (Test-Path "$env:LOCALAPPDATA\WittyMediaServer\wms.db") "the library stayed"
Check (-not (Test-Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\WittyMediaServer")) "out of Installed apps"
Remove-Item -Force "$perUser\my-own-file.txt"; Remove-Item -Force $perUser -ErrorAction SilentlyContinue

Say "the command line, for when the installer has gone missing"
Install $msi @("MSIINSTALLPERUSER=1")
$report = Join-Path $env:TEMP "cli-report.txt"
& "$perUser\bin\wittymediaserver.exe" --uninstall --yes --keep-data --report $report
Check (WaitGone "$perUser\bin") "the program went"
Check (Test-Path $report) "it wrote what it did"
Check (Test-Path "$env:LOCALAPPDATA\WittyMediaServer\wms.db") "the library stayed"

Say "install again, and a full clean-up"
Install $msi @("MSIINSTALLPERUSER=1")
Start-Process "$perUser\WittyMediaServer.exe"
Check (Up) "the server answers, with the library it had"
"notes" | Set-Content "$env:LOCALAPPDATA\WittyMediaServer\my-own-notes.txt"
$out = Uninstaller $perUser @("/RemoveData")
Check (WaitGone "$perUser") "the program folder went"
Check (-not (Test-Path "$env:LOCALAPPDATA\WittyMediaServer\wms.db")) "the library went"
Check (Test-Path "$env:LOCALAPPDATA\WittyMediaServer\my-own-notes.txt") "a file it didn't make stayed"
Check ($out -match "my-own-notes") "and it said so"
Check (Test-Path $film) "a film wasn't touched"
Remove-Item -Force "$env:LOCALAPPDATA\WittyMediaServer\my-own-notes.txt"; Remove-Item -Force "$env:LOCALAPPDATA\WittyMediaServer" -ErrorAction SilentlyContinue
Remove-Item -Force $film
Start-Sleep -Seconds 5
Same $before "just me"

Say "for everyone: the service and the firewall rule"
$machine = "C:\Program Files\WittyMediaServer"
Install $msi @('MSIINSTALLPERUSER=""', "INSTALLSERVICE=1", "ADDFIREWALL=1", 'AUTOSTART=""', "PORT=9797")
Check (Test-Path "$machine\bin\wittymediaserver.exe") "in Program Files"
$svc = Get-CimInstance Win32_Service -Filter "Name='WittyMediaServer'"
Check ($svc) "the service is registered"
if ($svc) { Write-Host "  $($svc.PathName), as $($svc.StartName), $($svc.State)" }
Check ($svc.StartName -match 'LocalService') "under its limited account"
Check (Up) "the service answers"
Check (Test-Path "C:\ProgramData\WittyMediaServer\wms.db") "the library is in ProgramData"
Check (@(Get-NetFirewallRule -DisplayName WittyMediaServer -ErrorAction SilentlyContinue).Count -ge 2) "the firewall rules are there"
icacls "C:\ProgramData\WittyMediaServer"

Say "upgrade for everyone keeps the service"
Install $new @()
Check (Up) "the service answers after the upgrade"
$v = & "$machine\bin\wittymediaserver.exe" version
$v
Check ($v -match "9\.9\.9") "the new version"
Check (Get-Service WittyMediaServer -ErrorAction SilentlyContinue) "the service stayed"

Say "remove everything"
$out = Uninstaller $machine @("/RemoveData")
Check (WaitGone $machine) "the program folder went"
Check (-not (Get-Service WittyMediaServer -ErrorAction SilentlyContinue)) "the service went"
Check (-not (Test-Path "C:\ProgramData\WittyMediaServer")) "the data went"
Start-Sleep -Seconds 5
Same $before "for everyone"

Say "the result"
if ($failed -eq 0) { Write-Host "ALLPASSED" -ForegroundColor Green } else { Write-Host "$failed FAILED" -ForegroundColor Red }
Stop-Transcript | Out-Null
