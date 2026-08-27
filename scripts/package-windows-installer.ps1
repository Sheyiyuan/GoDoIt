param(
  [Parameter(Mandatory = $true)][string]$Version,
  [Parameter(Mandatory = $true)][string]$Root,
  [Parameter(Mandatory = $true)][string]$Output,
  [string]$Makensis = "makensis"
)
$ErrorActionPreference = "Stop"
$stage = Join-Path $env:RUNNER_TEMP "godoit-nsis"
Remove-Item -Recurse -Force $stage -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $stage | Out-Null
Copy-Item (Join-Path $Root "bin\gdit.exe") $stage
Copy-Item (Join-Path $Root "project\gui\build\bin\gdit-gui.exe") (Join-Path $stage "gdit-gui.exe")
Copy-Item "LICENSE", "THIRD_PARTY_NOTICES.txt" $stage
$nsi = Join-Path $stage "GoDoIt.nsi"
Copy-Item "packaging\windows\GoDoIt.nsi" $nsi
$outFile = "GoDoIt_${Version}_windows_amd64_setup.exe"
(Get-Content $nsi -Raw).Replace('GoDoIt_windows_amd64_setup.exe', $outFile) | Set-Content -NoNewline $nsi
& $Makensis "/V2" $nsi
New-Item -ItemType Directory -Force $Output | Out-Null
Move-Item (Join-Path $stage $outFile) (Join-Path $Output $outFile) -Force
