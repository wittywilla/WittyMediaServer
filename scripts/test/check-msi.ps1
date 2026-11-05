<#
.SYNOPSIS
  Checks a built MSI without installing it: reads its tables, unpacks it with an administrative
  extract, which registers nothing, and checks what comes out. Needs no administrator, opens no
  window, and changes nothing on this computer but the folder it's given.
.EXAMPLE
  scripts\test\check-msi.ps1 -Msi dist\wittymediaserver-0.6.0-alpha-windows-amd64.msi -Version 0.6.0-alpha -Scratch D:\scratch\msi-check
#>
param(
  [Parameter(Mandatory)][string]$Msi,
  [Parameter(Mandatory)][string]$Version,
  [Parameter(Mandatory)][string]$Scratch,
  [string]$PackageDir = ""
)
$ErrorActionPreference = "Stop"
$Msi = (Resolve-Path $Msi).Path
$Scratch = (New-Item -ItemType Directory -Force $Scratch).FullName
$failed = 0
function Fail($text) { Write-Host "FAIL: $text"; $script:failed++ }
function Ok($text) { Write-Host "ok: $text" }

function Query($db, $sql, $cols) {
  $view = $db.GetType().InvokeMember("OpenView", "InvokeMethod", $null, $db, @($sql))
  [void]$view.GetType().InvokeMember("Execute", "InvokeMethod", $null, $view, $null)
  $rows = @()
  while ($true) {
    $rec = $view.GetType().InvokeMember("Fetch", "InvokeMethod", $null, $view, $null)
    if ($null -eq $rec) { break }
    $row = @{}
    for ($i = 0; $i -lt $cols.Count; $i++) {
      $row[$cols[$i]] = $rec.GetType().InvokeMember("StringData", "GetProperty", $null, $rec, @($i + 1))
    }
    $rows += [pscustomobject]$row
  }
  [void]$view.GetType().InvokeMember("Close", "InvokeMethod", $null, $view, $null)
  return $rows
}

$installer = New-Object -ComObject WindowsInstaller.Installer
$db = $installer.GetType().InvokeMember("OpenDatabase", "InvokeMethod", $null, $installer, @($Msi, 0))

# ---- the tables
$props = @{}
foreach ($r in (Query $db 'SELECT Property, Value FROM Property' @('p', 'v'))) { $props[$r.p] = $r.v }
Write-Host "ProductName=$($props.ProductName) ProductVersion=$($props.ProductVersion) UpgradeCode=$($props.UpgradeCode) ProductCode=$($props.ProductCode)"
Write-Host "ALLUSERS=$($props.ALLUSERS) MSIINSTALLPERUSER=$($props.MSIINSTALLPERUSER) ARPSYSTEMCOMPONENT=$($props.ARPSYSTEMCOMPONENT) AUTOSTART=$($props.AUTOSTART) PORT=$($props.PORT)"
if ($props.ProductName -ne "WittyMediaServer") { Fail "product name" }
if ($props.UpgradeCode -ne "{6F3C2A9E-58D1-4B77-9C1E-2D7A40B5E8C3}") { Fail "the upgrade code changed: $($props.UpgradeCode)" } else { Ok "the upgrade code is the fixed one" }
if ($props.ALLUSERS -ne "2" -or $props.MSIINSTALLPERUSER -ne "1") { Fail "it should start as just this account, and be able to install for everyone" } else { Ok "just me by default, everyone when asked" }
if ($props.ARPSYSTEMCOMPONENT -ne "1") { Fail "the MSI's own line would show in Installed apps" } else { Ok "Installed apps shows our entry, which can ask about data" }
foreach ($p in "AUTOSTART", "DESKTOPSHORTCUT", "ADDTOPATH", "INSTALLSERVICE", "ADDFIREWALL", "PORT") {
  if ($props.SecureCustomProperties -notmatch $p) { Fail "$p isn't secure, so a wizard's answer wouldn't reach an install for everyone" }
}
$summary = $db.GetType().InvokeMember("SummaryInformation", "GetProperty", $null, $db, @(0))
$word = $summary.GetType().InvokeMember("Property", "GetProperty", $null, $summary, @(15))
Write-Host "Summary word count: $word"
if ($word -band 8) { Fail "the package says it never needs an administrator" } else { Ok "it can ask for an administrator when it's for everyone" }
[void][Runtime.InteropServices.Marshal]::ReleaseComObject($summary)

$upgrades = Query $db 'SELECT UpgradeCode, VersionMin, VersionMax, Attributes, ActionProperty FROM Upgrade' @('code', 'min', 'max', 'attr', 'prop')
$upgrades | ForEach-Object { Write-Host "Upgrade: $($_.code) min=$($_.min) max=$($_.max) attr=$($_.attr) -> $($_.prop)" }
if (-not ($upgrades | Where-Object { $_.prop -eq 'WIX_UPGRADE_DETECTED' })) { Fail "nothing upgrades in place" }

$shortcuts = Query $db 'SELECT Shortcut, Directory_, Name, Component_, Target FROM Shortcut' @('id', 'dir', 'name', 'comp', 'target')
$shortcuts | ForEach-Object { Write-Host "Shortcut: $($_.name) in $($_.dir), component $($_.comp), target $($_.target)" }
if (-not ($shortcuts | Where-Object { $_.dir -eq 'ProgramMenuFolder' })) { Fail "no Start menu entry" }

$comps = Query $db 'SELECT Component, Condition FROM Component' @('id', 'cond')
foreach ($want in @{DesktopEntry = 'DESKTOPSHORTCUT'; StartAtSignIn = 'AUTOSTART'; ServerAsService = 'INSTALLSERVICE'; ServerPlain = 'INSTALLSERVICE'; FirewallRule = 'ADDFIREWALL'; PathForMe = 'ADDTOPATH'; PathForEveryone = 'ADDTOPATH'}.GetEnumerator()) {
  $c = $comps | Where-Object { $_.id -eq $want.Key }
  if (-not $c -or $c.cond -notmatch $want.Value) { Fail "$($want.Key) isn't conditional on $($want.Value)" } else { Ok "$($want.Key) only with: $($c.cond)" }
}

$svc = Query $db 'SELECT ServiceInstall, Name, StartName, Arguments, Component_ FROM ServiceInstall' @('id', 'name', 'account', 'args', 'comp')
$svc | ForEach-Object { Write-Host "Service: $($_.name) as $($_.account), arguments [$($_.args)], in $($_.comp)" }
if ($svc.account -notmatch 'LocalService') { Fail "the service isn't under the limited account" } else { Ok "the service runs under LocalService" }
if ($svc.comp -ne 'ServerAsService') { Fail "the service is in the wrong component" }
# A folder property ends in a backslash, which would escape the closing quote.
if ($svc.args -match '\[ServiceDataFolder\]"') { Fail "the service's data folder argument ends in a backslash before its quote" } else { Ok "the service's arguments are quoted safely" }
# The port is in settings.ini, where Settings → Network can change it: -listen would win over that.
if ($svc.args -match '-listen') { Fail "the service is given -listen, which would win over settings.ini" } else { Ok "the service takes its port from settings.ini" }
$tables = (Query $db 'SELECT Name FROM _Tables' @('n')).n
$fwTable = $tables | Where-Object { $_ -match 'FirewallException' } | Select-Object -First 1
if (-not $fwTable) {
  Fail "no firewall table"
} else {
  $cols = (Query $db "SELECT Name FROM _Columns WHERE ``Table``='$fwTable'" @('n')).n
  $fw = Query $db ("SELECT * FROM ``$fwTable``") $cols
  $fw | ForEach-Object { Write-Host "Firewall ($fwTable): $(($_.PSObject.Properties | Where-Object { $_.Value } | ForEach-Object { "$($_.Name)=$($_.Value)" }) -join ' ')" }
  if (@($fw).Count -ne 2) { Fail "there should be a rule for each of TCP and UDP" } else { Ok "a firewall rule for each of TCP and UDP" }
  # TCP follows the program, not one port, so it goes on working when the port is changed.
  $tcp = $fw | Where-Object { ($_.PSObject.Properties | Where-Object { $_.Value -match 'wittymediaserver\.exe' }) }
  if (-not $tcp) { Fail "the TCP rule isn't for the server program, so it wouldn't follow a changed port" } else { Ok "the TCP rule is for the server program" }
}

$reg = Query $db 'SELECT `Registry`, `Root`, `Key`, `Name`, `Value`, `Component_` FROM `Registry`' @('id', 'root', 'key', 'name', 'value', 'comp')
$arp = $reg | Where-Object { $_.key -like '*CurrentVersion\Uninstall\WittyMediaServer' }
Write-Host "Registry rows: $($reg.Count). Installed apps entry:"
$arp | ForEach-Object { Write-Host "  $($_.name) = $($_.value)" }
if (($arp | Where-Object { $_.name -eq 'UninstallString' }).value -notmatch 'Uninstall WittyMediaServer\.exe') { Fail "Installed apps doesn't point at our uninstaller" }
if (($arp | Where-Object { $_.name -eq 'QuietUninstallString' }).value -notmatch '/S') { Fail "no quiet uninstall string" }
if (($arp | Where-Object { $_.name -eq 'DisplayVersion' }).value -ne $Version) { Fail "the version shown isn't $Version" }
if (($arp | Where-Object { $_.name -eq 'ProductCode' }).value -ne '[ProductCode]') { Fail "the entry doesn't hold the product code the uninstaller needs" }

$files = Query $db 'SELECT File, Component_, FileName, FileSize FROM File' @('id', 'comp', 'name', 'size')
Write-Host "Files in the MSI: $($files.Count)"
$files | ForEach-Object { Write-Host ("  {0,12} {1}" -f $_.size, $_.name) }
if ($files | Where-Object { $_.name -match '(^|\|)wms\.exe$' }) { Fail "the MSI holds the forwarder" }
if (-not ($files | Where-Object { $_.name -match '(^|\|)settings\.ini$' })) { Fail "no settings.ini beside the program" } else { Ok "settings.ini is beside the program" }

$actions = Query $db 'SELECT Action, Condition, Sequence FROM InstallExecuteSequence' @('a', 'c', 's') | Sort-Object { [int]$_.s }
$order = @{}
$actions | ForEach-Object { $order[$_.a] = [int]$_.s }
if ($order.StopRunningApp -ge $order.RemoveExistingProducts) { Fail "it stops the running copy too late" } else { Ok "stops what's running before removing the old version" }
if ($order.SetAUTOSTART -le $order.FindRelatedProducts) { Fail "turning off start at sign-in is remembered before an upgrade is detected" } else { Ok "remembers start at sign-in once an upgrade is detected" }
if ($order.SetScope -ge $order.FindRelatedProducts) { Fail "who it's for is settled after looking for what to upgrade" } else { Ok "settles who it's for before looking for what to upgrade" }
if ($order.SecureServiceData -ge $order.StartServices) { Fail "the service would start before its folder is made private" } else { Ok "the service's folder is private before it first starts" }
if (-not $order.CarryPort -or $order.CarryPort -le $order.SecureServiceData -or $order.CarryPort -ge $order.StartServices) { Fail "the service's port isn't carried over between securing its folder and starting it" } else { Ok "carries the service's port over before it starts" }
if ($order.TakeOverOldCopyUser -le $order.InstallFiles) { Fail "it clears the old copy before the new one's there" }
if ($order.TakeOverOldCopyUser -ge $order.WriteRegistryValues) { Fail "it would clear the new entries too" } else { Ok "clears the old copy between the new files and the new registry entries" }

$ca = Query $db 'SELECT Action, Type, Source, Target FROM CustomAction' @('a', 't', 's', 'tgt')
foreach ($id in "SetScope", "SetAUTOSTART") {
  $c = $ca | Where-Object { $_.a -eq $id }
  Write-Host "$id sets $($c.s) to [$($c.tgt)]"
  if ($c.tgt -eq '{}') { Fail "$id would set the text {} rather than clear it" }
}

# ---- an administrative extract: files only, and nothing is registered
[void][Runtime.InteropServices.Marshal]::ReleaseComObject($db)
$target = Join-Path $Scratch "extract"
if (Test-Path $target) { Remove-Item -Recurse -Force $target }
New-Item -ItemType Directory -Force $target | Out-Null
$log = Join-Path $Scratch "extract.log"
$proc = Start-Process msiexec.exe -ArgumentList @("/a", "`"$Msi`"", "/qn", "TARGETDIR=`"$target`"", "/l*v", "`"$log`"") -Wait -PassThru
Write-Host "msiexec /a exit code: $($proc.ExitCode)"
if ($proc.ExitCode -ne 0) { Fail "the administrative extract failed: see $log" }
$root = Get-ChildItem -Path $target -Recurse -Filter WittyMediaServer.exe -File | Select-Object -First 1
if (-not $root) {
  Fail "WittyMediaServer.exe wasn't in the extract"
} else {
  $dir = $root.Directory.FullName
  Write-Host "extracted to: $dir"
  Get-ChildItem -LiteralPath $dir -Recurse -File | Where-Object { $_.Extension -ne ".msi" } | ForEach-Object { Write-Host ("  {0,12} {1}" -f $_.Length, $_.FullName.Substring($dir.Length + 1)) }
  foreach ($f in "Uninstall WittyMediaServer.exe", "package-info.json", "bin\wittymediaserver.exe", "docs\LICENSE", "docs\NOTICE", "docs\THIRD_PARTY_LICENSES.md") {
    if (-not (Test-Path -LiteralPath (Join-Path $dir $f))) { Fail "$f is missing" }
  }
  if ((Get-Content -LiteralPath (Join-Path $dir "package-info.json") -Raw) -notmatch '"msi"') { Fail "the marker doesn't say msi" } else { Ok "the marker says the MSI installed it" }
  if ($PackageDir) {
    $bad = 0
    Get-ChildItem -LiteralPath $PackageDir -Recurse -File | ForEach-Object {
      $rel = $_.FullName.Substring((Resolve-Path $PackageDir).Path.Length + 1)
      if ($rel -in "wms.exe", "package-info.json") { return }
      $other = Join-Path $dir $rel
      if (-not (Test-Path -LiteralPath $other) -or (Get-FileHash -LiteralPath $other).Hash -ne (Get-FileHash -LiteralPath $_.FullName).Hash) { Fail "$rel differs from the package"; $bad++ }
    }
    if ($bad -eq 0) { Ok "every file is the same as in the portable package" }
  }

  # What Properties and Task Manager will say.
  Add-Type -AssemblyName System.Drawing
  foreach ($exe in "WittyMediaServer.exe", "Uninstall WittyMediaServer.exe", "bin\wittymediaserver.exe") {
    $v = (Get-Item -LiteralPath (Join-Path $dir $exe)).VersionInfo
    Write-Host ("{0,-34} description=[{1}] product=[{2}] version=[{3}] file=[{4}] original=[{5}]" -f $exe, $v.FileDescription, $v.ProductName, $v.ProductVersion, $v.FileVersion, $v.OriginalFilename)
    if (-not $v.FileDescription -or $v.ProductName -ne "WittyMediaServer" -or -not $v.ProductVersion.StartsWith($Version) -or $v.OriginalFilename -ne (Split-Path $exe -Leaf)) { Fail "$exe's details are wrong" }
    $icon = [System.Drawing.Icon]::ExtractAssociatedIcon((Join-Path $dir $exe))
    if (-not $icon) { Fail "$exe has no icon" } else { $icon.Dispose() }
  }
  Ok "every program has an icon, a description and a version"
}

if ($failed) { Write-Host "FAILED: $failed"; exit 1 }
Write-Host "ALLPASSED"
