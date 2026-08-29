BeforeAll {
    . (Join-Path (Join-Path (Join-Path $PSScriptRoot '..') '..') 'install.ps1')

    $script:OnWindows = [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)

    function NewWorkspace {
        $work = Join-Path ([System.IO.Path]::GetTempPath()) ('spinoza-tests-' + [System.Guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $work -Force | Out-Null
        return $work
    }

    function NewRelease {
        param([string]$Version = 'v9.9.9', [string]$Arch = 'amd64')
        $release = NewWorkspace
        $staging = Join-Path $release 'staging'
        New-Item -ItemType Directory -Path $staging -Force | Out-Null

        Set-Content -LiteralPath (Join-Path $staging 'spinoza.exe') -Value "spinoza $Version" -NoNewline
        Set-Content -LiteralPath (Join-Path $staging 'LICENSE') -Value 'a licence' -NoNewline
        $binaryZip = Join-Path $release "spinoza_${Version}_windows_${Arch}.zip"
        Compress-Archive -Path (Join-Path $staging '*') -DestinationPath $binaryZip -Force

        $appStaging = Join-Path $release 'app-staging'
        New-Item -ItemType Directory -Path $appStaging -Force | Out-Null
        Set-Content -LiteralPath (Join-Path $appStaging 'Spinoza.exe') -Value "desktop $Version" -NoNewline
        $appZip = Join-Path $release "spinoza_${Version}_windows_${Arch}_app.zip"
        Compress-Archive -Path (Join-Path $appStaging '*') -DestinationPath $appZip -Force

        WriteChecksums -Release $release
        return $release
    }

    function WriteChecksums {
        param([string]$Release)
        $lines = @()
        foreach ($archive in Get-ChildItem -LiteralPath $Release -Filter '*.zip') {
            $lines += "$(Get-Sha256 -Path $archive.FullName)  $($archive.Name)"
        }
        Set-Content -LiteralPath (Join-Path $Release 'checksums.txt') -Value $lines
    }

    function NewStartMenu {
        $appdata = NewWorkspace
        if ($script:OnWindows) {
            $menu = Join-Path $appdata (Join-Path 'Microsoft' (Join-Path 'Windows' (Join-Path 'Start Menu' 'Programs')))
            New-Item -ItemType Directory -Path $menu -Force | Out-Null
        }
        return $appdata
    }

    function NewFakeGh {
        param([int]$ExitCode = 0, [string]$Log)
        $bin = NewWorkspace
        if ($script:OnWindows) {
            $script = Join-Path $bin 'gh.cmd'
            Set-Content -LiteralPath $script -Value @(
                '@echo off',
                "echo %* >> `"$Log`"",
                "exit /b $ExitCode"
            )
        }
        else {
            $script = Join-Path $bin 'gh'
            Set-Content -LiteralPath $script -Value @(
                '#!/bin/sh',
                "echo `"`$@`" >> '$Log'",
                "exit $ExitCode"
            )
            chmod +x $script
        }
        return $bin
    }
}

# configuration the release layout depends on

Describe 'the release this installer is pinned to' {
    It 'names the repository the binaries are published from' {
        $script:Repo | Should -Be 'sophotechlabs/spinoza'
    }
    It 'builds the releases url from that repository' {
        $script:Releases | Should -Be 'https://github.com/sophotechlabs/spinoza/releases'
    }
    It 'writes the path into the key windows keeps user environment in' {
        $script:EnvironmentKey | Should -Be 'Environment'
    }
}

# input construction

Describe 'Resolve-Architecture' {
    It 'maps what this machine reports onto a published architecture' {
        Resolve-Architecture | Should -BeIn @('amd64', 'arm64')
    }
    It 'maps the name a 32-bit shell reports on a 64-bit machine' {
        Mock Get-OSArchitecture { 'AMD64' }
        Resolve-Architecture | Should -Be 'amd64'
    }
    It 'maps the name the runtime reports on an arm machine' {
        Mock Get-OSArchitecture { 'Arm64' }
        Resolve-Architecture | Should -Be 'arm64'
    }
    It 'refuses an architecture with no published build' {
        Mock Get-OSArchitecture { 'MIPS' }
        { Resolve-Architecture } | Should -Throw '*MIPS is not supported*'
    }
}

Describe 'Get-OSArchitecture' {
    It 'names an architecture on this machine' {
        Get-OSArchitecture | Should -Not -BeNullOrEmpty
    }
}

Describe 'Get-InstallDirectory' {
    BeforeEach {
        $script:previousInstallDir = $env:SPINOZA_INSTALL_DIR
        $script:previousLocalAppData = $env:LOCALAPPDATA
    }
    AfterEach {
        $env:SPINOZA_INSTALL_DIR = $script:previousInstallDir
        $env:LOCALAPPDATA = $script:previousLocalAppData
    }

    It 'takes the directory the caller chose' {
        $env:SPINOZA_INSTALL_DIR = 'C:\chosen'
        Get-InstallDirectory | Should -Be 'C:\chosen'
    }
    It 'falls back to a programs directory under the local app data' {
        $env:SPINOZA_INSTALL_DIR = ''
        $env:LOCALAPPDATA = Join-Path ([System.IO.Path]::GetTempPath()) 'AppData'
        Get-InstallDirectory | Should -Be (Join-Path $env:LOCALAPPDATA (Join-Path 'Programs' 'spinoza'))
    }
}

# output parsing

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
    It 'hashes an empty file to the vector for no bytes at all' {
        $work = NewWorkspace
        try {
            $file = Join-Path $work 'empty.txt'
            [System.IO.File]::WriteAllText($file, '')
            Get-Sha256 -Path $file | Should -Be 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'
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
        $script:list = Join-Path $script:work 'checksums.txt'
        Set-Content -LiteralPath $script:list -Value @(
            "$(Get-Sha256 -Path $script:payload)  spinoza_v9.9.9_windows_amd64.zip",
            '0000000000000000000000000000000000000000000000000000000000000000  spinoza_v9.9.9_linux_amd64.tar.gz'
        )
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

Describe 'Resolve-LatestVersion' {
    It 'reads the tag out of the redirect' {
        Mock Get-RedirectLocation { 'https://github.com/sophotechlabs/spinoza/releases/tag/v1.17.0' }
        Resolve-LatestVersion | Should -Be 'v1.17.0'
    }
    It 'asks the latest url for that redirect' {
        Mock Get-RedirectLocation { 'https://github.com/sophotechlabs/spinoza/releases/tag/v1.17.0' }
        Resolve-LatestVersion | Out-Null
        Should -Invoke Get-RedirectLocation -Times 1 -ParameterFilter { $Uri -eq 'https://github.com/sophotechlabs/spinoza/releases/latest' }
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
    It 'reports what the installed binary prints' -Tag 'unix' -Skip:([System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
        $work = NewWorkspace
        try {
            $stub = Join-Path $work 'spinoza'
            Set-Content -LiteralPath $stub -Value @('#!/bin/sh', 'echo v9.9.8')
            chmod +x $stub
            Get-InstalledVersion -Path $stub | Should -Be 'v9.9.8'
        }
        finally {
            Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

# file placement

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

# the path this installer edits

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

Describe 'the registry the installer writes' -Tag 'windows' -Skip:(-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
    BeforeEach {
        $script:scratch = 'Environment\SpinozaTests\' + [System.Guid]::NewGuid().ToString('N')
    }
    AfterEach {
        [Microsoft.Win32.Registry]::CurrentUser.DeleteSubKeyTree($script:scratch, $false)
    }

    It 'reads back what it wrote' {
        Set-UserPath -Value 'C:\one;C:\two' -Kind ([Microsoft.Win32.RegistryValueKind]::ExpandString) -Key $script:scratch
        $read = Get-UserPath -Key $script:scratch
        $read.Value | Should -Be 'C:\one;C:\two'
        $read.Kind | Should -Be 'ExpandString'
    }
    It 'stores a variable without expanding it, so the path survives the round trip' {
        Set-UserPath -Value '%USERPROFILE%\bin' -Kind ([Microsoft.Win32.RegistryValueKind]::ExpandString) -Key $script:scratch
        (Get-UserPath -Key $script:scratch).Value | Should -Be '%USERPROFILE%\bin'
    }
    It 'keeps a plain string value plain' {
        Set-UserPath -Value 'C:\one' -Kind ([Microsoft.Win32.RegistryValueKind]::String) -Key $script:scratch
        (Get-UserPath -Key $script:scratch).Kind | Should -Be 'String'
    }
    It 'creates the key where the account has none' {
        { Set-UserPath -Value 'C:\one' -Kind ([Microsoft.Win32.RegistryValueKind]::ExpandString) -Key $script:scratch } | Should -Not -Throw
    }
    It 'reads an account with no path as empty rather than failing' {
        $absent = 'Environment\SpinozaTests\' + [System.Guid]::NewGuid().ToString('N')
        $read = Get-UserPath -Key $absent
        $read.Value | Should -Be ''
        $read.Kind | Should -Be 'ExpandString'
    }
    It 'adds and removes a directory against the real registry' {
        Set-UserPath -Value 'C:\Windows' -Kind ([Microsoft.Win32.RegistryValueKind]::ExpandString) -Key $script:scratch
        $script:EnvironmentKey = $script:scratch
        try {
            Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
            (Get-UserPath -Key $script:scratch).Value | Should -Be 'C:\Windows;C:\Apps\spinoza'
            Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeFalse
            Remove-FromPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
            (Get-UserPath -Key $script:scratch).Value | Should -Be 'C:\Windows'
        }
        finally {
            $script:EnvironmentKey = 'Environment'
        }
    }
}

Describe 'Add-ToPath' {
    BeforeEach {
        Mock Get-UserPath { [pscustomobject]@{ Value = $script:currentPath; Kind = 'ExpandString' } }
        Mock Set-UserPath { }
    }

    It 'appends to a path that does not carry the directory' {
        $script:currentPath = 'C:\Windows'
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq 'C:\Windows;C:\Apps\spinoza' }
    }
    It 'leaves the path alone where the directory is already on it' {
        $script:currentPath = 'C:\Windows;C:\Apps\spinoza'
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeFalse
        Should -Invoke Set-UserPath -Times 0
    }
    It 'writes a bare directory where the account has no path yet' {
        $script:currentPath = ''
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq 'C:\Apps\spinoza' }
    }
    It 'does not leave a doubled separator behind a trailing one' {
        $script:currentPath = 'C:\Windows;'
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq 'C:\Windows;C:\Apps\spinoza' }
    }
    It 'keeps the value kind the registry already had' {
        $script:currentPath = '%USERPROFILE%\bin'
        Add-ToPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Kind -eq 'ExpandString' }
    }
}

Describe 'Remove-FromPath' {
    BeforeEach {
        Mock Get-UserPath { [pscustomobject]@{ Value = $script:currentPath; Kind = 'ExpandString' } }
        Mock Set-UserPath { }
    }

    It 'takes the directory off a path that carries it' {
        $script:currentPath = 'C:\Windows;C:\Apps\spinoza;C:\Other'
        Remove-FromPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq 'C:\Windows;C:\Other' }
    }
    It 'leaves a path that never carried it alone' {
        $script:currentPath = 'C:\Windows'
        Remove-FromPath -Directory 'C:\Apps\spinoza' | Should -BeFalse
        Should -Invoke Set-UserPath -Times 0
    }
    It 'takes it off however it was spelled' {
        $script:currentPath = 'C:\Windows;C:\APPS\SPINOZA\'
        Remove-FromPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq 'C:\Windows' }
    }
    It 'does not leave an empty entry behind the one it took out' {
        $script:currentPath = 'C:\Apps\spinoza'
        Remove-FromPath -Directory 'C:\Apps\spinoza' | Should -BeTrue
        Should -Invoke Set-UserPath -Times 1 -ParameterFilter { $Value -eq '' }
    }
}

# the gh call the provenance check shells out to

Describe 'Test-Attestation' {
    BeforeEach {
        $script:work = NewWorkspace
        $script:log = Join-Path $script:work 'argv.txt'
        $script:asset = Join-Path $script:work 'spinoza_v9.9.9_windows_amd64.zip'
        Set-Content -LiteralPath $script:asset -Value 'payload' -NoNewline
        $script:previousPath = $env:PATH
        $script:previousAsk = $env:SPINOZA_VERIFY_ATTESTATION
    }
    AfterEach {
        $env:PATH = $script:previousPath
        $env:SPINOZA_VERIFY_ATTESTATION = $script:previousAsk
        Remove-Item -LiteralPath $script:work -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'checks nothing unless the caller asked for it' {
        $env:SPINOZA_VERIFY_ATTESTATION = ''
        $bin = NewFakeGh -ExitCode 0 -Log $script:log
        $env:PATH = $bin + [System.IO.Path]::PathSeparator + $env:PATH
        Test-Attestation -Path $script:asset -Name 'spinoza_v9.9.9_windows_amd64.zip' | Should -BeFalse
        Test-Path -LiteralPath $script:log | Should -BeFalse
    }
    It 'accepts an asset gh vouches for' {
        $env:SPINOZA_VERIFY_ATTESTATION = '1'
        $bin = NewFakeGh -ExitCode 0 -Log $script:log
        $env:PATH = $bin + [System.IO.Path]::PathSeparator + $env:PATH
        Test-Attestation -Path $script:asset -Name 'spinoza_v9.9.9_windows_amd64.zip' | Should -BeTrue
    }
    It 'asks gh to verify that asset against this repository' {
        $env:SPINOZA_VERIFY_ATTESTATION = '1'
        $bin = NewFakeGh -ExitCode 0 -Log $script:log
        $env:PATH = $bin + [System.IO.Path]::PathSeparator + $env:PATH
        Test-Attestation -Path $script:asset -Name 'spinoza_v9.9.9_windows_amd64.zip' | Out-Null
        $argv = Get-Content -LiteralPath $script:log -Raw
        $argv | Should -Match 'attestation verify'
        $argv | Should -Match ([regex]::Escape($script:asset))
        $argv | Should -Match '--repo sophotechlabs/spinoza'
    }
    It 'refuses an asset gh will not vouch for' {
        $env:SPINOZA_VERIFY_ATTESTATION = '1'
        $bin = NewFakeGh -ExitCode 1 -Log $script:log
        $env:PATH = $bin + [System.IO.Path]::PathSeparator + $env:PATH
        { Test-Attestation -Path $script:asset -Name 'spinoza_v9.9.9_windows_amd64.zip' } | Should -Throw '*carries no build provenance*'
    }
    It 'refuses to pretend where gh is not installed' {
        $env:SPINOZA_VERIFY_ATTESTATION = '1'
        Mock Get-Command { $null } -ParameterFilter { $Name -eq 'gh' }
        { Test-Attestation -Path $script:asset -Name 'spinoza_v9.9.9_windows_amd64.zip' } | Should -Throw '*gh is not on PATH*'
    }
}

# installing, end to end, against a real release

Describe 'installing a release' {
    BeforeEach {
        $script:release = NewRelease
        $script:target = NewWorkspace
        $script:previous = @{
            InstallDir = $env:SPINOZA_INSTALL_DIR
            Version    = $env:SPINOZA_VERSION
            SkipApp    = $env:SPINOZA_SKIP_APP
            Path       = $env:Path
            AppData    = $env:APPDATA
        }
        $env:SPINOZA_INSTALL_DIR = $script:target
        $env:SPINOZA_VERSION = 'v9.9.9'
        $env:SPINOZA_SKIP_APP = ''
        $script:userPath = 'C:\Windows'
        Mock Get-OSArchitecture { 'X64' }
        Mock Get-UserPath { [pscustomobject]@{ Value = $script:userPath; Kind = 'ExpandString' } }
        Mock Set-UserPath { $script:userPath = $Value }
        $script:previous.AppData = $env:APPDATA
        $env:APPDATA = NewStartMenu
        Mock Invoke-WebRequest {
            $name = Split-Path -Leaf ([uri]$Uri).AbsolutePath
            Copy-Item -LiteralPath (Join-Path $script:release $name) -Destination $OutFile -Force
        }
    }
    AfterEach {
        $env:APPDATA = $script:previous.AppData
        $env:SPINOZA_INSTALL_DIR = $script:previous.InstallDir
        $env:SPINOZA_VERSION = $script:previous.Version
        $env:SPINOZA_SKIP_APP = $script:previous.SkipApp
        $env:Path = $script:previous.Path
        Remove-Item -LiteralPath $script:release -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $script:target -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'unpacks the published binary onto disk' {
        Install-Spinoza | Out-Null
        Get-Content -LiteralPath (Join-Path $script:target 'spinoza.exe') -Raw | Should -Be 'spinoza v9.9.9'
    }
    It 'downloads the asset named for the pinned version and this architecture' {
        Install-Spinoza | Out-Null
        Should -Invoke Invoke-WebRequest -Times 1 -ParameterFilter {
            $Uri -eq 'https://github.com/sophotechlabs/spinoza/releases/download/v9.9.9/spinoza_v9.9.9_windows_amd64.zip'
        }
    }
    It 'puts the desktop app beside the binary rather than over it' {
        Install-Spinoza | Out-Null
        Get-Content -LiteralPath (Join-Path (Join-Path $script:target 'app') 'Spinoza.exe') -Raw | Should -Be 'desktop v9.9.9'
        Get-Content -LiteralPath (Join-Path $script:target 'spinoza.exe') -Raw | Should -Be 'spinoza v9.9.9'
    }
    It 'leaves the app alone when the caller asked it to' {
        $env:SPINOZA_SKIP_APP = '1'
        Install-Spinoza | Out-Null
        Test-Path -LiteralPath (Join-Path $script:target 'app') | Should -BeFalse
    }
    It 'puts the install directory on the path' {
        Install-Spinoza | Out-Null
        $script:userPath | Should -Be "C:\Windows;$($script:target)"
    }
    It 'says what it installed and where' {
        $said = (Install-Spinoza) -join "`n"
        $said | Should -Match ([regex]::Escape("Installed spinoza v9.9.9 in $($script:target)"))
    }
    It 'refuses a payload whose bytes do not match the published checksum' {
        Set-Content -LiteralPath (Join-Path $script:release 'spinoza_v9.9.9_windows_amd64.zip') -Value 'tampered' -NoNewline
        { Install-Spinoza } | Should -Throw '*checksum mismatch*'
        Test-Path -LiteralPath (Join-Path $script:target 'spinoza.exe') | Should -BeFalse
    }
    It 'refuses a release that lists no checksum for the asset' {
        Set-Content -LiteralPath (Join-Path $script:release 'checksums.txt') -Value 'aaaa  something-else.zip'
        { Install-Spinoza } | Should -Throw '*not listed*'
    }
    It 'refuses an archive that carries no binary' {
        $empty = Join-Path $script:release 'empty'
        New-Item -ItemType Directory -Path $empty -Force | Out-Null
        Set-Content -LiteralPath (Join-Path $empty 'README.txt') -Value 'no binary here' -NoNewline
        Compress-Archive -Path (Join-Path $empty '*') -DestinationPath (Join-Path $script:release 'spinoza_v9.9.9_windows_amd64.zip') -Force
        WriteChecksums -Release $script:release
        { Install-Spinoza } | Should -Throw '*did not contain a spinoza.exe*'
    }
    It 'asks the release page for a version where none was pinned' {
        $env:SPINOZA_VERSION = ''
        Mock Get-RedirectLocation { 'https://github.com/sophotechlabs/spinoza/releases/tag/v9.9.9' }
        Install-Spinoza | Out-Null
        Should -Invoke Get-RedirectLocation -Times 1
        Test-Path -LiteralPath (Join-Path $script:target 'spinoza.exe') | Should -BeTrue
    }
    It 'replaces an older install and clears the copy it retired' {
        Install-Spinoza | Out-Null
        Remove-Item -LiteralPath $script:release -Recurse -Force
        $script:release = NewRelease -Version 'v9.9.9'
        Set-Content -LiteralPath (Join-Path (Join-Path $script:release 'staging') 'spinoza.exe') -Value 'spinoza newer' -NoNewline
        Compress-Archive -Path (Join-Path (Join-Path $script:release 'staging') '*') -DestinationPath (Join-Path $script:release 'spinoza_v9.9.9_windows_amd64.zip') -Force
        WriteChecksums -Release $script:release
        Install-Spinoza | Out-Null
        Get-Content -LiteralPath (Join-Path $script:target 'spinoza.exe') -Raw | Should -Be 'spinoza newer'
        Test-Path -LiteralPath (Join-Path $script:target 'spinoza.exe.old') | Should -BeFalse
    }
    It 'checks provenance before unpacking when the caller asked for it' {
        $log = Join-Path $script:target 'argv.txt'
        $bin = NewFakeGh -ExitCode 1 -Log $log
        $previousPath = $env:PATH
        $previousAsk = $env:SPINOZA_VERIFY_ATTESTATION
        try {
            $env:PATH = $bin + [System.IO.Path]::PathSeparator + $env:PATH
            $env:SPINOZA_VERIFY_ATTESTATION = '1'
            { Install-Spinoza } | Should -Throw '*carries no build provenance*'
            Test-Path -LiteralPath (Join-Path $script:target 'spinoza.exe') | Should -BeFalse
        }
        finally {
            $env:PATH = $previousPath
            $env:SPINOZA_VERIFY_ATTESTATION = $previousAsk
        }
    }
}

# removing what the install put down

Describe 'uninstalling after a real install' {
    BeforeEach {
        $script:release = NewRelease
        $script:target = NewWorkspace
        $script:previousInstallDir = $env:SPINOZA_INSTALL_DIR
        $script:previousVersion = $env:SPINOZA_VERSION
        $script:previousPath = $env:Path
        $env:SPINOZA_INSTALL_DIR = $script:target
        $env:SPINOZA_VERSION = 'v9.9.9'
        $script:userPath = 'C:\Windows'
        Mock Get-OSArchitecture { 'X64' }
        Mock Get-UserPath { [pscustomobject]@{ Value = $script:userPath; Kind = 'ExpandString' } }
        Mock Set-UserPath { $script:userPath = $Value }
        $script:previousAppData = $env:APPDATA
        $env:APPDATA = NewStartMenu
        Mock Invoke-WebRequest {
            $name = Split-Path -Leaf ([uri]$Uri).AbsolutePath
            Copy-Item -LiteralPath (Join-Path $script:release $name) -Destination $OutFile -Force
        }
    }
    AfterEach {
        $env:APPDATA = $script:previousAppData
        $env:SPINOZA_INSTALL_DIR = $script:previousInstallDir
        $env:SPINOZA_VERSION = $script:previousVersion
        $env:Path = $script:previousPath
        Remove-Item -LiteralPath $script:release -Recurse -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $script:target -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'takes back everything the install put down' {
        Install-Spinoza | Out-Null
        Uninstall-Spinoza | Out-Null
        Test-Path -LiteralPath (Join-Path $script:target 'spinoza.exe') | Should -BeFalse
        Test-Path -LiteralPath (Join-Path $script:target 'app') | Should -BeFalse
        Test-Path -LiteralPath $script:target | Should -BeFalse
        $script:userPath | Should -Be 'C:\Windows'
    }
    It 'names what it removed' {
        Install-Spinoza | Out-Null
        $said = (Uninstall-Spinoza) -join "`n"
        $said | Should -Match 'spinoza\.exe'
        $said | Should -Match 'the desktop app'
        $said | Should -Match 'the PATH entry'
    }
    It 'says so where there is nothing installed to remove' {
        ((Uninstall-Spinoza) -join "`n") | Should -Match 'not installed'
    }
    It 'clears a retired copy an interrupted update left behind' {
        Install-Spinoza | Out-Null
        Set-Content -LiteralPath (Join-Path $script:target 'spinoza.exe.old') -Value 'stale' -NoNewline
        ((Uninstall-Spinoza) -join "`n") | Should -Match 'spinoza\.exe\.old'
    }
    It 'leaves the directory where something else still lives in it' {
        Install-Spinoza | Out-Null
        Set-Content -LiteralPath (Join-Path $script:target 'notes.txt') -Value 'mine' -NoNewline
        Uninstall-Spinoza | Out-Null
        Test-Path -LiteralPath (Join-Path $script:target 'notes.txt') | Should -BeTrue
    }
}

# the start menu entry

Describe 'Get-StartMenuShortcut' {
    BeforeEach {
        $script:previousAppData = $env:APPDATA
        $script:appdata = NewWorkspace
        $env:APPDATA = $script:appdata
    }
    AfterEach {
        $env:APPDATA = $script:previousAppData
        Remove-Item -LiteralPath $script:appdata -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'names no shortcut where this account has no start menu' {
        Get-StartMenuShortcut | Should -Be ''
    }
    It 'names the shortcut inside the start menu programs folder' {
        $menu = Join-Path $script:appdata (Join-Path 'Microsoft' (Join-Path 'Windows' (Join-Path 'Start Menu' 'Programs')))
        New-Item -ItemType Directory -Path $menu -Force | Out-Null
        Get-StartMenuShortcut | Should -Be (Join-Path $menu 'Spinoza.lnk')
    }
}

Describe 'Add-StartMenuShortcut' {
    BeforeEach {
        $script:previousAppData = $env:APPDATA
        $script:appdata = NewWorkspace
        $env:APPDATA = $script:appdata
    }
    AfterEach {
        $env:APPDATA = $script:previousAppData
        Remove-Item -LiteralPath $script:appdata -Recurse -Force -ErrorAction SilentlyContinue
    }

    It 'writes nothing where this account has no start menu' {
        { Add-StartMenuShortcut -Target 'C:\Apps\spinoza\app\Spinoza.exe' } | Should -Not -Throw
        Get-StartMenuShortcut | Should -Be ''
    }
    It 'writes a shortcut that points at the app' -Tag 'windows' -Skip:(-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
        $menu = Join-Path $script:appdata (Join-Path 'Microsoft' (Join-Path 'Windows' (Join-Path 'Start Menu' 'Programs')))
        New-Item -ItemType Directory -Path $menu -Force | Out-Null
        $target = Join-Path $script:appdata 'Spinoza.exe'
        Set-Content -LiteralPath $target -Value 'app' -NoNewline

        Add-StartMenuShortcut -Target $target

        $link = Join-Path $menu 'Spinoza.lnk'
        Test-Path -LiteralPath $link | Should -BeTrue
        $shell = New-Object -ComObject WScript.Shell
        $shell.CreateShortcut($link).TargetPath | Should -Be $target
    }
}

# the entry point a piped install actually runs

Describe 'running install.ps1 as a script' -Tag 'windows' -Skip:(-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform([System.Runtime.InteropServices.OSPlatform]::Windows)) {
    BeforeEach {
        $script:target = NewWorkspace
        $script:installer = Join-Path (Join-Path (Join-Path $PSScriptRoot '..') '..') 'install.ps1'
    }
    AfterEach { Remove-Item -LiteralPath $script:target -Recurse -Force -ErrorAction SilentlyContinue }

    It 'dispatches to the uninstall when the caller asked for one' {
        $shell = (Get-Process -Id $PID).Path
        $said = & $shell -NoProfile -Command "`$env:SPINOZA_UNINSTALL='1'; `$env:SPINOZA_INSTALL_DIR='$($script:target)'; & '$($script:installer)'" 2>&1
        (($said) -join "`n") | Should -Match 'not installed'
    }
}

# the shape of the script itself

Describe 'the installer source' {
    BeforeAll {
        $script:sources = @(
            (Join-Path (Join-Path (Join-Path $PSScriptRoot '..') '..') 'install.ps1'),
            (Join-Path (Join-Path $PSScriptRoot '..') 'smoke.ps1'),
            (Join-Path (Join-Path $PSScriptRoot '..') 'pester.ps1'),
            (Join-Path $PSScriptRoot 'windows.ps1'),
            (Join-Path $PSScriptRoot 'install.Tests.ps1')
        )
    }

    It 'never joins a path with more segments than windows powershell accepts' {
        $overlong = @()
        foreach ($source in $script:sources) {
            $tokens = $null
            $errors = $null
            $tree = [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path $source), [ref]$tokens, [ref]$errors)
            $calls = $tree.FindAll({ $args[0] -is [System.Management.Automation.Language.CommandAst] }, $true)
            foreach ($call in $calls) {
                if ($call.GetCommandName() -ne 'Join-Path') {
                    continue
                }
                $named = $call.CommandElements | Where-Object { $_ -is [System.Management.Automation.Language.CommandParameterAst] }
                if ($named) {
                    continue
                }
                if (($call.CommandElements.Count - 1) -le 2) {
                    continue
                }
                $overlong += "$(Split-Path -Leaf $source) line $($call.Extent.StartLineNumber): Join-Path takes $($call.CommandElements.Count - 1) segments, and 5.1 accepts two"
            }
        }
        $overlong -join "`n" | Should -Be ''
    }

    It 'never assigns to a variable a function already took as a parameter' {
        $shadowed = @()
        foreach ($source in $script:sources) {
            $tokens = $null
            $errors = $null
            $tree = [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path $source), [ref]$tokens, [ref]$errors)
            $functions = $tree.FindAll({ $args[0] -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true)
            foreach ($function in $functions) {
                $parameters = @()
                if ($function.Body.ParamBlock) {
                    $parameters = @($function.Body.ParamBlock.Parameters.Name.VariablePath.UserPath)
                }
                if ($parameters.Count -eq 0) {
                    continue
                }
                $assignments = $function.Body.FindAll({ $args[0] -is [System.Management.Automation.Language.AssignmentStatementAst] }, $true)
                foreach ($assignment in $assignments) {
                    if ($assignment.Left -isnot [System.Management.Automation.Language.VariableExpressionAst]) {
                        continue
                    }
                    $assigned = $assignment.Left.VariablePath.UserPath
                    if ($parameters -contains $assigned) {
                        $shadowed += "$(Split-Path -Leaf $source) line $($assignment.Extent.StartLineNumber): $($function.Name) assigns to its own parameter `$$assigned"
                    }
                }
            }
        }
        $shadowed -join "`n" | Should -Be ''
    }
}
