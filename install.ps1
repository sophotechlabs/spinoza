#Requires -Version 5.1

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$script:Repo = 'sophotechlabs/spinoza'
$script:Releases = "https://github.com/$script:Repo/releases"

function Get-OSArchitecture {
    try {
        return [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    }
    catch {
        $null = $_
    }
    if ($env:PROCESSOR_ARCHITEW6432) {
        return $env:PROCESSOR_ARCHITEW6432
    }
    return $env:PROCESSOR_ARCHITECTURE
}

function Resolve-Architecture {
    $known = @{
        'X64'   = 'amd64'
        'AMD64' = 'amd64'
        'Arm64' = 'arm64'
    }
    $reported = Get-OSArchitecture
    if ($known.ContainsKey($reported)) {
        return $known[$reported]
    }
    throw "$reported is not supported; see $script:Releases for the other builds"
}

function Resolve-LatestVersion {
    $request = [System.Net.HttpWebRequest]::Create("$script:Releases/latest")
    $request.AllowAutoRedirect = $false
    $request.UserAgent = 'spinoza-install'
    $response = $request.GetResponse()
    try {
        $location = $response.Headers['Location']
    }
    finally {
        $response.Close()
    }
    if ($location -notmatch '/releases/tag/(.+)$') {
        throw "$script:Repo has no published release yet; set SPINOZA_VERSION to pick one"
    }
    return $Matches[1]
}

function Save-Download {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$Path
    )
    Invoke-WebRequest -Uri $Uri -OutFile $Path -UseBasicParsing -UserAgent 'spinoza-install'
}

function Get-ListedChecksum {
    param(
        [Parameter(Mandatory = $true)][string]$ListPath,
        [Parameter(Mandatory = $true)][string]$Name
    )
    foreach ($line in Get-Content -LiteralPath $ListPath) {
        $fields = $line -split '\s+', 2
        if ($fields.Count -ne 2) {
            continue
        }
        if ($fields[1].Trim() -eq $Name) {
            return $fields[0].Trim()
        }
    }
    return ''
}

function Test-Checksum {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$ListPath
    )
    $expected = Get-ListedChecksum -ListPath $ListPath -Name $Name
    if ($expected -eq '') {
        throw "$Name is not listed in checksums.txt"
    }
    $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
    if ($expected -ne $actual.ToLowerInvariant()) {
        throw "checksum mismatch for $Name"
    }
}

function Get-InstallDirectory {
    if ($env:SPINOZA_INSTALL_DIR) {
        return $env:SPINOZA_INSTALL_DIR
    }
    return (Join-Path $env:LOCALAPPDATA 'Programs\spinoza')
}

function Get-InstalledVersion {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) {
        return ''
    }
    try {
        return (& $Path --version 2>$null | Select-Object -First 1)
    }
    catch {
        return ''
    }
}

function Install-Binary {
    param(
        [Parameter(Mandatory = $true)][string]$Source,
        [Parameter(Mandatory = $true)][string]$Target
    )
    $retired = "$Target.old"
    if (Test-Path -LiteralPath $retired) {
        Remove-Item -LiteralPath $retired -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $Target) {
        Move-Item -LiteralPath $Target -Destination $retired -Force
    }
    try {
        Copy-Item -LiteralPath $Source -Destination $Target -Force
    }
    catch {
        if (Test-Path -LiteralPath $retired) {
            Move-Item -LiteralPath $retired -Destination $Target -Force
        }
        throw
    }
    Remove-Item -LiteralPath $retired -Force -ErrorAction SilentlyContinue
}

function Test-OnPath {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Path
    )
    $wanted = $Directory.TrimEnd('\')
    foreach ($entry in $Path -split ';') {
        if ($entry.Trim().TrimEnd('\') -eq $wanted) {
            return $true
        }
    }
    return $false
}

function Add-ToPath {
    param([Parameter(Mandatory = $true)][string]$Directory)
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
        $current = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        if (Test-OnPath -Directory $Directory -Path $current) {
            return $false
        }
        $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
        if ($current -ne '') {
            $kind = $key.GetValueKind('Path')
        }
        $updated = $Directory
        if ($current.TrimEnd(';') -ne '') {
            $updated = $current.TrimEnd(';') + ';' + $Directory
        }
        $key.SetValue('Path', $updated, $kind)
    }
    finally {
        $key.Close()
    }
    return $true
}

function Install-App {
    param(
        [Parameter(Mandatory = $true)][string]$Temp,
        [Parameter(Mandatory = $true)][string]$Version,
        [Parameter(Mandatory = $true)][string]$Arch,
        [Parameter(Mandatory = $true)][string]$Directory
    )
    $asset = "spinoza_${Version}_windows_${Arch}_app.zip"
    $listed = Get-ListedChecksum -ListPath (Join-Path $Temp 'checksums.txt') -Name $asset
    if ($listed -eq '') {
        return $false
    }
    Save-Download -Uri "$script:Releases/download/$Version/$asset" -Path (Join-Path $Temp $asset)
    Test-Checksum -Path (Join-Path $Temp $asset) -Name $asset -ListPath (Join-Path $Temp 'checksums.txt')
    Expand-Archive -LiteralPath (Join-Path $Temp $asset) -DestinationPath (Join-Path $Temp 'app') -Force
    $bundled = Join-Path $Temp 'app\Spinoza.exe'
    if (-not (Test-Path -LiteralPath $bundled)) {
        throw 'the app archive did not contain Spinoza.exe'
    }
    $appDirectory = Join-Path $Directory 'app'
    New-Item -ItemType Directory -Path $appDirectory -Force | Out-Null
    $installed = Join-Path $appDirectory 'Spinoza.exe'
    Install-Binary -Source $bundled -Target $installed
    Add-StartMenuShortcut -Target $installed
    return $true
}

function Add-StartMenuShortcut {
    param([Parameter(Mandatory = $true)][string]$Target)
    $menu = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'
    if (-not (Test-Path -LiteralPath $menu)) {
        return
    }
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut((Join-Path $menu 'Spinoza.lnk'))
    $shortcut.TargetPath = $Target
    $shortcut.WorkingDirectory = Split-Path -Parent $Target
    $shortcut.Description = 'Spinoza'
    $shortcut.Save()
}

function Install-Spinoza {
    $arch = Resolve-Architecture
    $version = $env:SPINOZA_VERSION
    if (-not $version) {
        $version = Resolve-LatestVersion
    }
    $asset = "spinoza_${version}_windows_${arch}.zip"
    $directory = Get-InstallDirectory
    $binary = Join-Path $directory 'spinoza.exe'
    $temp = Join-Path ([System.IO.Path]::GetTempPath()) ('spinoza-' + [System.Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $temp -Force | Out-Null
    try {
        Write-Output "Downloading spinoza $version (windows/$arch)"
        Save-Download -Uri "$script:Releases/download/$version/$asset" -Path (Join-Path $temp $asset)
        Save-Download -Uri "$script:Releases/download/$version/checksums.txt" -Path (Join-Path $temp 'checksums.txt')
        Test-Checksum -Path (Join-Path $temp $asset) -Name $asset -ListPath (Join-Path $temp 'checksums.txt')
        Expand-Archive -LiteralPath (Join-Path $temp $asset) -DestinationPath (Join-Path $temp 'unpacked') -Force
        $unpacked = Join-Path $temp 'unpacked\spinoza.exe'
        if (-not (Test-Path -LiteralPath $unpacked)) {
            throw 'the archive did not contain a spinoza.exe'
        }
        $previous = Get-InstalledVersion -Path $binary
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
        Install-Binary -Source $unpacked -Target $binary
        if ($previous -ne '') {
            Write-Output "Updated spinoza $previous -> $version in $directory"
        }
        else {
            Write-Output "Installed spinoza $version in $directory"
        }
        $app = $false
        if (-not $env:SPINOZA_SKIP_APP) {
            $app = Install-App -Temp $temp -Version $version -Arch $arch -Directory $directory
        }
        if ($app) {
            Write-Output "Installed the Spinoza app in $directory\app, and in the Start menu"
        }
        $added = Add-ToPath -Directory $directory
        if ($added) {
            Write-Output "Added $directory to your PATH; open a new terminal to pick it up"
        }
        $env:Path = "$env:Path;$directory"
        Write-Output "Run it in your browser with 'spinoza --open'"
    }
    finally {
        Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
}
catch {
    $null = $_
}

if ($MyInvocation.InvocationName -ne '.') {
    Install-Spinoza
}
