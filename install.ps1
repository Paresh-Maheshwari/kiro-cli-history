# Install the latest kiro-cli-history release on Windows.
#
#   irm https://raw.githubusercontent.com/Paresh-Maheshwari/kiro-cli-history/main/install.ps1 | iex
#
# Installs to %LOCALAPPDATA%\Programs\kiro-cli-history (override with
# $env:KIRO_CLI_HISTORY_INSTALL_DIR) and adds it to your user PATH.
# Works in Windows PowerShell 5.1 and PowerShell 7+.

& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # Invoke-WebRequest is much faster without the progress bar

    $Repo = 'Paresh-Maheshwari/kiro-cli-history'
    $Bin = 'kiro-cli-history'
    $InstallDir = $env:KIRO_CLI_HISTORY_INSTALL_DIR
    if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\$Bin" }

    # PROCESSOR_ARCHITEW6432 is set when a 32-bit shell runs on 64-bit Windows.
    $arch = $env:PROCESSOR_ARCHITEW6432
    if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
    switch ($arch) {
        'AMD64' { $arch = 'amd64' }
        'ARM64' { $arch = 'arm64' }
        default { throw "Unsupported architecture: $arch (need 64-bit x86 or ARM)" }
    }

    # Windows PowerShell 5.1 may default to TLS 1.0, which GitHub rejects.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $asset = "$Bin-windows-$arch.exe"
    $version = $env:KIRO_CLI_HISTORY_VERSION
    $releaseDir = $env:KIRO_CLI_HISTORY_RELEASE_DIR
    if (-not $version -and $releaseDir) { throw 'Staged installation requires KIRO_CLI_HISTORY_VERSION' }
    if (-not $version) {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
        $version = $release.tag_name
    }
    if ($version -notmatch '^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$') { throw 'Invalid release version' }
    $version = $version -replace '^v', ''
    $base = "https://github.com/$Repo/releases/download/v$version"
    Write-Host "Detected: windows/$arch"

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
    $staged = $null
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        $exeTmp = Join-Path $tmp $asset
        Write-Host "Downloading $asset..."
        $sumsTmp = Join-Path $tmp 'checksums.txt'
        if ($releaseDir) {
            Copy-Item (Join-Path $releaseDir $asset) $exeTmp
            Copy-Item (Join-Path $releaseDir 'checksums.txt') $sumsTmp
        } else {
            Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile $exeTmp
            Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile $sumsTmp
        }

        $matchesFound = @()
        foreach ($line in Get-Content $sumsTmp) {
            $f = $line -split '\s+'
            if ($f.Count -ge 2 -and $f[1].TrimStart('*') -eq $asset) { $matchesFound += $f[0].ToLower() }
        }
        $got = (Get-FileHash -Algorithm SHA256 -Path $exeTmp).Hash.ToLower()
        if ($matchesFound.Count -ne 1 -or $matchesFound[0] -ne $got) { throw "Checksum verification failed for $asset" }
        Write-Host 'Checksum verified.'

        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
        $target = Join-Path $InstallDir "$Bin.exe"
        $staged = Join-Path $InstallDir ".$Bin-$([Guid]::NewGuid()).exe"
        Copy-Item $exeTmp $staged
        $reported = @(& $staged --version)
        if ($LASTEXITCODE -ne 0 -or $reported.Count -eq 0 -or $reported[0] -ne "$Bin $version") {
            throw 'Downloaded binary reported an unexpected version'
        }
        $old = "$target.old"
        $backedUp = $false
        if (Test-Path $target) {
            # A running exe can't be overwritten but can be renamed; the app
            # removes the .old file on its next start.
            if (Test-Path $old) { Remove-Item $old -Force }
            Move-Item $target $old -Force
            $backedUp = $true
        }
        try {
            Move-Item $staged $target -Force
            $staged = $null
        } catch {
            $installError = $_
            if ($backedUp) { Move-Item $old $target -Force }
            throw $installError
        }
        Write-Host "Installed to $target"
    } finally {
        if ($staged -and (Test-Path $staged)) { Remove-Item $staged -Force -ErrorAction SilentlyContinue }
        Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }

    # Add to the user PATH (persistent) and to this session.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $parts = @()
    if ($userPath) { $parts = $userPath -split ';' | Where-Object { $_ } }
    if ($parts -notcontains $InstallDir) {
        [Environment]::SetEnvironmentVariable('Path', (($parts + $InstallDir) -join ';'), 'User')
        Write-Host "Added $InstallDir to your user PATH (open a new terminal to use it everywhere)."
    }
    if (($env:Path -split ';') -notcontains $InstallDir) { $env:Path = "$env:Path;$InstallDir" }

    Write-Host ''
    & (Join-Path $InstallDir "$Bin.exe") --version
    Write-Host ''
    Write-Host "Run: $Bin    Update later: $Bin update"
}
