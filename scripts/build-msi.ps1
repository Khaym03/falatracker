<#
.SYNOPSIS
    Builds and packages falatracker as a production Windows MSI installer using WiX v5 (WixToolset.Sdk) and go-winres.
#>
[CmdletBinding()]
param(
    [Parameter()]
    [string]$Version,

    [Parameter()]
    [ValidateSet("x64", "arm64", "x86")]
    [string]$Arch = "x64",

    [Parameter()]
    [string]$SignCertThumbprint,

    [Parameter()]
    [string]$TimestampUrl = "http://timestamp.digicert.com"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-VersionFromGit {
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $tag = (& git describe --tags --abbrev=0 --match "v[0-9]*.[0-9]*.[0-9]*" 2>$null)
        if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($tag)) {
            return ($tag.Trim() -replace '^v', '')
        }
    }
    return "0.1.0"
}

function Resolve-GitValue {
    param(
        [Parameter(Mandatory)][string[]]$GitArgs,
        [Parameter(Mandatory)][string]$Fallback
    )
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $out = (& git @GitArgs 2>$null)
        if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($out)) {
            return $out.Trim()
        }
    }
    return $Fallback
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

Push-Location $RepoRoot
try {
    if ([string]::IsNullOrWhiteSpace($Version)) {
        $Version = Resolve-VersionFromGit
        Write-Host "==> Resolved version from Git tags (SSoT): $Version" -ForegroundColor Cyan
    }
    else {
        $Version = $Version.Trim() -replace '^v', ''
    }

    if ($Version -notmatch '^\d+\.\d+\.\d+$') {
        throw "Invalid version '$Version'. Expected Major.Minor.Patch (e.g. 1.2.3 or v1.2.3)."
    }

    $Commit = Resolve-GitValue -GitArgs @("rev-parse", "--short", "HEAD") -Fallback "unknown"
    $Date   = Resolve-GitValue -GitArgs @("show", "-s", "--format=%cI", "HEAD") -Fallback "unknown"

    $ArchMap = @{
        "x64"   = @{ GoArch = "amd64"; WinResArch = "amd64" }
        "arm64" = @{ GoArch = "arm64"; WinResArch = "arm64" }
        "x86"   = @{ GoArch = "386";   WinResArch = "386" }
    }

    $GoArch       = $ArchMap[$Arch].GoArch
    $WinResArch   = $ArchMap[$Arch].WinResArch
    $BuildDir     = Join-Path $RepoRoot "build"
    $WinResJson   = Join-Path $RepoRoot "winres\winres.json"
    $WixProjPath  = Join-Path $RepoRoot "packaging\windows\falatracker.wixproj"
    $TemplatePath = Join-Path $RepoRoot "templates\standard.xlsx"
    $ExePath        = Join-Path $BuildDir "falatracker.exe"
    $ReleaseExePath = Join-Path $BuildDir "falatracker-$Version-windows-$GoArch.exe"
    $MsiPath        = Join-Path $BuildDir "falatracker-$Version-$Arch.msi"
    $WinVersion   = "$Version.0"

    function Assert-CommandExists {
        param([Parameter(Mandatory)][string]$Name)
        if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
            throw "Required tool '$Name' was not found in PATH."
        }
    }

    function Invoke-CodeSign {
        param(
            [Parameter(Mandatory)][string]$FilePath,
            [Parameter(Mandatory)][string]$Thumbprint,
            [Parameter(Mandatory)][string]$TimestampServer
        )
        Assert-CommandExists -Name "signtool.exe"
        Write-Host "==> Signing $FilePath ..." -ForegroundColor Cyan
        & signtool.exe sign `
            /sha1 $Thumbprint `
            /fd SHA256 `
            /tr $TimestampServer `
            /td SHA256 `
            /d "FalaTracker" `
            $FilePath
        if ($LASTEXITCODE -ne 0) {
            throw "signtool.exe failed for '$FilePath' with exit code $LASTEXITCODE."
        }
    }

    Assert-CommandExists -Name "go"
    Assert-CommandExists -Name "go-winres"
    Assert-CommandExists -Name "dotnet"

    if (-not (Test-Path $TemplatePath)) {
        throw "Required Excel template not found at '$TemplatePath'."
    }

    Write-Host "==> [1/5] Cleaning build directory: $BuildDir" -ForegroundColor Cyan
    if (Test-Path $BuildDir) { Remove-Item -Path $BuildDir -Recurse -Force }
    New-Item -Path $BuildDir -ItemType Directory | Out-Null

    Write-Host "==> [2/5] Generating Windows metadata (.syso) with go-winres ($WinResArch)..." -ForegroundColor Cyan
    & go-winres make --in $WinResJson --arch $WinResArch --file-version $WinVersion --product-version $WinVersion
    if ($LASTEXITCODE -ne 0) { throw "go-winres make failed with exit code $LASTEXITCODE." }

    Write-Host "==> [3/5] Compiling falatracker.exe (windows/$GoArch, v$Version)..." -ForegroundColor Cyan
    $env:GOOS = "windows"
    $env:GOARCH = $GoArch
    $env:CGO_ENABLED = "0"
    $versionPkg = "github.com/Khaym03/falatracker/internal/version"
    $ldflags = "-s -w -X $versionPkg.Version=v$Version -X $versionPkg.Commit=$Commit -X $versionPkg.Date=$Date"
    & go build -trimpath -ldflags $ldflags -o $ExePath .
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE." }

    if (-not [string]::IsNullOrWhiteSpace($SignCertThumbprint)) {
        Invoke-CodeSign -FilePath $ExePath -Thumbprint $SignCertThumbprint -TimestampServer $TimestampUrl
    }

    Copy-Item -Path $ExePath -Destination $ReleaseExePath -Force

    Write-Host "==> [4/5] Building MSI installer via WiX v5 SDK: $MsiPath" -ForegroundColor Cyan
    & dotnet build $WixProjPath `
        -c Release `
        "-p:InstallerPlatform=$Arch" `
        "-p:ProductVersion=$Version" `
        "-p:BinPath=$ExePath" `
        "-p:TemplatePath=$TemplatePath" `
        "-p:OutputPath=$BuildDir\"
    if ($LASTEXITCODE -ne 0) { throw "dotnet build failed for '$WixProjPath' with exit code $LASTEXITCODE." }

    if (-not [string]::IsNullOrWhiteSpace($SignCertThumbprint)) {
        Write-Host "==> [5/5] Signing MSI package..." -ForegroundColor Cyan
        Invoke-CodeSign -FilePath $MsiPath -Thumbprint $SignCertThumbprint -TimestampServer $TimestampUrl
    }
    else {
        Write-Host "==> [5/5] Skipping code signing (-SignCertThumbprint not specified)." -ForegroundColor Yellow
    }

    Write-Host ""
    Write-Host "Build completed successfully:" -ForegroundColor Green
    Write-Host "  Version    : v$Version ($Commit)"
    Write-Host "  Binary     : $ExePath"
    Write-Host "  Release Exe: $ReleaseExePath"
    Write-Host "  MSI        : $MsiPath"
}
finally {
    Get-ChildItem -Path $RepoRoot -Filter "*.syso" -File -ErrorAction SilentlyContinue |
        Remove-Item -Force -ErrorAction SilentlyContinue
    Pop-Location
}
