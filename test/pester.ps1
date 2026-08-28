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

$covered = [math]::Round($result.CodeCoverage.CoveragePercent, 1)
Write-Output "pester: $($result.PassedCount) tests passed, install.ps1 coverage $covered%"
if ($covered -lt $MinimumCoverage) {
    throw "pester: install.ps1 coverage $covered% is under the $MinimumCoverage% gate"
}
