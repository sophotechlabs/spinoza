#Requires -Version 5.1

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Binary,
    [int]$Port = 34988
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Wait-ForFile {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [int]$Attempts = 100
    )
    for ($try = 0; $try -lt $Attempts; $try++) {
        if (Test-Path -LiteralPath $Path) {
            $content = (Get-Content -LiteralPath $Path -Raw -ErrorAction SilentlyContinue)
            if ($content) {
                return $content.Trim()
            }
        }
        Start-Sleep -Milliseconds 200
    }
    return ''
}

function Wait-ForHealth {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][hashtable]$Headers,
        [int]$Attempts = 100
    )
    for ($try = 0; $try -lt $Attempts; $try++) {
        try {
            $answer = Invoke-WebRequest -Uri $Uri -Headers $Headers -UseBasicParsing -TimeoutSec 5
            if ($answer.StatusCode -eq 200) {
                return $true
            }
        }
        catch {
            $null = $_
        }
        Start-Sleep -Milliseconds 200
    }
    return $false
}

if (-not (Test-Path -LiteralPath $Binary)) {
    throw "smoke: $Binary is missing, run just release-dist first"
}
$Binary = (Resolve-Path -LiteralPath $Binary).Path

$reported = (& $Binary --version | Select-Object -First 1)
Write-Output "smoke: $Binary reports $reported"

$work = Join-Path ([System.IO.Path]::GetTempPath()) ('spinoza-smoke-' + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work -Force | Out-Null
$kubeconfig = Join-Path $work 'kubeconfig'
$tokenFile = Join-Path $work 'token'
Set-Content -LiteralPath $kubeconfig -Value "apiVersion: v1`nkind: Config`n" -NoNewline

$previousKubeconfig = $env:KUBECONFIG
$env:KUBECONFIG = $kubeconfig
$server = $null
try {
    $server = Start-Process -FilePath $Binary -ArgumentList @(
        '--addr', "127.0.0.1:$Port",
        '--token-file', $tokenFile
    ) -PassThru -NoNewWindow

    $token = Wait-ForFile -Path $tokenFile
    if ($token -eq '') {
        throw 'smoke: the token file was never written'
    }

    $headers = @{ 'X-Spinoza-Token' = $token }
    $healthy = Wait-ForHealth -Uri "http://127.0.0.1:$Port/healthz" -Headers $headers
    if (-not $healthy) {
        throw 'smoke: healthz never answered 200'
    }

    $page = Invoke-WebRequest -Uri "http://127.0.0.1:$Port/" -Headers $headers -UseBasicParsing -TimeoutSec 10
    if ($page.Content -match 'built without its frontend') {
        throw 'smoke: the binary embeds the placeholder index.html'
    }
    Write-Output "smoke: $reported answered healthz and served the frontend"
}
finally {
    if ($server -and -not $server.HasExited) {
        Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue
    }
    $env:KUBECONFIG = $previousKubeconfig
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}
