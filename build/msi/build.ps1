<#
.SYNOPSIS
  Builds Liro Bridge's release artefacts: the two MSIs and the plain EXE.

.DESCRIPTION
  One version in, three files out:

    liro-bridge-<version>-x64.msi                per-user, no administrator rights
    liro-bridge-<version>-x64-per-machine.msi    for Group Policy deployment
    liro-bridge-<version>-x64.exe                the binary on its own

  The EXE and the two MSIs carry the same binary. There is no
  "installer edition" of this program (SPEC section 15).

  The version is stamped into the binary through -ldflags, so
  `liro-bridge --version` reports exactly what a person can match
  against a release page (F10 section 1.1). A build with no -Version is a
  development build and says "dev", which is what it is.

  WiX is fetched once into a cache directory and pinned by SHA-256 --
  the same discipline D-110 applied to golangci-lint, and for the same
  reason: a toolchain that can change overnight with no commit turns a
  release into a coin toss.

.EXAMPLE
  pwsh build/msi/build.ps1 -Version 1.0.0 -Out dist/release
#>
[CmdletBinding()]
param(
    # The released version, without a leading v. Omit for a development
    # build: the binary then reports "dev" and the MSIs are stamped
    # 0.0.0, which Windows Installer accepts and no release ever uses.
    [string] $Version = "",

    # Where the artefacts go.
    [string] $Out = "dist/release",

    # Where candle.exe and light.exe are. Found automatically if the
    # WiX toolset is installed; fetched into -WixCache otherwise.
    [string] $WixBin = "",

    # Where a fetched WiX is kept between builds.
    [string] $WixCache = "$env:LOCALAPPDATA\Liro\build\wix314"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

# WiX 3.14.1 RTM, the last v3 release. Pinned by digest: this is a
# build tool that produces the file a stranger double-clicks.
$WixZipUrl = "https://github.com/wixtoolset/wix3/releases/download/wix3141rtm/wix314-binaries.zip"
$WixZipSha256 = "6ac824e1642d6f7277d0ed7ea09411a508f6116ba6fae0aa5f2c7daa2ff43d31"

$repo = Resolve-Path (Join-Path $PSScriptRoot "../..")
Push-Location $repo
try {
    # ---- the toolchain ------------------------------------------------
    function Find-WixBin {
        if ($WixBin) { return $WixBin }
        if ($env:WIX -and (Test-Path (Join-Path $env:WIX "bin/candle.exe"))) {
            return (Join-Path $env:WIX "bin")
        }
        foreach ($v in @("v3.14", "v3.11")) {
            $p = "${env:ProgramFiles(x86)}\WiX Toolset $v\bin"
            if (Test-Path (Join-Path $p "candle.exe")) { return $p }
        }
        if (Test-Path (Join-Path $WixCache "candle.exe")) { return $WixCache }

        Write-Host "WiX not found; fetching $WixZipUrl"
        New-Item -ItemType Directory -Force -Path $WixCache | Out-Null
        $zip = Join-Path $WixCache "wix314-binaries.zip"
        Invoke-WebRequest -Uri $WixZipUrl -OutFile $zip -UseBasicParsing
        $got = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
        if ($got -ne $WixZipSha256) {
            Remove-Item $zip -Force
            throw "the WiX archive hashes to $got, expected $WixZipSha256"
        }
        Expand-Archive -Path $zip -DestinationPath $WixCache -Force
        return $WixCache
    }

    $wix = Find-WixBin
    $candle = Join-Path $wix "candle.exe"
    $light = Join-Path $wix "light.exe"
    foreach ($t in @($candle, $light)) {
        if (-not (Test-Path $t)) { throw "$t is not there" }
    }

    # ---- the binary ---------------------------------------------------
    $goVersion = if ($Version) { $Version } else { "dev" }
    # Windows Installer's ProductVersion is at most three numeric parts
    # and ignores anything after them, so a pre-release tag (1.0.0-rc.1)
    # installs as 1.0.0. That is a real limitation of the format rather
    # than a choice: the binary still reports the full version, which is
    # what --version prints and what the update check compares.
    $msiVersion = if ($Version) { ($Version -split '-')[0] } else { "0.0.0" }

    $commit = "none"
    try { $commit = (git rev-parse --short HEAD).Trim() } catch { }
    $buildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

    $outDir = if ([IO.Path]::IsPathRooted($Out)) { $Out } else { Join-Path $repo $Out }
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null
    $stage = Join-Path $outDir ".stage"
    if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
    New-Item -ItemType Directory -Force -Path $stage | Out-Null

    $exe = Join-Path $stage "liro-bridge.exe"
    $ldflags = "-s -w " +
        "-X main.version=$goVersion " +
        "-X main.commit=$commit " +
        "-X main.buildDate=$buildDate"

    Write-Host "building liro-bridge.exe ($goVersion, $commit)"
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    & go build -trimpath -ldflags $ldflags -o $exe ./cmd/liro-bridge
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }

    # The guides travel with the program, so a person who has lost the
    # download page still has them (F10 section 6). Both languages and
    # the one shared set of screenshots: the Serbian text is the
    # authoritative one and the English is its translation, and each
    # links to the other (D-242).
    $guide = Join-Path $repo "docs/guide/Uputstvo.html"
    $guideEn = Join-Path $repo "docs/guide/Guide.html"
    $guideImages = Join-Path $repo "docs/guide/slike"
    foreach ($g in @($guide, $guideEn)) {
        if (-not (Test-Path $g)) { throw "$g is not there" }
    }
    # Every picture the two documents reference, checked here rather than
    # discovered as a broken image by the first person to read the guide.
    # The WiX source names these eight explicitly, so a ninth added to
    # the folder without being added there would ship in neither.
    $expectedImages = @(
        "01-dokumenti-prazno.png", "02-dokumenti.png", "03-sertifikat.png", "04-metod.png",
        "05-napredak.png", "06-izvestaj.png", "07-dnevnik.png", "08-podesavanja.png")
    foreach ($img in $expectedImages) {
        $p = Join-Path $guideImages $img
        if (-not (Test-Path $p)) {
            throw "$p is not there. Regenerate with: LIRO_GUIDE_SHOTS=1 go test ./cmd/liro-bridge/ -run TestCaptureGuideScreens"
        }
    }
    $onDisk = @(Get-ChildItem $guideImages -Filter *.png | Select-Object -ExpandProperty Name)
    $extra = $onDisk | Where-Object { $expectedImages -notcontains $_ }
    if ($extra) {
        throw ("docs/guide/slike holds $($extra -join ', '), which liro-bridge.wxs does not install. " +
               "Add them there or remove them.")
    }
    $icon = Join-Path $repo "internal/ui/assets/icon.ico"

    # ---- Authenticode: present, and a no-op until there is a
    #      certificate (SPEC section 15.1) --------------------------------------
    & (Join-Path $PSScriptRoot "sign-authenticode.ps1") -Path $exe

    # ---- the two packages ---------------------------------------------
    foreach ($scope in @("perUser", "perMachine")) {
        $suffix = if ($scope -eq "perUser") { "" } else { "-per-machine" }
        $name = if ($Version) { "liro-bridge-$Version-x64$suffix.msi" } else { "liro-bridge-dev-x64$suffix.msi" }
        $msi = Join-Path $outDir $name
        $wixobj = Join-Path $stage "liro-bridge-$scope.wixobj"

        Write-Host "building $name"
        & $candle -nologo -arch x64 `
            -dVersion="$msiVersion" `
            -dScope="$scope" `
            -dExeFile="$exe" `
            -dGuideFile="$guide" `
            -dGuideEnFile="$guideEn" `
            -dGuideImagesDir="$guideImages" `
            -dIconFile="$icon" `
            -out $wixobj (Join-Path $PSScriptRoot "liro-bridge.wxs")
        if ($LASTEXITCODE -ne 0) { throw "candle failed for $scope" }

        # Three ICE checks are suppressed, and each is suppressed for a
        # reason rather than to make a message go away:
        #
        #   ICE38, ICE64  Both are written for the pre-Windows-Installer-5
        #                 way of installing into a user profile
        #                 (ALLUSERS="" plus a registry keypath per
        #                 component). This is a modern per-user package
        #                 (MSIINSTALLPERUSER, InstallScope perUser), where
        #                 a file keypath is correct and is what every
        #                 per-user installer ships. ICE64's real point --
        #                 that a directory in a user profile has to be
        #                 removed explicitly -- is answered by the
        #                 InstallFolderCleanup component, not by the
        #                 suppression.
        #   ICE91         Warns that the install directory is under the
        #                 user profile. That is the whole design.
        #
        # -sw1076 is the advertised-shortcut keypath warning: this
        # program has no advertised features, and an advertised shortcut
        # would make a Start menu click trigger a repair.
        $iceSuppress = @("-sice:ICE38", "-sice:ICE64", "-sice:ICE91")
        if ($scope -eq "perMachine") {
            # ICE43 wants a non-advertised shortcut's component keypath
            # under HKCU. In a per-machine package the shortcut is in
            # the All Users Start menu -- per-machine data -- so an HKCU
            # keypath is precisely what ICE57 forbids here. The two
            # checks contradict each other for this shape, which is a
            # known limitation of ICE43 rather than a defect in the
            # package; the alternative, an advertised shortcut, would
            # make every Start menu click trigger a repair.
            $iceSuppress += "-sice:ICE43"
        }
        # -pdbout into the staging directory: the .wixpdb is build
        # output, not something a release page hands anybody.
        & $light -nologo -sw1076 @iceSuppress -pdbout (Join-Path $stage "liro-bridge-$scope.wixpdb") -out $msi $wixobj
        if ($LASTEXITCODE -ne 0) { throw "light failed for $scope" }

        & (Join-Path $PSScriptRoot "sign-authenticode.ps1") -Path $msi
    }

    # ---- the plain EXE ------------------------------------------------
    $exeName = if ($Version) { "liro-bridge-$Version-x64.exe" } else { "liro-bridge-dev-x64.exe" }
    Copy-Item $exe (Join-Path $outDir $exeName) -Force

    Remove-Item -Recurse -Force $stage

    Write-Host ""
    Write-Host "artefacts in $outDir"
    Get-ChildItem $outDir -File | ForEach-Object {
        "{0,-46} {1,10:N0} bytes  {2}" -f $_.Name, $_.Length, (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower()
    }
}
finally {
    Pop-Location
}
