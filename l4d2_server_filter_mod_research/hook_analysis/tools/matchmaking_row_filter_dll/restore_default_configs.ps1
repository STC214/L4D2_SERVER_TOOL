$ErrorActionPreference = 'Stop'

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$defaults = Join-Path $scriptDir 'config_defaults'

$pairs = @(
    @{ Source = 'default_blocked_keywords.txt'; Target = 'blocked_keywords.txt' },
    @{ Source = 'default_blocked_connectstrings.txt'; Target = 'blocked_connectstrings.txt' },
    @{ Source = 'default_learned_connectstrings.txt'; Target = 'learned_connectstrings.txt' },
    @{ Source = 'default_auto_derived_connectstrings.txt'; Target = 'auto_derived_connectstrings.txt' }
)

foreach ($pair in $pairs) {
    $source = Join-Path $defaults $pair.Source
    $target = Join-Path $scriptDir $pair.Target
    Copy-Item -LiteralPath $source -Destination $target -Force
    Write-Host "Restored $($pair.Target)"
}

Write-Host 'Default matchmaking row-filter configs restored.'
