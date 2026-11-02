param(
    [Parameter(Mandatory = $true)][string]$ReleaseDir,
    [Parameter(Mandatory = $true)][string]$Version
)

$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
$oldDir = $env:KIRO_CLI_HISTORY_RELEASE_DIR
$oldVersion = $env:KIRO_CLI_HISTORY_VERSION
$oldInstall = $env:KIRO_CLI_HISTORY_INSTALL_DIR
$oldPath = $env:Path
$oldUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$installer = Join-Path $PSScriptRoot '..\install.ps1'
$wrapper = Join-Path $PSScriptRoot '..\install.cmd'
$install = Join-Path $root 'installed'
$target = Join-Path $install 'kiro-cli-history.exe'

function Assert-InstallFails([string]$Label) {
    $failed = $false
    try { & $installer } catch { $failed = $true }
    if (-not $failed) { throw "$Label unexpectedly succeeded" }
}

try {
    New-Item -ItemType Directory -Path $root | Out-Null
    $env:KIRO_CLI_HISTORY_RELEASE_DIR = $ReleaseDir
    $env:KIRO_CLI_HISTORY_VERSION = $Version
    $env:KIRO_CLI_HISTORY_INSTALL_DIR = $install
    & $installer
    $expected = (Get-FileHash -Algorithm SHA256 $target).Hash

    # Keep the cmd.exe intermediary: it inherits PowerShell 7's module path,
    # which must not leak into the Windows PowerShell started by install.cmd.
    & $env:ComSpec /d /c "call `"$wrapper`" /nopause"
    if ($LASTEXITCODE -ne 0) { throw 'Command installer failed' }
    if ((Get-FileHash -Algorithm SHA256 $target).Hash -ne $expected) {
        throw 'Command installer installed a different binary'
    }

    # A failed checksum must leave the already installed binary unchanged.
    $bad = Join-Path $root 'bad-release'
    New-Item -ItemType Directory -Path $bad | Out-Null
    Get-ChildItem $ReleaseDir -Filter 'kiro-cli-history-windows-*.exe' |
        Copy-Item -Destination $bad
    Set-Content -Path (Join-Path $bad 'checksums.txt') -Value 'invalid checksum'
    $env:KIRO_CLI_HISTORY_RELEASE_DIR = $bad
    Assert-InstallFails 'Checksum failure'
    if ((Get-FileHash -Algorithm SHA256 $target).Hash -ne $expected) {
        throw 'Checksum failure changed the existing binary'
    }

    # Simulate failure only after the original target has been backed up.
    $env:KIRO_CLI_HISTORY_RELEASE_DIR = $ReleaseDir
    function Move-Item {
        [CmdletBinding()]
        param([string]$Path, [string]$Destination, [switch]$Force)
        if ($Destination -eq $target -and [IO.Path]::GetFileName($Path) -like '.kiro-cli-history-*.exe') {
            throw 'Simulated final move failure'
        }
        Microsoft.PowerShell.Management\Move-Item @PSBoundParameters
    }
    Assert-InstallFails 'Final move failure'
    if (-not (Test-Path $target) -or (Get-FileHash -Algorithm SHA256 $target).Hash -ne $expected) {
        throw 'Installer did not restore the original binary'
    }
    if (Get-ChildItem $install -Filter '.kiro-cli-history-*.exe') {
        throw 'Installer left a staged binary'
    }
    Write-Host 'PowerShell and command installer success, checksum rejection, and rollback checks passed.'
} finally {
    Remove-Item Function:\Move-Item -ErrorAction SilentlyContinue
    $env:KIRO_CLI_HISTORY_RELEASE_DIR = $oldDir
    $env:KIRO_CLI_HISTORY_VERSION = $oldVersion
    $env:KIRO_CLI_HISTORY_INSTALL_DIR = $oldInstall
    $env:Path = $oldPath
    [Environment]::SetEnvironmentVariable('Path', $oldUserPath, 'User')
    Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue
}
