# Builds both halves of the B22 probe. Run from anywhere; paths are derived.
#
# The C# half needs no SDK and installs nothing: the .NET Framework compiler and
# both accessibility client assemblies are part of Windows. Their GAC paths are
# spelled out because reconstructing them is the kind of ten-minute detour that
# makes a tool feel lost even when it is committed.

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$repo = Resolve-Path (Join-Path $here "..\..")
$out  = if ($args.Count -ge 1) { $args[0] } else { $here }

$csc = "C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe"
if (-not (Test-Path $csc)) { throw "no .NET Framework compiler at $csc" }

$gac  = "C:\WINDOWS\Microsoft.Net\assembly\GAC_MSIL"
$refs = @(
  "/r:$gac\UIAutomationClient\v4.0_4.0.0.0__31bf3856ad364e35\UIAutomationClient.dll",
  "/r:$gac\UIAutomationTypes\v4.0_4.0.0.0__31bf3856ad364e35\UIAutomationTypes.dll",
  "/r:System.dll", "/r:System.Core.dll"
)
# Accessibility.dll (IAccessible, the MSAA half) is referenced automatically by
# csc from its own directory. Passing it explicitly is an error: CS1703, the
# same assembly imported twice.
foreach ($r in $refs) {
  $p = $r.Substring(3)
  if ($p -like "*:*" -and -not (Test-Path $p)) { throw "missing assembly: $p" }
}

Write-Host "probe.exe  (the instrument: UIA, MSAA, WM_GETTEXT)"
& $csc /nologo /platform:x64 /optimize+ /out:"$out\probe.exe" $refs "$here\probe.cs"
if ($LASTEXITCODE -ne 0) { throw "csc failed" }

Write-Host "holder.exe (the subject: the shipped dialog and two controls)"
Push-Location $repo
try {
  & go build -o "$out\holder.exe" ./scripts/b22probe
  if ($LASTEXITCODE -ne 0) { throw "go build failed" }
} finally { Pop-Location }

Write-Host ""
Write-Host "Built into $out. Two terminals, holder first:"
Write-Host "    & `"$out\holder.exe`" `"$out`""
Write-Host "    & `"$out\probe.exe`"  `"$out`""
