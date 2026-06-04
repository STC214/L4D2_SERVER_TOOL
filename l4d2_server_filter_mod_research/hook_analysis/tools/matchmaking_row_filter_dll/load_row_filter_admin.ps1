$ErrorActionPreference = "Stop"

$loader = Join-Path $PSScriptRoot "..\matchmaking_probe_loader\L4D2MatchmakingProbeLoader.exe"
$dll = Join-Path $PSScriptRoot "matchmaking_row_filter.dll"

if (!(Test-Path $loader)) {
    throw "Loader exe not found: $loader"
}
if (!(Test-Path $dll)) {
    throw "Row filter DLL not found: $dll"
}

Start-Process -FilePath $loader -ArgumentList @("-dll", "`"$dll`"") -Verb RunAs -Wait
