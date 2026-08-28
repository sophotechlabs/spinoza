#Requires -Version 5.1

[CmdletBinding()]
param(
    [string]$Script = 'install.ps1'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

. (Resolve-Path $Script)

$checked = 0
$failed = @()

function Assert-Equal {
    param(
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Expected,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Actual,
        [Parameter(Mandatory = $true)][string]$What
    )
    $script:checked++
    if ($Expected -ne $Actual) {
        $script:failed += "$What : expected '$Expected', got '$Actual'"
    }
}

function Assert-Throw {
    param(
        [Parameter(Mandatory = $true)][scriptblock]$Action,
        [Parameter(Mandatory = $true)][string]$What
    )
    $script:checked++
    try {
        & $Action
    }
    catch {
        return
    }
    $script:failed += "$What : nothing was thrown"
}

$work = Join-Path ([System.IO.Path]::GetTempPath()) ('spinoza-units-' + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work -Force | Out-Null
try {
    $payload = Join-Path $work 'spinoza_v1.16.0_windows_amd64.zip'
    Set-Content -LiteralPath $payload -Value 'not really a zip' -NoNewline
    $hash = (Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash.ToLowerInvariant()

    $list = Join-Path $work 'checksums.txt'
    Set-Content -LiteralPath $list -Value @(
        "$hash  spinoza_v1.16.0_windows_amd64.zip",
        "0000000000000000000000000000000000000000000000000000000000000000  spinoza_v1.16.0_linux_amd64.tar.gz"
    )

    Assert-Equal -Expected $hash -Actual (Get-ListedChecksum -ListPath $list -Name 'spinoza_v1.16.0_windows_amd64.zip') -What 'a listed name returns its hash'
    Assert-Equal -Expected '' -Actual (Get-ListedChecksum -ListPath $list -Name 'spinoza_v1.16.0_windows_arm64.zip') -What 'an unlisted name returns nothing'
    Assert-Equal -Expected '' -Actual (Get-ListedChecksum -ListPath $list -Name 'windows_amd64.zip') -What 'a partial name does not match'

    Test-Checksum -Path $payload -Name 'spinoza_v1.16.0_windows_amd64.zip' -ListPath $list
    $checked++

    Assert-Throw -Action { Test-Checksum -Path $payload -Name 'spinoza_v1.16.0_linux_amd64.tar.gz' -ListPath $list } -What 'a wrong hash is refused'
    Assert-Throw -Action { Test-Checksum -Path $payload -Name 'spinoza_v1.16.0_windows_arm64.zip' -ListPath $list } -What 'an unlisted asset is refused'

    Assert-Equal -Expected 'True' -Actual (Test-OnPath -Directory 'C:\Apps\spinoza' -Path 'C:\Windows;C:\Apps\spinoza') -What 'a directory already on PATH is found'
    Assert-Equal -Expected 'True' -Actual (Test-OnPath -Directory 'C:\Apps\spinoza' -Path 'C:\Windows;C:\Apps\spinoza\') -What 'a trailing separator still matches'
    Assert-Equal -Expected 'False' -Actual (Test-OnPath -Directory 'C:\Apps\spinoza' -Path 'C:\Windows;C:\Apps\spinozan') -What 'a longer neighbour does not match'
    Assert-Equal -Expected 'False' -Actual (Test-OnPath -Directory 'C:\Apps\spinoza' -Path '') -What 'an empty PATH matches nothing'

    $env:SPINOZA_INSTALL_DIR = 'C:\chosen'
    Assert-Equal -Expected 'C:\chosen' -Actual (Get-InstallDirectory) -What 'SPINOZA_INSTALL_DIR wins'
    Remove-Item -Path Env:\SPINOZA_INSTALL_DIR

    Assert-Equal -Expected '' -Actual (Get-InstalledVersion -Path (Join-Path $work 'absent.exe')) -What 'a missing binary reports no version'

    $bin = Join-Path $work 'bin'
    New-Item -ItemType Directory -Path $bin -Force | Out-Null
    $source = Join-Path $work 'source.bin'
    $target = Join-Path $bin 'target.bin'

    Set-Content -LiteralPath $source -Value 'first' -NoNewline
    Install-Binary -Source $source -Target $target
    Assert-Equal -Expected 'first' -Actual (Get-Content -LiteralPath $target -Raw) -What 'a first install writes the binary'

    Set-Content -LiteralPath $source -Value 'second' -NoNewline
    Install-Binary -Source $source -Target $target
    Assert-Equal -Expected 'second' -Actual (Get-Content -LiteralPath $target -Raw) -What 'a second install replaces the binary'
    Assert-Equal -Expected 'False' -Actual (Test-Path -LiteralPath "$target.old") -What 'the replaced binary is not left behind'

    Assert-Throw -Action { Install-Binary -Source (Join-Path $work 'absent.bin') -Target $target } -What 'an install from nothing is refused'
    Assert-Equal -Expected 'second' -Actual (Get-Content -LiteralPath $target -Raw) -What 'a failed install puts the old binary back'

    $checked++
    $resolved = Resolve-Architecture
    if ($resolved -ne 'amd64' -and $resolved -ne 'arm64') {
        $failed += "architecture resolves to something published : got '$resolved'"
    }
}
finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}

if ($failed.Count -gt 0) {
    $failed | ForEach-Object { Write-Output "install-functions: $_" }
    throw "install-functions: $($failed.Count) of $checked checks failed"
}

Write-Output "install-functions: $checked checks passed"
