param([string]$Prefix = "$env:ProgramFiles\WittyMediaServer", [switch]$KeepData)
$ErrorActionPreference = "SilentlyContinue"
Stop-ScheduledTask -TaskName "WittyMediaServer"
Unregister-ScheduledTask -TaskName "WittyMediaServer" -Confirm:$false
Get-Process wms, WittyMediaServer | Stop-Process -Force
Remove-Item -Force "$env:ProgramData\Microsoft\Windows\Start Menu\Programs\WittyMediaServer.lnk"
Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "WittyMediaServer"
netsh advfirewall firewall delete rule name="WittyMediaServer" | Out-Null
Remove-Item -Recurse -Force $Prefix
if (-not $KeepData) { Remove-Item -Recurse -Force "$env:ProgramData\WittyMediaServer" }
Write-Host "WittyMediaServer removed."
