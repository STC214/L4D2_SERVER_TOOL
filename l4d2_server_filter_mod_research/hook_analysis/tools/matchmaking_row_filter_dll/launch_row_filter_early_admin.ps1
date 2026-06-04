$ErrorActionPreference = "Stop"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
$isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    $script = $MyInvocation.MyCommand.Path
    Start-Process -FilePath "powershell.exe" -ArgumentList @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", "`"$script`""
    ) -Verb RunAs
    exit
}

$loader = Join-Path $PSScriptRoot "..\matchmaking_probe_loader\L4D2MatchmakingProbeLoader.exe"
$dll = Join-Path $PSScriptRoot "matchmaking_row_filter.dll"
$processName = "left4dead2"

if (!(Test-Path $loader)) {
    throw "Loader exe not found: $loader"
}
if (!(Test-Path $dll)) {
    throw "Row filter DLL not found: $dll"
}

$existing = Get-Process -Name $processName -ErrorAction SilentlyContinue
if ($existing) {
    Write-Host "left4dead2.exe is already running; injecting into the existing process."
    & $loader -dll "$dll"
    exit $LASTEXITCODE
}

Write-Host "Launching Left 4 Dead 2 through Steam..."
Start-Process "steam://run/550"

$deadline = (Get-Date).AddSeconds(120)
$target = $null
while ((Get-Date) -lt $deadline) {
    $target = Get-Process -Name $processName -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($target) {
        break
    }
    Start-Sleep -Milliseconds 100
}

if (!$target) {
    throw "Timed out waiting for left4dead2.exe."
}

Write-Host "left4dead2.exe detected, injecting row filter immediately..."
& $loader -dll "$dll"
exit $LASTEXITCODE
