<#
.SYNOPSIS
  The Authenticode signing step. It is wired in, it runs on every
  artefact, and today it does nothing.

.DESCRIPTION
  SPEC section 15.1: there is currently no code-signing certificate, so
  Windows SmartScreen warns on first run. That is a known, accepted
  condition, documented for users with a screenshot in the installation
  guide.

  What SPEC asks for beyond that is this file. "The build pipeline
  contains a signing step that is currently a no-op. When a certificate
  is obtained, it is filled in; nothing else changes. Do not design
  around its permanent absence." A step that is absent is not the same
  as a step that does nothing: an absent step has to be invented,
  placed, sequenced and tested at the moment somebody finally has a
  certificate and is in a hurry. This one is already in the right place
  in build.ps1, already runs against every artefact, and already fails
  the build if signing is configured and does not work.

  To fill it in, set two things in the build environment:

    LIRO_AUTHENTICODE_PFX_BASE64   the .pfx, base64-encoded
    LIRO_AUTHENTICODE_PASSWORD     its password

  and nothing else changes. In CI those are repository secrets handed
  to the build only on a tag push. That is deliberately a weaker
  protection than the release signing key's, which lives in a
  deployment environment a workflow file cannot reach around; the
  reason the two differ, and why this one has to arrive here rather
  than later, is written where it is applied
  (.github/workflows/release.yml).

  It has to arrive here because of ordering: build.ps1 signs the staged
  executable before WiX embeds it in the two MSIs, so an artefact
  signed after the build would be a package whose own copy of the
  program is unsigned.

  The timestamp URL is a public RFC 3161 service run by DigiCert. It is
  not one of SPEC section 6.8's four outbound requests, and it does not need
  to be: this runs on a build machine, never in the shipped agent.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string] $Path,

    [string] $TimestampUrl = "http://timestamp.digicert.com"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$pfxBase64 = $env:LIRO_AUTHENTICODE_PFX_BASE64
if (-not $pfxBase64) {
    Write-Host "authenticode: no certificate configured; $([IO.Path]::GetFileName($Path)) is unsigned (SPEC section 15.1)"
    exit 0
}

# From here on a failure is a failure. A build that was asked to sign
# and quietly did not would publish an unsigned artefact under a
# release that claims to be signed, which is worse than not signing at
# all.
$signtool = $null
foreach ($candidate in @(
        (Get-Command signtool.exe -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source),
        (Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin" -Recurse -Filter signtool.exe -ErrorAction SilentlyContinue |
            Where-Object { $_.FullName -match '\\x64\\' } |
            Sort-Object FullName -Descending |
            Select-Object -First 1 -ExpandProperty FullName))) {
    if ($candidate) { $signtool = $candidate; break }
}
if (-not $signtool) { throw "authenticode: a certificate is configured but signtool.exe is not on this machine" }

$pfx = Join-Path ([IO.Path]::GetTempPath()) ("liro-authenticode-" + [guid]::NewGuid().ToString("N") + ".pfx")
try {
    [IO.File]::WriteAllBytes($pfx, [Convert]::FromBase64String($pfxBase64))

    $args = @("sign", "/fd", "sha256", "/td", "sha256", "/tr", $TimestampUrl, "/f", $pfx)
    if ($env:LIRO_AUTHENTICODE_PASSWORD) { $args += @("/p", $env:LIRO_AUTHENTICODE_PASSWORD) }
    $args += $Path

    & $signtool @args
    if ($LASTEXITCODE -ne 0) { throw "authenticode: signtool failed for $Path" }
    Write-Host "authenticode: signed $([IO.Path]::GetFileName($Path))"
}
finally {
    # The certificate never outlives the step that used it.
    if (Test-Path $pfx) { Remove-Item $pfx -Force }
}
