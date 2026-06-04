$ErrorActionPreference = "Stop"

$loader = Join-Path $PSScriptRoot "L4D2MatchmakingProbeLoader.exe"
$dll = Join-Path $PSScriptRoot "..\matchmaking_probe_dll\matchmaking_probe_v3.dll"

if (!(Test-Path $loader)) {
    throw "Loader exe not found: $loader"
}
if (!(Test-Path $dll)) {
    throw "Probe DLL not found: $dll"
}

Start-Process -FilePath $loader -ArgumentList @("-dll", "`"$dll`"") -Verb RunAs -Wait
