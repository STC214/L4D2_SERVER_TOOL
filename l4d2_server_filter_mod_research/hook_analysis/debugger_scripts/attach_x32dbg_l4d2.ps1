$ErrorActionPreference = "Stop"

$x32dbg = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages\x64dbg.x64dbg_Microsoft.Winget.Source_8wekyb3d8bbwe\release\x32\x32dbg.exe"
if (!(Test-Path $x32dbg)) {
    throw "x32dbg.exe not found: $x32dbg"
}

$proc = Get-Process -Name "left4dead2" -ErrorAction SilentlyContinue | Select-Object -First 1
if ($null -eq $proc) {
    throw "left4dead2.exe is not running. Start L4D2 and wait at the main menu first."
}

Write-Host "x32dbg: $x32dbg"
Write-Host "left4dead2 pid: $($proc.Id)"
Write-Host "Starting x32dbg as administrator..."

Start-Process -FilePath $x32dbg -ArgumentList @("-p", "$($proc.Id)") -Verb RunAs

Write-Host ""
Write-Host "After x32dbg opens, use the command bar:"
Write-Host "scriptexec `"F:\Project\03_Game_Tools\L4D2_SERVER_TOOL\l4d2_server_filter_mod_research\hook_analysis\debugger_scripts\matchmaking_row_suppression_breakpoints.x32dbg.txt`""
