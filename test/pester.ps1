#Requires -Version 5.1

[CmdletBinding()]
param([double]$MinimumCoverage = 80)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$available = Get-Module -ListAvailable -Name Pester | Sort-Object Version -Descending | Select-Object -First 1
if ($null -eq $available -or $available.Version.Major -lt 5) {
    Write-Output 'pester: installing Pester'
    Install-Module -Name Pester -Scope CurrentUser -Force -SkipPublisherCheck -MinimumVersion 5.0.0
}
Import-Module Pester -MinimumVersion 5.0.0

$root = Split-Path -Parent $PSScriptRoot
$config = New-PesterConfiguration
$config.Run.Path = (Join-Path $PSScriptRoot 'install')
$config.Run.PassThru = $true
$config.Output.Verbosity = 'Detailed'
$config.CodeCoverage.Enabled = $true
$config.CodeCoverage.Path = (Join-Path $root 'install.ps1')
$config.CodeCoverage.OutputPath = (Join-Path $root 'coverage.ps1.xml')

$result = Invoke-Pester -Configuration $config

if ($result.FailedCount -gt 0) {
    throw "pester: $($result.FailedCount) of $($result.TotalCount) tests failed"
}

if ($result.Result -ne 'Passed') {
    throw "pester: the run came back $($result.Result), which is how a discovery error hides behind a green test count"
}

foreach ($container in $result.Containers) {
    if (-not $container.Passed) {
        throw "pester: $($container.Item) did not pass discovery"
    }
}

$onWindows = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)
$allowed = 'windows'
if ($onWindows) {
    $allowed = 'unix'
}

$undeclared = @()
foreach ($test in $result.Tests) {
    if ($test.Result -ne 'Skipped') {
        continue
    }
    $tags = @($test.Tag) + @($test.Block.Tag)
    if ($tags -contains $allowed) {
        continue
    }
    $undeclared += $test.ExpandedPath
}
if ($undeclared.Count -gt 0) {
    throw "pester: these tests skipped without being tagged '$allowed', so they stopped running for a reason nobody declared:`n$($undeclared -join "`n")"
}

$covered = [math]::Round($result.CodeCoverage.CoveragePercent, 1)
Write-Output "pester: $($result.PassedCount) tests passed, $($result.SkippedCount) skipped, install.ps1 coverage $covered%"
if ($covered -lt $MinimumCoverage) {
    throw "pester: install.ps1 coverage $covered% is under the $MinimumCoverage% gate"
}
