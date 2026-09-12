# Collects everything that bears on "I dragged a document onto the
# window and nothing happened" into one file, in one reading.
#
# Run it WHILE the window is still open, before closing anything. It
# takes no arguments, changes nothing, and writes a text file to the
# Desktop whose path it prints at the end.
#
# Why it exists. A drop that goes nowhere leaves no trace in the agent
# at all: if the window under the cursor is not ours, no IDropTarget of
# ours is consulted and the log shows a perfect registration and
# nothing else. The state that explains it is on the screen at that
# moment and gone a second later. This is the thing to run instead of
# remembering what the screen looked like.

$ErrorActionPreference = 'Continue'

Add-Type @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class DR {
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr l);
  [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr p, EnumProc cb, IntPtr l);
  [DllImport("user32.dll")] public static extern int GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassNameW(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr GetPropW(IntPtr h, string s);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern IntPtr GetWindowLongPtrW(IntPtr h, int i);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern IntPtr WindowFromPoint(POINT p);
  [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
  [StructLayout(LayoutKind.Sequential)] public struct RECT { public int L,T,R,B; }
  [StructLayout(LayoutKind.Sequential)] public struct POINT { public int X, Y; }

  [DllImport("kernel32.dll", SetLastError=true)] public static extern IntPtr OpenProcess(int a, bool inh, int pid);
  [DllImport("kernel32.dll", SetLastError=true)] public static extern bool CloseHandle(IntPtr h);
  [DllImport("advapi32.dll", SetLastError=true)] public static extern bool OpenProcessToken(IntPtr p, int a, out IntPtr t);
  [DllImport("advapi32.dll", SetLastError=true)] public static extern bool GetTokenInformation(IntPtr t, int cls, IntPtr buf, int len, out int ret);
  [DllImport("advapi32.dll", SetLastError=true, CharSet=CharSet.Unicode)] public static extern bool ConvertSidToStringSidW(IntPtr sid, out IntPtr str);
  [DllImport("kernel32.dll")] public static extern IntPtr LocalFree(IntPtr h);
}
"@ -ErrorAction SilentlyContinue

$out = New-Object System.Collections.ArrayList
function Say([string]$s) { [void]$out.Add($s); Write-Host $s }

function ClassOf([IntPtr]$h) { $sb = New-Object System.Text.StringBuilder 256; [void][DR]::GetClassNameW($h,$sb,256); $sb.ToString() }

function IntegrityOf([int]$procId) {
  $h = [DR]::OpenProcess(0x1000, $false, $procId)
  if ($h -eq [IntPtr]::Zero) { return 'denied' }
  try {
    $tok = [IntPtr]::Zero
    if (-not [DR]::OpenProcessToken($h, 0x0008, [ref]$tok)) { return 'denied' }
    try {
      $len = 0
      [void][DR]::GetTokenInformation($tok, 25, [IntPtr]::Zero, 0, [ref]$len)
      $buf = [Runtime.InteropServices.Marshal]::AllocHGlobal($len)
      try {
        if (-not [DR]::GetTokenInformation($tok, 25, $buf, $len, [ref]$len)) { return 'unknown' }
        $sidPtr = [Runtime.InteropServices.Marshal]::ReadIntPtr($buf)
        $strPtr = [IntPtr]::Zero
        if (-not [DR]::ConvertSidToStringSidW($sidPtr, [ref]$strPtr)) { return 'unknown' }
        $sid = [Runtime.InteropServices.Marshal]::PtrToStringUni($strPtr)
        [void][DR]::LocalFree($strPtr)
        switch -Wildcard ($sid) {
          '*-4096'  { return 'LOW' }
          '*-8192'  { return 'MEDIUM' }
          '*-8448'  { return 'MEDIUM+' }
          '*-12288' { return 'HIGH  <-- above Explorer: UIPI discards drags from Explorer' }
          '*-16384' { return 'SYSTEM <-- above Explorer: UIPI discards drags from Explorer' }
          default   { return $sid }
        }
      } finally { [Runtime.InteropServices.Marshal]::FreeHGlobal($buf) }
    } finally { [void][DR]::CloseHandle($tok) }
  } finally { [void][DR]::CloseHandle($h) }
}

Say ""
Say "================================================================"
Say "  Liro Bridge drag report"
Say ("  taken {0}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))
Say "================================================================"

# ---- the machine ------------------------------------------------------
Say ""
Say "---- machine ----"
try {
  $os = Get-CimInstance Win32_OperatingSystem
  Say ("  Windows            {0} build {1}" -f $os.Caption, $os.BuildNumber)
} catch { Say "  Windows            (could not read)" }
$wv = $null
foreach ($k in @(
  'HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
  'HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
  'HKCU:\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}')) {
  try { $v = (Get-ItemProperty -Path $k -ErrorAction Stop).pv; if ($v) { $wv = $v; break } } catch { }
}
Say ("  WebView2 runtime   {0}" -f $(if ($wv) { $wv } else { 'NOT FOUND' }))

# ---- processes --------------------------------------------------------
Say ""
Say "---- liro-bridge processes ----"
$procs = @(Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -like 'liro-bridge*' })
if ($procs.Count -eq 0) { Say "  none running - was the window still open when this ran?" }
foreach ($p in $procs) {
  $path = try { $p.Path } catch { '(denied)' }
  Say ("  pid {0,-7} integrity {1,-12} started {2:HH:mm:ss}  {3}" -f $p.Id, (IntegrityOf $p.Id), $p.StartTime, $path)
}
if ($procs.Count -gt 1) {
  Say "  NOTE: more than one agent is running. Two agent windows are centred on the"
  Say "        same point, so one can sit exactly on top of the other."
}
foreach ($p in (Get-Process -Name explorer -ErrorAction SilentlyContinue)) {
  Say ("  explorer pid {0,-7} integrity {1}" -f $p.Id, (IntegrityOf $p.Id))
}

# ---- the windows ------------------------------------------------------
Say ""
Say "---- every Liro Bridge window, and what is really in front of it ----"
$tops = New-Object System.Collections.ArrayList
$cb = [DR+EnumProc]{ param($h, $l) if ((ClassOf $h) -eq 'LiroBridgeWindow') { [void]$tops.Add($h) }; return $true }
[void][DR]::EnumWindows($cb, [IntPtr]::Zero)
if ($tops.Count -eq 0) { Say "  none on the desktop" }

foreach ($top in $tops) {
  $owner = 0
  [void][DR]::GetWindowThreadProcessId($top, [ref]$owner)
  Say ""
  Say ("  window owned by pid {0}" -f $owner)

  $tree = New-Object System.Collections.ArrayList
  [void]$tree.Add($top)
  $cb2 = [DR+EnumProc]{ param($h, $l) [void]$tree.Add($h); return $true }
  [void][DR]::EnumChildWindows($top, $cb2, [IntPtr]::Zero)

  foreach ($h in $tree) {
    $r = New-Object DR+RECT
    [void][DR]::GetWindowRect($h, [ref]$r)
    $tgt = if ([DR]::GetPropW($h,'OleDropTargetInterface') -ne [IntPtr]::Zero) { 'YES' } else { ' -- ' }
    Say ("    {0,-30} {1,-10} droptarget {2}  visible {3,-3} ex 0x{4:x8}  {5}x{6}" -f `
      (ClassOf $h), ('0x{0:x}' -f $h.ToInt64()), $tgt, `
      $(if ([DR]::IsWindowVisible($h)) {'yes'} else {'no'}), `
      [DR]::GetWindowLongPtrW($h,-20).ToInt64(), ($r.R-$r.L), ($r.B-$r.T))
  }

  # The question the log cannot answer. Membership of THIS window's tree
  # is the test, not the owning process: Chrome_RenderWidgetHostHWND is
  # owned by the msedgewebview2 process and is still ours to drop on.
  $r = New-Object DR+RECT
  [void][DR]::GetWindowRect($top, [ref]$r)
  $pt = New-Object DR+POINT
  $pt.X = [int](($r.L + $r.R) / 2); $pt.Y = [int](($r.T + $r.B) / 2)
  $under = [DR]::WindowFromPoint($pt)
  $inTree = $false
  foreach ($h in $tree) { if ($h -eq $under) { $inTree = $true; break } }
  $uPid = 0
  [void][DR]::GetWindowThreadProcessId($under, [ref]$uPid)
  $uName = try { (Get-Process -Id $uPid -ErrorAction Stop).ProcessName } catch { '?' }
  Say ""
  if ($inTree) {
    Say ("    at the centre ({0},{1}) sits {2} - part of this window. A drop here reaches the agent." -f $pt.X, $pt.Y, (ClassOf $under))
  } else {
    Say ("    at the centre ({0},{1}) sits '{2}' pid {3} ({4})" -f $pt.X, $pt.Y, (ClassOf $under), $uPid, $uName)
    Say  "    *** THIS IS NOT PART OF THE AGENT'S WINDOW ***"
    Say  "    A drop at that point goes to that window. The agent is never told, and"
    Say  "    its log will show a perfect registration and no drag at all."
  }
}

$fg = [DR]::GetForegroundWindow()
$fgPid = 0
[void][DR]::GetWindowThreadProcessId($fg, [ref]$fgPid)
$fgName = try { (Get-Process -Id $fgPid -ErrorAction Stop).ProcessName } catch { '?' }
Say ""
Say ("  foreground window: class '{0}' pid {1} ({2})" -f (ClassOf $fg), $fgPid, $fgName)

# ---- the agent's own account of itself --------------------------------
$log = Join-Path $env:LOCALAPPDATA 'Liro\logs\bridge.log'
Say ""
Say "---- the agent's log, by session ----"
Say "  reg      = drop targets registered      entered = drags that reached a window"
Say "  covered  = times another window was in front of it"
if (-not (Test-Path $log)) { Say "  no log at $log" }
else {
  $sessions = New-Object System.Collections.ArrayList
  $cur = $null
  foreach ($line in (Get-Content $log)) {
    $j = $null
    try { $j = $line | ConvertFrom-Json } catch { continue }
    if ($j.msg -match 'liro-bridge starting') {
      if ($cur) { [void]$sessions.Add($cur) }
      $cur = [pscustomobject]@{ t = $j.time; ver = $j.version; commit = $j.commit; reg = 0; late = 0; entered = 0; dropped = 0; covered = 0 }
    } elseif ($cur) {
      if     ($j.msg -match 'appeared later')          { $cur.late++; $cur.reg++ }
      elseif ($j.msg -match 'registered a drop target'){ $cur.reg++ }
      elseif ($j.msg -match 'drag entered')            { $cur.entered++ }
      elseif ($j.msg -match 'drop received')           { $cur.dropped++ }
      elseif ($j.msg -match 'in front of this one')    { $cur.covered++ }
    }
  }
  if ($cur) { [void]$sessions.Add($cur) }
  $windowed = @($sessions | Where-Object { $_.reg -gt 0 })
  Say ""
  Say "  start                     version   commit    reg  late entered dropped covered"
  foreach ($s in ($windowed | Select-Object -Last 15)) {
    Say ("  {0,-25} {1,-9} {2,-9} {3,-4} {4,-4} {5,-7} {6,-7} {7}" -f `
      $s.t, $s.ver, $s.commit, $s.reg, $s.late, $s.entered, $s.dropped, $s.covered)
  }
  Say ""
  Say "---- the last 60 drag-related log lines ----"
  Get-Content $log | Where-Object { $_ -match 'drop target|will not accept|drag entered|drop received|in front of this one|documents added' } |
    Select-Object -Last 60 | ForEach-Object {
      $j = $null
      try { $j = $_ | ConvertFrom-Json } catch { Say ("  " + $_); return }
      $extra = @()
      foreach ($f in 'class','paths','pid','page') { if ($j.$f) { $extra += ("{0}={1}" -f $f, $j.$f) } }
      Say ("  {0}  {1}  {2}" -f $j.time, $j.msg, ($extra -join ' '))
    }
}

Say ""
Say "---- the one gesture this report cannot take for you ----"
Say "  While it is still failing, drag the same file onto something that is NOT"
Say "  Liro Bridge - a second Explorer window, or Notepad."
Say "    it refuses there too  -> the desktop's drag delivery is wedged; not the agent."
Say "    it works there        -> the drag is fine and the agent is not receiving it."
Say "  Write down which, and send this file."
Say ""

$dest = Join-Path ([Environment]::GetFolderPath('Desktop')) ("liro-drag-report-{0}.txt" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
$out | Out-File -FilePath $dest -Encoding utf8
Write-Host ""
Write-Host "  written to: $dest" -ForegroundColor Cyan
Write-Host ""
