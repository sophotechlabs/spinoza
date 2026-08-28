#Requires -Version 5.1

[CmdletBinding()]
param(
    [string]$Script = 'install.ps1'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$Script = (Resolve-Path -LiteralPath $Script).Path
$target = Join-Path ([System.IO.Path]::GetTempPath()) ('spinoza-install-' + [System.Guid]::NewGuid().ToString('N'))
$before = [string][Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment').GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)

$env:SPINOZA_INSTALL_DIR = $target
try {
    & $Script

    $binary = Join-Path $target 'spinoza.exe'
    if (-not (Test-Path -LiteralPath $binary)) {
        throw "test-install: $binary was not written"
    }

    $reported = (& $binary --version | Select-Object -First 1)
    if (-not $reported) {
        throw 'test-install: the installed binary reported no version'
    }

    if ($env:SPINOZA_VERSION -and $reported -ne $env:SPINOZA_VERSION) {
        throw "test-install: asked for $env:SPINOZA_VERSION and got $reported"
    }

    $userPath = [string][Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment').GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    if ($userPath -notlike "*$target*") {
        throw 'test-install: the install directory was not added to the user PATH'
    }

    $again = & $Script
    if (-not ($again -match 'Updated spinoza|Installed spinoza')) {
        throw 'test-install: a second run did not report what it did'
    }

    Write-Output "test-install: installed and ran spinoza $reported from $target"
}
finally {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
        $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
        if ($before -ne '') {
            $kind = $key.GetValueKind('Path')
        }
        $key.SetValue('Path', $before, $kind)
    }
    finally {
        $key.Close()
    }
    Remove-Item -LiteralPath $target -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -Path Env:\SPINOZA_INSTALL_DIR -ErrorAction SilentlyContinue
}
