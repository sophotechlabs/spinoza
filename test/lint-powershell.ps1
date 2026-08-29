#Requires -Version 7.0

[CmdletBinding()]
param(
    [string[]]$Path = @('install.ps1', 'test/smoke.ps1', 'test/pester.ps1', 'test/install/windows.ps1', 'test/install/install.Tests.ps1', 'test/lint-powershell.ps1')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

if (-not (Get-Module -ListAvailable -Name PSScriptAnalyzer)) {
    Write-Output 'lint-ps: installing PSScriptAnalyzer'
    Install-Module -Name PSScriptAnalyzer -Scope CurrentUser -Force -AllowClobber
}
Import-Module PSScriptAnalyzer

$parseErrors = @()
foreach ($file in $Path) {
    $tokens = $null
    $errors = $null
    [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path $file), [ref]$tokens, [ref]$errors) | Out-Null
    if ($errors) {
        $parseErrors += $errors
    }
}
if ($parseErrors.Count -gt 0) {
    $parseErrors | ForEach-Object { Write-Output "lint-ps: $($_.Extent.File):$($_.Extent.StartLineNumber) $($_.Message)" }
    throw "lint-ps: $($parseErrors.Count) parse errors"
}

$findings = @()
foreach ($file in $Path) {
    $findings += Invoke-ScriptAnalyzer -Path $file -Severity Error, Warning
}
if ($findings.Count -gt 0) {
    $findings | Format-Table -AutoSize RuleName, Severity, ScriptName, Line, Message | Out-String | Write-Output
    throw "lint-ps: $($findings.Count) findings"
}

$underFiveOne = @('install.ps1', 'test/pester.ps1', 'test/install/install.Tests.ps1', 'test/install/windows.ps1')
$syntax = @{
    Rules = @{
        PSUseCompatibleSyntax = @{
            Enable         = $true
            TargetVersions = @('5.1')
        }
    }
}
$commands = @{
    Rules = @{
        PSUseCompatibleCommands = @{
            Enable         = $true
            TargetProfiles = @('win-48_x64_10.0.17763.0_5.1.17763.316_x64_4.0.30319.42000_framework')
        }
    }
}

$incompatible = @()
foreach ($file in $underFiveOne) {
    $incompatible += Invoke-ScriptAnalyzer -Path $file -Settings $syntax -IncludeRule PSUseCompatibleSyntax
}
$incompatible += Invoke-ScriptAnalyzer -Path 'install.ps1' -Settings $commands -IncludeRule PSUseCompatibleCommands
if ($incompatible.Count -gt 0) {
    $incompatible | ForEach-Object { Write-Output "lint-ps: $(Split-Path -Leaf $_.ScriptPath):$($_.Line) $($_.Message)" }
    throw "lint-ps: $($incompatible.Count) things windows powershell 5.1 does not have"
}

Write-Output "lint-ps: $($Path.Count) scripts parse clean and pass PSScriptAnalyzer"
Write-Output "lint-ps: $($underFiveOne.Count) of them use no syntax windows powershell 5.1 lacks, and install.ps1 no command it lacks"
