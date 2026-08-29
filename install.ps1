#Requires -Version 5.1

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$script:Repo = 'sophotechlabs/spinoza'
$script:Releases = "https://github.com/$script:Repo/releases"
$script:EnvironmentKey = 'Environment'

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

function Get-RedirectLocation {
    param([Parameter(Mandatory = $true)][string]$Uri)
    $request = [System.Net.HttpWebRequest]::Create($Uri)
    $request.AllowAutoRedirect = $false
    $request.UserAgent = 'spinoza-install'
    $response = $request.GetResponse()
    try {
        return $response.Headers['Location']
    }
    finally {
        $response.Close()
    }
}

function Resolve-LatestVersion {
    $location = Get-RedirectLocation -Uri "$script:Releases/latest"
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

function Get-Sha256 {
    param([Parameter(Mandatory = $true)][string]$Path)
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $sha = [System.Security.Cryptography.SHA256]::Create()
        try {
            return [System.BitConverter]::ToString($sha.ComputeHash($stream)).Replace('-', '').ToLowerInvariant()
        }
        finally {
            $sha.Dispose()
        }
    }
    finally {
        $stream.Dispose()
    }
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
    $actual = Get-Sha256 -Path $Path
    if ($expected -ne $actual) {
        throw "checksum mismatch for $Name"
    }
}

function Get-InstallDirectory {
    if ($env:SPINOZA_INSTALL_DIR) {
        return $env:SPINOZA_INSTALL_DIR
    }
    return (Join-Path $env:LOCALAPPDATA (Join-Path 'Programs' 'spinoza'))
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

function Install-Copyright {
    param(
        [Parameter(Mandatory = $true)][string]$Unpacked,
        [Parameter(Mandatory = $true)][string]$Directory
    )
    $source = Join-Path $Unpacked 'LICENSE'
    if (-not (Test-Path -LiteralPath $source)) {
        return
    }
    Copy-Item -LiteralPath $source -Destination (Join-Path $Directory 'LICENSE.txt') -Force
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

function Get-UserPath {
    param([string]$Key = $script:EnvironmentKey)
    $handle = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($Key, $false)
    if ($null -eq $handle) {
        return [pscustomobject]@{
            Value = ''
            Kind  = [Microsoft.Win32.RegistryValueKind]::ExpandString
        }
    }
    try {
        $value = [string]$handle.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
        if ($value -ne '') {
            $kind = $handle.GetValueKind('Path')
        }
        return [pscustomobject]@{
            Value = $value
            Kind  = $kind
        }
    }
    finally {
        $handle.Close()
    }
}

function Set-UserPath {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '')]
    param(
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Value,
        [Parameter(Mandatory = $true)]$Kind,
        [string]$Key = $script:EnvironmentKey
    )
    $handle = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($Key)
    try {
        $handle.SetValue('Path', $Value, $Kind)
    }
    finally {
        $handle.Close()
    }
}

function Add-ToPath {
    param([Parameter(Mandatory = $true)][string]$Directory)
    $current = Get-UserPath
    if (Test-OnPath -Directory $Directory -Path $current.Value) {
        return $false
    }
    $updated = $Directory
    if ($current.Value.TrimEnd(';') -ne '') {
        $updated = $current.Value.TrimEnd(';') + ';' + $Directory
    }
    Set-UserPath -Value $updated -Kind $current.Kind
    return $true
}

function Remove-FromPath {
    [Diagnostics.CodeAnalysis.SuppressMessageAttribute('PSUseShouldProcessForStateChangingFunctions', '')]
    param([Parameter(Mandatory = $true)][string]$Directory)
    $current = Get-UserPath
    $wanted = $Directory.TrimEnd('\')
    $kept = @()
    $removed = $false
    foreach ($entry in $current.Value -split ';') {
        if ($entry.Trim() -eq '') {
            continue
        }
        if ($entry.Trim().TrimEnd('\') -eq $wanted) {
            $removed = $true
            continue
        }
        $kept += $entry
    }
    if (-not $removed) {
        return $false
    }
    Set-UserPath -Value ($kept -join ';') -Kind $current.Kind
    return $true
}

function Test-Attestation {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Name
    )
    if (-not $env:SPINOZA_VERIFY_ATTESTATION) {
        return $false
    }
    if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
        throw 'SPINOZA_VERIFY_ATTESTATION is set but gh is not on PATH'
    }
    & gh attestation verify $Path --repo $script:Repo 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "$Name carries no build provenance signed for $script:Repo"
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
    Test-Attestation -Path (Join-Path $Temp $asset) -Name $asset | Out-Null
    Expand-Archive -LiteralPath (Join-Path $Temp $asset) -DestinationPath (Join-Path $Temp 'app') -Force
    $bundled = Join-Path (Join-Path $Temp 'app') 'Spinoza.exe'
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

function Get-StartMenuShortcut {
    $menu = Join-Path $env:APPDATA (Join-Path 'Microsoft' (Join-Path 'Windows' (Join-Path 'Start Menu' 'Programs')))
    if (-not (Test-Path -LiteralPath $menu)) {
        return ''
    }
    return (Join-Path $menu 'Spinoza.lnk')
}

function Add-StartMenuShortcut {
    param([Parameter(Mandatory = $true)][string]$Target)
    $menu = Join-Path $env:APPDATA (Join-Path 'Microsoft' (Join-Path 'Windows' (Join-Path 'Start Menu' 'Programs')))
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
        if (Test-Attestation -Path (Join-Path $temp $asset) -Name $asset) {
            Write-Output "Verified the build provenance of $asset"
        }
        Expand-Archive -LiteralPath (Join-Path $temp $asset) -DestinationPath (Join-Path $temp 'unpacked') -Force
        $unpacked = Join-Path (Join-Path $temp 'unpacked') 'spinoza.exe'
        if (-not (Test-Path -LiteralPath $unpacked)) {
            throw 'the archive did not contain a spinoza.exe'
        }
        $previous = Get-InstalledVersion -Path $binary
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
        Install-Binary -Source $unpacked -Target $binary
        Install-Copyright -Unpacked (Join-Path $temp 'unpacked') -Directory $directory
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

function Uninstall-Spinoza {
    $directory = Get-InstallDirectory
    $removed = @()
    foreach ($name in @('spinoza.exe', 'spinoza.exe.old', 'LICENSE.txt')) {
        $binary = Join-Path $directory $name
        if (Test-Path -LiteralPath $binary) {
            Remove-Item -LiteralPath $binary -Force
            $removed += $name
        }
    }
    $app = Join-Path $directory 'app'
    if (Test-Path -LiteralPath $app) {
        Remove-Item -LiteralPath $app -Recurse -Force
        $removed += 'the desktop app'
    }
    $shortcut = Get-StartMenuShortcut
    if ($shortcut -ne '' -and (Test-Path -LiteralPath $shortcut)) {
        Remove-Item -LiteralPath $shortcut -Force
        $removed += 'the start menu entry'
    }
    if (Remove-FromPath -Directory $directory) {
        $removed += 'the PATH entry'
    }
    if ((Test-Path -LiteralPath $directory) -and -not (Get-ChildItem -LiteralPath $directory -Force)) {
        Remove-Item -LiteralPath $directory -Force
    }
    if ($removed.Count -eq 0) {
        Write-Output "Nothing to remove: spinoza is not installed in $directory"
        return
    }
    Write-Output "Removed $($removed -join ', ') from $directory"
    Write-Output 'Settings and kubeconfigs were left alone'
}

if ($MyInvocation.InvocationName -ne '.') {
    if ($env:SPINOZA_UNINSTALL) {
        Uninstall-Spinoza
    }
    else {
        Install-Spinoza
    }
}
