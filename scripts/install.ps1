<#
.SYNOPSIS
  Installs WittyMediaServer on Windows as a background task that starts at boot.
  No runtimes or packages are required; everything runs from the install folder.
.EXAMPLE
  Right-click PowerShell -> Run as administrator, then:
  Set-ExecutionPolicy -Scope Process Bypass; .\install.ps1
#>
param(
  [string]$Prefix = "$env:ProgramFiles\WittyMediaServer",
  [string]$Data = "$env:ProgramData\WittyMediaServer",
  [int]$Port = 9797
)
$ErrorActionPreference = "Stop"
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw "Please run this script as Administrator."
}
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
Write-Host "==> installing to $Prefix (data: $Data, port: $Port)"
New-Item -ItemType Directory -Force $Prefix, $Data | Out-Null
# ProgramData lets every account read, and add files to, new folders. This one
# holds password hashes and sessions: only the service and administrators get in.
icacls $Data /inheritance:r /grant:r "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" | Out-Null
Copy-Item -Force "$here\wms.exe" $Prefix
if (Test-Path "$here\bin") { Copy-Item -Recurse -Force "$here\bin" $Prefix }
Get-ChildItem "$here\*.md" -ErrorAction SilentlyContinue | Copy-Item -Force -Destination $Prefix
Copy-Item -Force "$here\start-wms.cmd", "$here\uninstall.ps1" $Prefix
foreach ($f in @("WittyMediaServer.exe", "icon.ico", "LICENSE", "NOTICE")) { if (Test-Path "$here\$f") { Copy-Item -Force "$here\$f" $Prefix } }
# Start-menu shortcut for the tray launcher.
if (Test-Path "$Prefix\WittyMediaServer.exe") {
  $ws = New-Object -ComObject WScript.Shell
  $lnk = $ws.CreateShortcut("$env:ProgramData\Microsoft\Windows\Start Menu\Programs\WittyMediaServer.lnk")
  $lnk.TargetPath = "$Prefix\WittyMediaServer.exe"; $lnk.WorkingDirectory = $Prefix; $lnk.Description = "WittyMediaServer tray launcher"
  if (Test-Path "$Prefix\icon.ico") { $lnk.IconLocation = "$Prefix\icon.ico" }
  $lnk.Save()
}

# Firewall rule for LAN access
netsh advfirewall firewall delete rule name="WittyMediaServer" | Out-Null
# (home and work networks: not open to everyone on a public Wi-Fi network)
netsh advfirewall firewall add rule name="WittyMediaServer" dir=in action=allow protocol=TCP localport=$Port profile=private,domain | Out-Null

# Scheduled task: runs hidden at startup as SYSTEM, restarts on failure.
$action = New-ScheduledTaskAction -Execute "$Prefix\wms.exe" -Argument "serve -data `"$Data`" -listen :$Port" -WorkingDirectory $Prefix
$trigger = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
Unregister-ScheduledTask -TaskName "WittyMediaServer" -Confirm:$false -ErrorAction SilentlyContinue
Register-ScheduledTask -TaskName "WittyMediaServer" -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Description "WittyMediaServer media server" | Out-Null
Start-ScheduledTask -TaskName "WittyMediaServer"
Start-Sleep 2
Write-Host "==> running. Open http://localhost:$Port/ to finish setup."
Write-Host "    Manage with Task Scheduler (task 'WittyMediaServer') or run uninstall.ps1 to remove."
Write-Host "    Prefer a tray icon instead of a background task? Run uninstall.ps1 -KeepData, then start WittyMediaServer.exe."
