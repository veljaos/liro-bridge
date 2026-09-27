# DRY RUN ONLY - proves the instrument works before the owner spends a session.
#
# This drives the holder with WM_SETTEXT and WM_COMMAND, the way
# cmd/liro-bridge/pindialog_windows_test.go already does. That is NOT the
# measurement: B22 asks what is exposed when a person types, and typing raises
# per-keystroke events that WM_SETTEXT does not. Nothing this script produces
# answers B22. It answers "does probe.exe read anything at all".

$ErrorActionPreference = "Stop"
$SP = "C:\Users\Veljko\AppData\Local\Temp\claude\C--Users-Veljko-Desktop-liro-bridge\ae29397d-ed9b-4cc4-bd7e-1f2192c4e05a\scratchpad"

Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class W {
  public delegate bool EnumProc(IntPtr h, IntPtr p);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr p);
  [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr par, EnumProc cb, IntPtr p);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr h, StringBuilder sb, int m);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr h, int m, IntPtr w, string l);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, int m, IntPtr w, IntPtr l);
  public static IntPtr FindTop(uint pid, string cls) {
    IntPtr found = IntPtr.Zero;
    EnumWindows((h,p) => {
      uint q; GetWindowThreadProcessId(h, out q);
      if (q != pid || !IsWindowVisible(h)) return true;
      var sb = new StringBuilder(256); GetClassName(h, sb, sb.Capacity);
      if (sb.ToString() != cls) return true;
      found = h; return false; }, IntPtr.Zero);
    return found;
  }
  public static IntPtr FindEdit(IntPtr par) {
    IntPtr found = IntPtr.Zero;
    EnumChildWindows(par, (h,p) => {
      var sb = new StringBuilder(256); GetClassName(h, sb, sb.Capacity);
      if (!sb.ToString().Equals("Edit", StringComparison.OrdinalIgnoreCase)) return true;
      found = h; return false; }, IntPtr.Zero);
    return found;
  }
}
'@

Remove-Item "$SP\handover.json","$SP\phases-done","$SP\dry-holder.txt","$SP\dry-probe.txt" -ErrorAction SilentlyContinue

$hi = New-Object System.Diagnostics.ProcessStartInfo
$hi.FileName = "$SP\holder.exe"; $hi.Arguments = "`"$SP`""
$hi.UseShellExecute = $false; $hi.RedirectStandardInput = $true
$hi.RedirectStandardOutput = $true
$holder = [System.Diagnostics.Process]::Start($hi)
# Drain from the first moment. Reading a redirected stdout only at the end fills
# the pipe, blocks the child inside its own WriteLine, and truncates its output -
# which is how four of phase C's rows went missing on the previous dry run.
$hTask = $holder.StandardOutput.ReadToEndAsync()

Start-Sleep -Milliseconds 1200
if (-not (Test-Path "$SP\handover.json")) { throw "holder wrote no handover.json" }
$H = Get-Content "$SP\handover.json" -Raw | ConvertFrom-Json
"holder pid $($H.pid), run $($H.run_id)"

$pi = New-Object System.Diagnostics.ProcessStartInfo
$pi.FileName = "$SP\probe.exe"; $pi.Arguments = "`"$SP`""
$pi.UseShellExecute = $false; $pi.RedirectStandardInput = $true
$pi.RedirectStandardOutput = $true
$probe = [System.Diagnostics.Process]::Start($pi)
$pTask = $probe.StandardOutput.ReadToEndAsync()

Start-Sleep -Milliseconds 1500
$holder.StandardInput.WriteLine("")   # release phase A

$phases = @(
  @{ n="A"; cls=$H.class_a; needle=$H.needle_a },
  @{ n="B"; cls=$H.class_b; needle=$H.needle_b },
  @{ n="C"; cls=$H.class_c; needle=$H.needle_c }
)
foreach ($ph in $phases) {
  $top = [IntPtr]::Zero; $t = 0
  while ($top -eq [IntPtr]::Zero -and $t -lt 150) {
    $top = [W]::FindTop([uint32]$H.pid, $ph.cls); Start-Sleep -Milliseconds 100; $t++
  }
  if ($top -eq [IntPtr]::Zero) { throw "phase $($ph.n): window class $($ph.cls) never appeared" }
  $edit = [W]::FindEdit($top)
  if ($edit -eq [IntPtr]::Zero) { throw "phase $($ph.n): no Edit child" }
  "phase $($ph.n): hwnd=$top edit=$edit - filling one character at a time"
  # One character at a time, so the provider raises the change events a real
  # typist would raise. Still WM_SETTEXT, still not typing.
  for ($i = 1; $i -le $ph.needle.Length; $i++) {
    [void][W]::SendMessage($edit, 0x000C, [IntPtr]::Zero, $ph.needle.Substring(0,$i))
    Start-Sleep -Milliseconds 120
  }
  Start-Sleep -Milliseconds 3200       # the two-second steady state, and then some
  [void][W]::PostMessage($top, 0x0111, [IntPtr]1, [IntPtr]::Zero)  # WM_COMMAND, IDOK
  Start-Sleep -Milliseconds 900
}

$holder.StandardInput.WriteLine("")   # close the holder
Start-Sleep -Milliseconds 2500
$probe.StandardInput.WriteLine("")
if (-not $probe.WaitForExit(20000)) { "probe did not exit; killing"; $probe.Kill() }
if (-not $holder.HasExited) { $holder.Kill() }
$pTask.Result | Set-Content "$SP\dry-probe.txt" -Encoding utf8
$hTask.Result | Set-Content "$SP\dry-holder.txt" -Encoding utf8
"--- done; see dry-probe.txt and dry-holder.txt ---"
