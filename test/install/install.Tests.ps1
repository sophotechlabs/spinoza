BeforeAll {
    . (Join-Path $PSScriptRoot '..' '..' 'install.ps1')

    function NewWorkspace {
        $work = Join-Path ([System.IO.Path]::GetTempPath()) ('spinoza-tests-' + [System.Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $work -Force | Out-Null
        return $work
    }

    function NewChecksumList {
        param([string]$Work, [string]$Payload)
        $list = Join-Path $Work 'checksums.txt'
        Set-Content -LiteralPath $list -Value @(
            "$(Get-Sha256 -Path $Payload)  spinoza_v9.9.9_windows_amd64.zip",
            '0000000000000000000000000000000000000000000000000000000000000000  spinoza_v9.9.9_linux_amd64.tar.gz'
        )
        return $list
    }
}

Describe 'Get-ListedChecksum' {
    BeforeAll {
        $script:work = NewWorkspace
        $script:list = Join-Path $script:work 'list.txt'
        Set-Content -LiteralPath $script:list -Value @(
            'aaaa  spinoza_v1.0.0_windows_amd64.zip',
            '',
            'bbbb  spinoza_v1.0.0_linux_amd64.tar.gz'
        )
    }
    AfterAll { Remove-Item -LiteralPath $script:work -Recurse -Force -ErrorAction SilentlyContinue }

    It 'returns the hash listed against a name' {
        Get-ListedChecksum -ListPath $script:list -Name 'spinoza_v1.0.0_windows_amd64.zip' | Should -Be 'aaaa'
    }
    It 'returns nothing for a name that is not listed' {
        Get-ListedChecksum -ListPath $script:list -Name 'spinoza_v1.0.0_windows_arm64.zip' | Should -Be ''
    }
    It 'does not match on a partial name' {
        Get-ListedChecksum -ListPath $script:list -Name 'windows_amd64.zip' | Should -Be ''
    }
    It 'skips a blank line rather than failing on it' {
        Get-ListedChecksum -ListPath $script:list -Name 'spinoza_v1.0.0_linux_amd64.tar.gz' | Should -Be 'bbbb'
    }
}

Describe 'Get-Sha256' {
    It 'hashes a file to the published vector for its contents' {
        $work = NewWorkspace
        try {
            $file = Join-Path $work 'abc.txt'
            [System.IO.File]::WriteAllText($file, 'abc')
            Get-Sha256 -Path $file | Should -Be 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad'
        }
        finally {
            Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

Describe 'Test-Checksum' {
    BeforeAll {
        $script:work = NewWorkspace
        $script:payload = Join-Path $script:work 'spinoza_v9.9.9_windows_amd64.zip'
        Set-Content -LiteralPath $script:payload -Value 'not really a zip' -NoNewline
        $script:list = NewChecksumList -Work $script:work -Payload $script:payload
    }
    AfterAll { Remove-Item -LiteralPath $script:work -Recurse -Force -ErrorAction SilentlyContinue }

    It 'accepts a file whose hash matches the list' {
        { Test-Checksum -Path $script:payload -Name 'spinoza_v9.9.9_windows_amd64.zip' -ListPath $script:list } | Should -Not -Throw
    }
    It 'refuses a file whose hash does not match' {
        { Test-Checksum -Path $script:payload -Name 'spinoza_v9.9.9_linux_amd64.tar.gz' -ListPath $script:list } | Should -Throw '*checksum mismatch*'
    }
    It 'refuses an asset the list does not mention' {
        { Test-Checksum -Path $script:payload -Name 'spinoza_v9.9.9_windows_arm64.zip' -ListPath $script:list } | Should -Throw '*not listed*'
    }
}

Describe 'Test-OnPath' {
    It 'finds a directory already on the path' {
        Test-OnPath -Directory 'C:\Apps\spinoza' -Path 'C:\Windows;C:\Apps\spinoza' | Should -BeTrue
    }
    It 'ignores a trailing separator' {
        Test-OnPath -Directory 'C:\Apps\spinoza' -Path 'C:\Windows;C:\Apps\spinoza\' | Should -BeTrue
    }
    It 'matches without regard to case, the way windows does' {
        Test-OnPath -Directory 'C:\Apps\spinoza' -Path 'C:\APPS\SPINOZA' | Should -BeTrue
    }
    It 'does not match a longer neighbour' {
        Test-OnPath -Directory 'C:\Apps\spinoza' -Path 'C:\Windows;C:\Apps\spinozan' | Should -BeFalse
    }
    It 'matches nothing in an empty path' {
        Test-OnPath -Directory 'C:\Apps\spinoza' -Path '' | Should -BeFalse
    }
}

Describe 'Get-InstallDirectory' {
    AfterEach { Remove-Item -Path Env:\SPINOZA_INSTALL_DIR -ErrorAction SilentlyContinue }

    It 'takes the directory the caller chose' {
        $env:SPINOZA_INSTALL_DIR = 'C:\chosen'
        Get-InstallDirectory | Should -Be 'C:\chosen'
    }
    It 'falls back to a programs directory under the local app data' {
        Remove-Item -Path Env:\SPINOZA_INSTALL_DIR -ErrorAction SilentlyContinue
        $env:LOCALAPPDATA = Join-Path ([System.IO.Path]::GetTempPath()) 'AppData'
        Get-InstallDirectory | Should -Be (Join-Path $env:LOCALAPPDATA (Join-Path 'Programs' 'spinoza'))
    }
}

Describe 'Get-InstalledVersion' {
    It 'reports nothing where no binary is installed yet' {
        Get-InstalledVersion -Path (Join-Path ([System.IO.Path]::GetTempPath()) 'absent-spinoza.exe') | Should -Be ''
    }
    It 'reports nothing where the installed binary cannot be run' {
        $work = NewWorkspace
        try {
            $broken = Join-Path $work 'broken'
            Set-Content -LiteralPath $broken -Value 'not an executable'
            Get-InstalledVersion -Path $broken | Should -Be ''
        }
        finally {
            Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

Describe 'Install-Binary' {
    BeforeEach {
        $script:work = NewWorkspace
        $script:source = Join-Path $script:work 'source.bin'
        $script:target = Join-Path $script:work 'target.bin'
    }
    AfterEach { Remove-Item -LiteralPath $script:work -Recurse -Force -ErrorAction SilentlyContinue }

    It 'writes the binary where none was installed' {
        Set-Content -LiteralPath $script:source -Value 'first' -NoNewline
        Install-Binary -Source $script:source -Target $script:target
        Get-Content -LiteralPath $script:target -Raw | Should -Be 'first'
    }
    It 'replaces an installed binary and leaves nothing behind' {
        Set-Content -LiteralPath $script:source -Value 'first' -NoNewline
        Install-Binary -Source $script:source -Target $script:target
        Set-Content -LiteralPath $script:source -Value 'second' -NoNewline
        Install-Binary -Source $script:source -Target $script:target
        Get-Content -LiteralPath $script:target -Raw | Should -Be 'second'
        Test-Path -LiteralPath "$($script:target).old" | Should -BeFalse
    }
    It 'puts the installed binary back when the new one cannot be written' {
        Set-Content -LiteralPath $script:source -Value 'first' -NoNewline
        Install-Binary -Source $script:source -Target $script:target
        { Install-Binary -Source (Join-Path $script:work 'absent.bin') -Target $script:target } | Should -Throw
        Get-Content -LiteralPath $script:target -Raw | Should -Be 'first'
    }
    It 'clears a stale retired copy from an interrupted install' {
        Set-Content -LiteralPath "$($script:target).old" -Value 'stale' -NoNewline
        Set-Content -LiteralPath $script:source -Value 'fresh' -NoNewline
        Install-Binary -Source $script:source -Target $script:target
        Test-Path -LiteralPath "$($script:target).old" | Should -BeFalse
    }
}

Describe 'Resolve-Architecture' {
    It 'maps what this machine reports onto a published architecture' {
        Resolve-Architecture | Should -BeIn @('amd64', 'arm64')
    }
    It 'maps the name a 32-bit shell reports on a 64-bit machine' {
        Mock Get-OSArchitecture { 'AMD64' }
        Resolve-Architecture | Should -Be 'amd64'
    }
    It 'refuses an architecture with no published build' {
        Mock Get-OSArchitecture { 'MIPS' }
        { Resolve-Architecture } | Should -Throw '*is not supported*'
    }
}

Describe 'Get-OSArchitecture' {
    It 'names an architecture on this machine' {
        Get-OSArchitecture | Should -Not -BeNullOrEmpty
    }
}

Describe 'Resolve-LatestVersion' {
    It 'reads the tag out of the redirect' {
        Mock Get-RedirectLocation { 'https://github.com/sophotechlabs/spinoza/releases/tag/v1.17.0' }
        Resolve-LatestVersion | Should -Be 'v1.17.0'
    }
    It 'refuses a redirect that names no tag' {
        Mock Get-RedirectLocation { 'https://github.com/sophotechlabs/spinoza/releases' }
        { Resolve-LatestVersion } | Should -Throw '*no published release*'
    }
    It 'refuses a response with no location at all' {
        Mock Get-RedirectLocation { $null }
        { Resolve-LatestVersion } | Should -Throw '*no published release*'
    }
}

Describe 'Add-ToPath' {
    It 'appends to a path that does not carry the directory' {
        Mock Get-UserPath { [pscustomobject]@{ Value = 'C:\Windows'; Kind = 'ExpandString' } }
        Mock Set-UserPath { }
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq 'C:\Windows;C:\Apps\spinoza' }
    }
    It 'leaves the path alone where the directory is already on it' {
        Mock Get-UserPath { [pscustomobject]@{ Value = 'C:\Windows;C:\Apps\spinoza'; Kind = 'ExpandString' } }
        Mock Set-UserPath { }
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeFalse
        Should -Invoke Set-UserPath -Times 0
    }
    It 'writes a bare directory where the account has no path yet' {
        Mock Get-UserPath { [pscustomobject]@{ Value = ''; Kind = 'ExpandString' } }
        Mock Set-UserPath { }
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq 'C:\Apps\spinoza' }
    }
    It 'does not leave a doubled separator behind a trailing one' {
        Mock Get-UserPath { [pscustomobject]@{ Value = 'C:\Windows;'; Kind = 'ExpandString' } }
        Mock Set-UserPath { }
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq 'C:\Windows;C:\Apps\spinoza' }
    }
    It 'keeps the value kind the registry already had' {
        Mock Get-UserPath { [pscustomobject]@{ Value = '%USERPROFILE%\bin'; Kind = 'ExpandString' } }
        Mock Set-UserPath { }
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Kind -eq 'ExpandString' }
    }
}

Describe 'Install-App' {
    BeforeEach {
        $script:work = NewWorkspace
        Set-Content -LiteralPath (Join-Path $script:work 'checksums.txt') -Value 'aaaa  spinoza_v9.9.9_windows_amd64_app.zip'
        Mock Save-Download { }
        Mock Test-Checksum { }
        Mock Add-StartMenuShortcut { }
        Mock Install-Binary { }
        Mock Expand-Archive {
            New-Item -ItemType Directory -Path $DestinationPath -Force | Out-Null
            Set-Content -LiteralPath (Join-Path $DestinationPath 'Spinoza.exe') -Value 'app' -NoNewline
        }
    }
    AfterEach { Remove-Item -LiteralPath $script:work -Recurse -Force -ErrorAction SilentlyContinue }

    It 'does nothing where the release publishes no app for this architecture' {
        Set-Content -LiteralPath (Join-Path $script:work 'checksums.txt') -Value 'aaaa  spinoza_v9.9.9_windows_arm64_app.zip'
        Install-App -Temp $script:work -Version 'v9.9.9' -Arch 'amd64' -Directory $script:work | Should -BeFalse
        Should -Invoke Save-Download -Times 0
    }
    It 'installs the app beside the binary rather than over it' {
        $target = Join-Path (Join-Path $script:work 'app') 'Spinoza.exe'
        Install-App -Temp $script:work -Version 'v9.9.9' -Arch 'amd64' -Directory $script:work | Should -BeTrue
        Should -Invoke Install-Binary -Times 1 -ParameterFilter { $Target -eq $target }
        Should -Invoke Add-StartMenuShortcut -Times 1
    }
    It 'refuses an app archive that carries no executable' {
        Mock Expand-Archive { New-Item -ItemType Directory -Path $DestinationPath -Force | Out-Null }
        { Install-App -Temp $script:work -Version 'v9.9.9' -Arch 'amd64' -Directory $script:work } | Should -Throw '*did not contain Spinoza.exe*'
    }
}

Describe 'Install-Spinoza' {
    BeforeEach {
        $script:work = NewWorkspace
        $script:path = $env:Path
        $env:SPINOZA_INSTALL_DIR = $script:work
        $env:SPINOZA_VERSION = 'v9.9.9'
        Mock Resolve-Architecture { 'amd64' }
        Mock Resolve-LatestVersion { 'v1.17.0' }
        Mock Save-Download { }
        Mock Test-Checksum { }
        Mock Install-Binary { }
        Mock Install-App { $false }
        Mock Add-ToPath { $true }
        Mock Get-InstalledVersion { '' }
        Mock Expand-Archive {
            New-Item -ItemType Directory -Path $DestinationPath -Force | Out-Null
            Set-Content -LiteralPath (Join-Path $DestinationPath 'spinoza.exe') -Value 'binary' -NoNewline
        }
    }
    AfterEach {
        $env:Path = $script:path
        Remove-Item -Path Env:\SPINOZA_INSTALL_DIR -ErrorAction SilentlyContinue
        Remove-Item -Path Env:\SPINOZA_VERSION -ErrorAction SilentlyContinue
        Remove-Item -Path Env:\SPINOZA_SKIP_APP -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $script:work -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'downloads the asset named for the pinned version and this architecture' {
        Install-Spinoza | Out-Null
        Should -Invoke Save-Download -Times 1 -ParameterFilter {
            $Uri -eq 'https://github.com/sophotechlabs/spinoza/releases/download/v9.9.9/spinoza_v9.9.9_windows_amd64.zip'
        }
    }
    It 'asks the release page for a version where none was pinned' {
        Remove-Item -Path Env:\SPINOZA_VERSION
        Install-Spinoza | Out-Null
        Should -Invoke Resolve-LatestVersion -Times 1
    }
    It 'reports an install where nothing was there before' {
        Install-Spinoza | Should -Contain "Installed spinoza v9.9.9 in $($script:work)"
    }
    It 'reports an update over whatever was installed before' {
        Mock Get-InstalledVersion { 'v9.9.8' }
        Install-Spinoza | Should -Contain "Updated spinoza v9.9.8 -> v9.9.9 in $($script:work)"
    }
    It 'refuses an archive that carries no binary' {
        Mock Expand-Archive { New-Item -ItemType Directory -Path $DestinationPath -Force | Out-Null }
        { Install-Spinoza } | Should -Throw '*did not contain a spinoza.exe*'
    }
    It 'leaves the app alone when the caller asked it to' {
        $env:SPINOZA_SKIP_APP = '1'
        Install-Spinoza | Out-Null
        Should -Invoke Install-App -Times 0
    }
    It 'installs the app by default' {
        Install-Spinoza | Out-Null
        Should -Invoke Install-App -Times 1
    }
    It 'says where the app landed when one was installed' {
        Mock Install-App { $true }
        Install-Spinoza | Should -Contain "Installed the Spinoza app in $($script:work)\app, and in the Start menu"
    }
    It 'puts the install directory on the path' {
        Install-Spinoza | Out-Null
        Should -Invoke Add-ToPath -Times 1 -ParameterFilter { $Directory -eq $script:work }
    }
}

Describe 'Save-Download' {
    It 'asks for the file with the agent the release page sees' {
        Mock Invoke-WebRequest { }
        Save-Download -Uri 'https://example.invalid/spinoza.zip' -Path 'C:\temp\spinoza.zip'
        Should -Invoke Invoke-WebRequest -Times 1 -ParameterFilter {
            $Uri -eq 'https://example.invalid/spinoza.zip' -and $OutFile -eq 'C:\temp\spinoza.zip' -and $UserAgent -eq 'spinoza-install'
        }
    }
}

Describe 'Add-StartMenuShortcut' {
    It 'does nothing where this account has no start menu' {
        $appdata = $env:APPDATA
        try {
            $env:APPDATA = Join-Path ([System.IO.Path]::GetTempPath()) ('absent-' + [System.Guid]::NewGuid().ToString('N'))
            { Add-StartMenuShortcut -Target 'C:\Apps\spinoza\app\Spinoza.exe' } | Should -Not -Throw
        }
        finally {
            $env:APPDATA = $appdata
        }
    }
}

Describe 'Get-UserPath' {
    BeforeAll {
        $script:onWindows = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)
    }
    It 'reads the path this account carries, unexpanded' -Skip:(-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
        $current = Get-UserPath
        $current.Value | Should -Not -BeNullOrEmpty
        $current.Kind | Should -BeIn @('String', 'ExpandString')
    }
}
