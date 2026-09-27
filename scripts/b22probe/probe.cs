// B22 probe - what Windows accessibility is told about the PIN dialog's text.
//
// A separate process from the holder. It reads the holder's PID from
// handover.json and touches nothing else on the machine: every lookup is
// filtered on that exact PID, and an HWND whose PID differs is skipped and
// counted.
//
// Four methods, as asked, plus two clearly-labelled additions:
//
//   1  UIA  - a client subscribed to value-changed and text-changed events,
//             then reading ValuePattern.Value, TextPattern.DocumentRange's
//             text, and IsPassword. B22 asks for the subscription "on all
//             windows", so there are two: one scoped to the dialog's subtree,
//             and 1d at the desktop root, installed before any window exists.
//             1d receives other processes' events; they are counted and dropped
//             without reading any property.
//   1b UIA  - LegacyIAccessiblePattern.Value: intended as a second provider
//             path, NEVER ATTEMPTED, and it says so in every report rather
//             than going quiet. The managed UIA client has no such pattern; it
//             exists only on the COM client, this machine has no type library
//             to bind it, and the interface is not IDispatch so late binding
//             cannot reach it. 2b reads the same provider through MSAA, which
//             is what LegacyIAccessible wraps.
//   2  MSAA - an out-of-process WinEvent hook (WINEVENT_OUTOFCONTEXT, idProcess
//             = the holder), AccessibleObjectFromEvent, get_accValue. The whole
//             event range is hooked rather than B22's three named events, so a
//             silence cannot be answered with "you hooked the wrong event".
//             **This is the method D-395 does not close** - see README, B24.
//   2b MSAA - AccessibleObjectFromWindow on the edit, get_accValue/accName/
//             accState. An addition: the hook only fires if the provider
//             raises, and a direct read answers a different question.
//   3  WM_GETTEXT - cross-process SendMessage, with WM_GETTEXTLENGTH.
//
// It never prints what it captured. For each capture it prints the length, a
// verdict against that phase's needle (exact / prefix / mask characters /
// something else), and for "something else" a character-class summary and the
// first 8 hex of a SHA-256, so an unexpected string is identifiable across
// runs without being readable.
//
// Polling is 10 Hz. The holder tells the owner to wait two seconds before
// pressing Enter, which is 20 polls of steady state - so a miss here is not a
// sampling miss.

using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Security.Cryptography;
using System.Text;
using System.Threading;
using System.Windows.Automation;
using System.Windows.Automation.Text;
using Accessibility;

static class Probe
{
    // ---------- win32 ----------
    const uint WINEVENT_OUTOFCONTEXT = 0x0000;
    // B22 names EVENT_OBJECT_VALUECHANGE, EVENT_OBJECT_NAMECHANGE "and the
    // text-edit events". The whole WinEvent range is hooked instead of those
    // three, because the hook is already filtered to one process and a wider
    // range makes "no event was delivered" a stronger statement than a narrow
    // one does: it cannot be answered with "you hooked the wrong event".
    const uint EVENT_MIN = 0x00000001, EVENT_MAX = 0x7FFFFFFF;
    const uint OBJID_CLIENT = 0xFFFFFFFC;
    const int WM_GETTEXT = 0x000D, WM_GETTEXTLENGTH = 0x000E;
    const int STATE_SYSTEM_PROTECTED = 0x20000000;

    delegate void WinEventProc(IntPtr hook, uint ev, IntPtr hwnd, int idObject, int idChild, uint thread, uint time);

    [DllImport("user32.dll")] static extern IntPtr SetWinEventHook(uint min, uint max, IntPtr hmod, WinEventProc cb, uint pid, uint tid, uint flags);
    [DllImport("user32.dll")] static extern bool UnhookWinEvent(IntPtr hook);
    [DllImport("user32.dll")] static extern int GetMessage(out MSG msg, IntPtr hwnd, uint min, uint max);
    [DllImport("user32.dll")] static extern bool TranslateMessage(ref MSG msg);
    [DllImport("user32.dll")] static extern IntPtr DispatchMessage(ref MSG msg);
    [DllImport("user32.dll")] static extern bool PostThreadMessage(uint tid, uint msg, IntPtr w, IntPtr l);
    [DllImport("kernel32.dll")] static extern uint GetCurrentThreadId();
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc cb, IntPtr p);
    [DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr parent, EnumProc cb, IntPtr p);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr hwnd, StringBuilder sb, int max);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll")] static extern int GetWindowLong(IntPtr hwnd, int index);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern IntPtr SendMessage(IntPtr hwnd, int msg, IntPtr w, StringBuilder l);
    [DllImport("user32.dll")] static extern IntPtr SendMessage(IntPtr hwnd, int msg, IntPtr w, IntPtr l);
    [DllImport("oleacc.dll")] static extern int AccessibleObjectFromEvent(IntPtr hwnd, uint id, uint child, out IAccessible acc, [MarshalAs(UnmanagedType.Struct)] out object childId);
    [DllImport("oleacc.dll")] static extern int AccessibleObjectFromWindow(IntPtr hwnd, uint id, ref Guid iid, [MarshalAs(UnmanagedType.Interface)] out object acc);

    delegate bool EnumProc(IntPtr hwnd, IntPtr p);
    [StructLayout(LayoutKind.Sequential)] struct MSG { public IntPtr h; public uint m; public IntPtr w; public IntPtr l; public uint t; public int x; public int y; }

    // ---------- handover ----------
    class Handover { public string run_id, started, needle_a, needle_b, needle_c, class_a, class_b, class_c; public int pid; }

    static Handover H;
    static uint TargetPid;
    static int foreignHwndsSkipped = 0;

    // ---------- findings ----------
    // One row per (phase, method). Worst case is kept: the strongest thing the
    // method ever saw for that phase.
    class Row
    {
        public int Events;            // subscription deliveries for this phase
        public bool Attempted;        // the method actually ran against a found element
        public string Error;          // why it could not run, if it could not
        public int BestLen = -1;      // -1: nothing ever returned
        public string Verdict = "no observation recorded";
        public int Rank;              // 0 nothing .. 5 exact needle
        public string Extra = "";
    }
    static readonly Dictionary<string, Row> Rows = new Dictionary<string, Row>();
    static Row R(string phase, string method)
    {
        string k = phase + "/" + method;
        if (!Rows.ContainsKey(k)) Rows[k] = new Row();
        return Rows[k];
    }

    // Every method gets a row in every phase the moment that phase's window is
    // found, so that a method which never ran prints a line saying so. The dry
    // run printed no row at all for the WinEvent hook in two phases, and a
    // missing row reads as a clean absence - which is the one thing this whole
    // exercise is built to not do.
    static readonly string[] Methods = {
        "0 lookup", "1 UIA IsPassword", "1 UIA value", "1 UIA text range",
        "1b UIA legacy", "1c UIA Name", "1d UIA all-win val", "1d UIA all-win txt",
        "2 MSAA WinEvent", "2b MSAA direct", "2c MSAA accName", "3 WM_GETTEXT",
    };
    static void EnsureRows(string phase) { foreach (string m in Methods) R(phase, m); }

    // The longest run of the needle present in a string. Character overlap is
    // not evidence: a label sharing letters with a random needle is ordinary,
    // and the dry run duly flagged a 75-character prompt for "containing
    // needle characters". A run of four is not ordinary.
    static int LongestRun(string got, string needle)
    {
        int best = 0;
        for (int i = 0; i < needle.Length; i++)
            for (int len = needle.Length - i; len > best; len--)
                if (got.IndexOf(needle.Substring(i, len), StringComparison.Ordinal) >= 0)
                { best = len; break; }
        return best;
    }

    static string Needle(string phase)
    {
        if (phase == "A") return H.needle_a;
        if (phase == "B") return H.needle_b;
        return H.needle_c;
    }

    // Classify without ever printing the text.
    static void Record(string phase, string method, string got, string note)
    {
        Row r = R(phase, method);
        r.Attempted = true;
        if (got == null)
        {
            // The REASON a method returned nothing is the finding, not a
            // detail. The dry run threw it away and printed "nothing
            // returned", which says nothing about whether Windows refused or
            // the probe was broken.
            if (r.Rank == 0)
            {
                r.Verdict = "no value returned";
                if (!string.IsNullOrEmpty(note)) r.Extra = note.Trim();
            }
            return;
        }
        string needle = Needle(phase);
        int rank; string verdict; string extra = note ?? "";

        if (got.Length == 0) { rank = 1; verdict = "empty string"; }
        else if (got == needle) { rank = 5; verdict = "EXACT NEEDLE"; }
        else if (needle.StartsWith(got, StringComparison.Ordinal)) { rank = 4; verdict = "PREFIX OF NEEDLE (" + got.Length + " of " + needle.Length + ")"; }
        else if (got.All(c => c == '●' || c == '*' || c == '•'))
        { rank = 2; verdict = "mask characters only"; extra += " mask=U+" + ((int)got[0]).ToString("X4"); }
        else
        {
            // Not the needle, not a mask. Could still be a leak of something
            // else, or an unrelated label. Identify it without showing it.
            int run = LongestRun(got, needle);
            rank = run >= 4 ? 3 : 2;
            verdict = run >= 4
                ? "other text, CONTAINS A " + run + "-CHARACTER RUN OF THE NEEDLE"
                : "other text, longest needle run " + run + " (noise)";
            extra += " sha256[0:8]=" + Sha(got) + " classes=" + Classes(got);
        }
        // Also catch a needle embedded in a longer string.
        if (got.Length > 0 && got != needle && got.IndexOf(needle, StringComparison.Ordinal) >= 0)
        { rank = 5; verdict = "NEEDLE FOUND INSIDE A LONGER STRING"; }

        if (rank > r.Rank || (rank == r.Rank && got.Length > r.BestLen))
        { r.Rank = rank; r.Verdict = verdict; r.BestLen = got.Length; r.Extra = extra.Trim(); }
    }

    static void Fail(string phase, string method, string why)
    {
        Row r = R(phase, method);
        if (r.Error == null) r.Error = why;
    }

    // Why an exception happened, in the form the report needs: the type, the
    // HRESULT and the provider's own sentence. The dry run printed
    // "InvalidOperationException hr=0x80131509" for the two password phases,
    // which does not distinguish a provider refusing from a client mistake.
    static string Why(Exception e)
    {
        string m = e.Message ?? "";
        if (m.Length > 160) m = m.Substring(0, 160) + "...";
        return e.GetType().Name + " hr=0x" + e.HResult.ToString("X8") + " \"" + m.Replace("\r", " ").Replace("\n", " ") + "\"";
    }

    static string Sha(string s)
    {
        using (var h = SHA256.Create())
            return BitConverter.ToString(h.ComputeHash(Encoding.Unicode.GetBytes(s)), 0, 4).Replace("-", "").ToLowerInvariant();
    }
    static string Classes(string s)
    {
        int u = s.Count(char.IsUpper), l = s.Count(char.IsLower), d = s.Count(char.IsDigit),
            w = s.Count(char.IsWhiteSpace), o = s.Length;
        return string.Format(CultureInfo.InvariantCulture, "len={0} upper={1} lower={2} digit={3} space={4} other={5}",
            o, u, l, d, w, o - u - l - d - w);
    }

    // ---------- target lookup, PID-exact ----------
    static IntPtr FindTop(string cls)
    {
        IntPtr found = IntPtr.Zero;
        EnumWindows((hwnd, p) =>
        {
            uint pid; GetWindowThreadProcessId(hwnd, out pid);
            if (pid != TargetPid) { foreignHwndsSkipped++; return true; }
            if (!IsWindowVisible(hwnd)) return true;
            var sb = new StringBuilder(256);
            GetClassName(hwnd, sb, sb.Capacity);
            if (sb.ToString() != cls) return true;
            found = hwnd; return false;
        }, IntPtr.Zero);
        return found;
    }

    static IntPtr FindEdit(IntPtr parent)
    {
        IntPtr found = IntPtr.Zero;
        EnumChildWindows(parent, (hwnd, p) =>
        {
            uint pid; GetWindowThreadProcessId(hwnd, out pid);
            if (pid != TargetPid) { foreignHwndsSkipped++; return true; }
            var sb = new StringBuilder(256);
            GetClassName(hwnd, sb, sb.Capacity);
            if (!sb.ToString().Equals("Edit", StringComparison.OrdinalIgnoreCase)) return true;
            found = hwnd; return false;
        }, IntPtr.Zero);
        return found;
    }

    // ---------- method 3: cross-process WM_GETTEXT ----------
    static void WmGetText(string phase, IntPtr edit)
    {
        try
        {
            IntPtr len = SendMessage(edit, WM_GETTEXTLENGTH, IntPtr.Zero, IntPtr.Zero);
            var sb = new StringBuilder(512);
            IntPtr got = SendMessage(edit, WM_GETTEXT, (IntPtr)sb.Capacity, sb);
            Record(phase, "3 WM_GETTEXT", sb.ToString(),
                "WM_GETTEXTLENGTH=" + len.ToInt64() + " WM_GETTEXT returned=" + got.ToInt64());
        }
        catch (Exception e) { Fail(phase, "3 WM_GETTEXT", e.GetType().Name + ": " + e.Message); }
    }

    // ---------- method 2b: direct MSAA read ----------
    static void MsaaDirect(string phase, IntPtr edit)
    {
        try
        {
            Guid iid = new Guid("618736e0-3c3d-11cf-810c-00aa00389b71"); // IID_IAccessible
            object o;
            int hr = AccessibleObjectFromWindow(edit, OBJID_CLIENT, ref iid, out o);
            if (hr != 0 || !(o is IAccessible)) { Fail(phase, "2b MSAA direct", "AccessibleObjectFromWindow hr=0x" + hr.ToString("X8")); return; }
            var acc = (IAccessible)o;
            object self = 0;
            string note = "";
            try
            {
                int st = Convert.ToInt32(acc.get_accState(self), CultureInfo.InvariantCulture);
                note = "accState protected=" + ((st & STATE_SYSTEM_PROTECTED) != 0 ? "YES" : "no");
            }
            catch (Exception e) { note = "accState threw " + e.GetType().Name; }
            string v = null;
            try
            {
                v = acc.get_accValue(self);
                if (v == null) note += "; get_accValue returned null with no error";
            }
            catch (Exception e)
            {
                note += "; get_accValue REFUSED: " + Why(e)
                     + (((uint)e.HResult) == 0x80070005u ? " <- E_ACCESSDENIED" : "");
            }
            Record(phase, "2b MSAA direct", v, note);
        }
        catch (Exception e) { Fail(phase, "2b MSAA direct", Why(e)); }
    }

    // ---------- method 1 / 1b: UIA ----------
    static readonly HashSet<string> subscribed = new HashSet<string>();
    static readonly List<AutomationEventHandler> keepEv = new List<AutomationEventHandler>();
    static readonly List<AutomationPropertyChangedEventHandler> keepProp = new List<AutomationPropertyChangedEventHandler>();

    // B22 (1) asks for "a client subscribed to text-changed and
    // property-changed (Value) events ON ALL WINDOWS while the needle is
    // typed". The per-window subscription below is narrower than that, so this
    // one is installed as well, at the desktop root, once, BEFORE any of the
    // three windows exists - which is also the only way to be subscribed while
    // the needle is typed rather than after.
    //
    // It is desktop-wide by B22's wording, so it receives events belonging to
    // other processes. Those are counted and dropped: no property of an element
    // whose process is not the holder is ever read. "Exact PID only" is kept
    // where it matters - what is read - rather than where it would mean not
    // doing what B22 asks.
    static int allWinForeignDropped = 0, allWinTargetDeliveries = 0;
    static string allWinSetupError = null;
    static AutomationPropertyChangedEventHandler allWinProp;
    static AutomationEventHandler allWinText;

    static void SubscribeAllWindows()
    {
        try
        {
            allWinProp = new AutomationPropertyChangedEventHandler((s, e) =>
            {
                var src = s as AutomationElement;
                try { if (src == null || src.Current.ProcessId != (int)TargetPid) { allWinForeignDropped++; return; } }
                catch { allWinForeignDropped++; return; }
                allWinTargetDeliveries++;
                string ph = currentPhase; if (ph == null) return;
                Row row = R(ph, "1d UIA all-win val");
                row.Events++;
                var nv = e.NewValue as string;
                if (nv != null) Record(ph, "1d UIA all-win val", nv, "desktop-wide ValueProperty change");
                else
                {
                    // An event arrived and carried no string. WHICH property
                    // changed is the difference between "the provider said
                    // nothing" and "the provider announced a change and
                    // withheld the text", and the first dry run with this
                    // subscription hid both behind one blank line.
                    string pn = e.Property == null ? "(null property)" : e.Property.ProgrammaticName;
                    if (row.Extra.IndexOf(pn, StringComparison.Ordinal) < 0)
                        row.Extra = (row.Extra + " " + pn).Trim();
                }
            });
            Automation.AddAutomationPropertyChangedEventHandler(
                AutomationElement.RootElement, TreeScope.Subtree, allWinProp,
                ValuePattern.ValueProperty, AutomationElement.NameProperty);
        }
        catch (Exception e) { allWinSetupError = "value: " + Why(e); }

        try
        {
            allWinText = new AutomationEventHandler((s, e) =>
            {
                var src = s as AutomationElement;
                try { if (src == null || src.Current.ProcessId != (int)TargetPid) { allWinForeignDropped++; return; } }
                catch { allWinForeignDropped++; return; }
                allWinTargetDeliveries++;
                string ph = currentPhase; if (ph == null) return;
                R(ph, "1d UIA all-win txt").Events++;
                try
                {
                    var tp = src.GetCurrentPattern(TextPattern.Pattern) as TextPattern;
                    if (tp != null) Record(ph, "1d UIA all-win txt", tp.DocumentRange.GetText(-1), "read inside desktop-wide TextChanged");
                    else Fail(ph, "1d UIA all-win txt", "event arrived but the source exposes no TextPattern");
                }
                catch (Exception ex) { Fail(ph, "1d UIA all-win txt", "in-event read: " + Why(ex)); }
            });
            Automation.AddAutomationEventHandler(TextPattern.TextChangedEvent,
                AutomationElement.RootElement, TreeScope.Subtree, allWinText);
        }
        catch (Exception e) { allWinSetupError = (allWinSetupError ?? "") + " text: " + Why(e); }
    }

    static void Uia(string phase, IntPtr top, IntPtr edit)
    {
        AutomationElement el;
        try { el = AutomationElement.FromHandle(edit); }
        catch (Exception e) { Fail(phase, "1 UIA value", e.GetType().Name + ": " + e.Message); return; }
        if (el == null) { Fail(phase, "1 UIA value", "AutomationElement.FromHandle returned null"); return; }

        if (el.Current.ProcessId != (int)TargetPid)
        { Fail(phase, "1 UIA value", "element PID " + el.Current.ProcessId + " != target"); return; }

        // Subscription, scoped to the target's own subtree. Done once per phase.
        if (!subscribed.Contains(phase))
        {
            subscribed.Add(phase);
            string ph = phase;
            try
            {
                var pc = new AutomationPropertyChangedEventHandler((s, e) =>
                {
                    var src = s as AutomationElement;
                    try { if (src != null && src.Current.ProcessId != (int)TargetPid) return; } catch { return; }
                    R(ph, "1 UIA value").Events++;
                    var nv = e.NewValue as string;
                    if (nv != null) Record(ph, "1 UIA value", nv, "from ValueProperty change event");
                });
                keepProp.Add(pc);
                Automation.AddAutomationPropertyChangedEventHandler(
                    AutomationElement.FromHandle(top), TreeScope.Element | TreeScope.Descendants, pc,
                    ValuePattern.ValueProperty, AutomationElement.NameProperty);
            }
            catch (Exception e) { Fail(phase, "1 UIA value", "value-change subscription: " + e.GetType().Name + ": " + e.Message); }

            try
            {
                var th = new AutomationEventHandler((s, e) =>
                {
                    var src = s as AutomationElement;
                    try { if (src != null && src.Current.ProcessId != (int)TargetPid) return; } catch { return; }
                    R(ph, "1 UIA text range").Events++;
                    try
                    {
                        var tp = src.GetCurrentPattern(TextPattern.Pattern) as TextPattern;
                        if (tp != null) Record(ph, "1 UIA text range", tp.DocumentRange.GetText(-1), "read inside TextChanged event");
                    }
                    catch (Exception ex) { Fail(ph, "1 UIA text range", "in-event read: " + ex.GetType().Name); }
                });
                keepEv.Add(th);
                Automation.AddAutomationEventHandler(TextPattern.TextChangedEvent,
                    AutomationElement.FromHandle(top), TreeScope.Element | TreeScope.Descendants, th);
            }
            catch (Exception e) { Fail(phase, "1 UIA text range", "text-change subscription: " + e.GetType().Name + ": " + e.Message); }
        }

        // IsPassword, on the element itself.
        try
        {
            bool pw = el.Current.IsPassword;
            R(phase, "1 UIA IsPassword").Attempted = true;
            R(phase, "1 UIA IsPassword").Verdict = pw ? "IsPassword = TRUE" : "IsPassword = FALSE";
            R(phase, "1 UIA IsPassword").BestLen = 0;
        }
        catch (Exception e) { Fail(phase, "1 UIA IsPassword", e.GetType().Name + ": " + e.Message); }

        // ValuePattern.
        try
        {
            object p;
            if (el.TryGetCurrentPattern(ValuePattern.Pattern, out p))
                Record(phase, "1 UIA value", ((ValuePattern)p).Current.Value, "polled ValuePattern.Value");
            else
                Fail(phase, "1 UIA value", "the element does not support ValuePattern");
        }
        catch (Exception e) { Fail(phase, "1 UIA value", Why(e)); }

        // TextPattern document range.
        try
        {
            object p;
            if (el.TryGetCurrentPattern(TextPattern.Pattern, out p))
            {
                var tp = (TextPattern)p;
                Record(phase, "1 UIA text range", tp.DocumentRange.GetText(-1), "polled DocumentRange.GetText(-1)");
            }
            else Fail(phase, "1 UIA text range", "the element does not support TextPattern (TryGetCurrentPattern returned false)");
        }
        catch (Exception e) { Fail(phase, "1 UIA text range", Why(e)); }

        // 1b: LegacyIAccessible was to be the other UIA provider path to the
        // same control. It is not reachable from this client and this row says
        // so rather than reporting a silence: System.Windows.Automation (the
        // managed UIA client) exposes no LegacyIAccessiblePattern - the pattern
        // exists only on the COM client (IUIAutomationLegacyIAccessiblePattern,
        // pattern id 10018), which needs an interop assembly this machine has
        // no type library for, and the interface is not IDispatch so late
        // binding cannot reach it either. Method 2b reads the same provider
        // through MSAA directly, which is what LegacyIAccessible wraps.
        Fail(phase, "1b UIA legacy", "NOT ATTEMPTED - no LegacyIAccessiblePattern in the managed UIA client; see 2b");

        // Name, because a control that puts its text in Name leaks it there.
        try { Record(phase, "1c UIA Name", el.Current.Name, "AutomationElement.Current.Name"); }
        catch (Exception e) { Fail(phase, "1c UIA Name", e.GetType().Name); }
    }

    // ---------- method 2: out-of-process WinEvent hook ----------
    static volatile string currentPhase = null;
    static IntPtr hook = IntPtr.Zero;
    static WinEventProc hookDelegate;   // kept alive deliberately
    static uint hookThreadId;
    static int hookEventsTotal = 0;
    static string hookSetupError = null;

    static void HookThread()
    {
        hookThreadId = GetCurrentThreadId();
        hookDelegate = OnWinEvent;
        hook = SetWinEventHook(EVENT_MIN, EVENT_MAX, IntPtr.Zero, hookDelegate, TargetPid, 0, WINEVENT_OUTOFCONTEXT);
        if (hook == IntPtr.Zero)
        {
            hookSetupError = "SetWinEventHook returned 0, last error " + Marshal.GetLastWin32Error();
            return;
        }
        MSG msg;
        while (GetMessage(out msg, IntPtr.Zero, 0, 0) > 0)
        {
            if (msg.m == 0x0400) break; // our own quit
            TranslateMessage(ref msg);
            DispatchMessage(ref msg);
        }
        UnhookWinEvent(hook);
    }

    static void OnWinEvent(IntPtr h, uint ev, IntPtr hwnd, int idObject, int idChild, uint thread, uint time)
    {
        uint pid; GetWindowThreadProcessId(hwnd, out pid);
        if (pid != TargetPid) { foreignHwndsSkipped++; return; }
        hookEventsTotal++;
        string phase = currentPhase;
        if (phase == null) return;
        R(phase, "2 MSAA WinEvent").Events++;
        try
        {
            IAccessible acc; object child;
            int hr = AccessibleObjectFromEvent(hwnd, (uint)idObject, (uint)idChild, out acc, out child);
            if (hr != 0 || acc == null)
            { Fail(phase, "2 MSAA WinEvent", "AccessibleObjectFromEvent hr=0x" + hr.ToString("X8")); return; }
            string note = "event=0x" + ev.ToString("X4");
            try
            {
                int st = Convert.ToInt32(acc.get_accState(child), CultureInfo.InvariantCulture);
                note += " protected=" + ((st & STATE_SYSTEM_PROTECTED) != 0 ? "YES" : "no");
            }
            catch { }
            string v = null;
            try
            {
                v = acc.get_accValue(child);
                if (v == null) note += " get_accValue returned null with no error";
            }
            catch (Exception e)
            {
                note += " get_accValue REFUSED: " + Why(e)
                     + (((uint)e.HResult) == 0x80070005u ? " <- E_ACCESSDENIED" : "");
            }
            Record(phase, "2 MSAA WinEvent", v, note);
            try { Record(phase, "2c MSAA accName", acc.get_accName(child), "from the same event object"); } catch { }
        }
        catch (Exception e) { Fail(phase, "2 MSAA WinEvent", e.GetType().Name + ": " + e.Message); }
    }

    // ---------- main ----------
    [STAThread]
    static int Main(string[] argv)
    {
        string dir = argv.Length > 0 ? argv[0] : Directory.GetCurrentDirectory();
        string hp = Path.Combine(dir, "handover.json");
        string dp = Path.Combine(dir, "phases-done");
        if (!File.Exists(hp)) { Console.WriteLine("no handover.json in " + dir + " - start the holder first."); return 2; }
        H = ParseHandover(File.ReadAllText(hp));
        TargetPid = (uint)H.pid;

        Process proc;
        try { proc = Process.GetProcessById(H.pid); }
        catch (Exception e) { Console.WriteLine("the holder PID " + H.pid + " is not running: " + e.Message); return 2; }

        Console.WriteLine();
        Console.WriteLine("  PROBE   watching pid=" + H.pid + " (" + proc.ProcessName + ")  run=" + H.run_id);
        Console.WriteLine("  started " + DateTime.Now.ToString("o"));
        Console.WriteLine("  Every lookup is filtered on that PID. Nothing else is read.");
        Console.WriteLine("  Press Enter in the holder now. This window prints its report when");
        Console.WriteLine("  all three phases are done.");
        Console.WriteLine();

        var hookThread = new Thread(HookThread) { IsBackground = true, Name = "winevent" };
        hookThread.SetApartmentState(ApartmentState.STA);
        hookThread.Start();
        Thread.Sleep(300);
        Console.WriteLine(hookSetupError == null
            ? "  method 2: WinEvent hook installed on pid " + H.pid + " (whole event range)."
            : "  method 2: HOOK FAILED - " + hookSetupError);

        SubscribeAllWindows();
        Console.WriteLine(allWinSetupError == null
            ? "  method 1d: desktop-wide UIA subscription installed, before any window exists."
            : "  method 1d: DESKTOP-WIDE SUBSCRIPTION FAILED - " + allWinSetupError);

        var seen = new HashSet<string>();
        var deadline = DateTime.UtcNow.AddMinutes(20);
        string lastPhase = null;

        while (DateTime.UtcNow < deadline)
        {
            if (File.Exists(dp) && seen.Count > 0) break;
            if (proc.HasExited) break;

            foreach (var pair in new[] {
                new[]{"A", H.class_a}, new[]{"B", H.class_b}, new[]{"C", H.class_c} })
            {
                IntPtr top = FindTop(pair[1]);
                if (top == IntPtr.Zero) continue;
                currentPhase = pair[0];
                if (pair[0] != lastPhase)
                {
                    lastPhase = pair[0];
                    Console.WriteLine("  phase " + pair[0] + ": window found (class " + pair[1] + "), measuring...");
                }
                seen.Add(pair[0]);
                EnsureRows(pair[0]);
                IntPtr edit = FindEdit(top);
                if (edit == IntPtr.Zero)
                {
                    Fail(pair[0], "0 lookup", "no Edit child under the window");
                    continue;
                }
                int style = GetWindowLong(edit, -16); // GWL_STYLE
                R(pair[0], "0 lookup").Attempted = true;
                R(pair[0], "0 lookup").Verdict = "edit found, ES_PASSWORD " + (((style & 0x20) != 0) ? "SET" : "clear");
                R(pair[0], "0 lookup").BestLen = 0;

                WmGetText(pair[0], edit);
                MsaaDirect(pair[0], edit);
                Uia(pair[0], top, edit);
            }
            Thread.Sleep(100);
        }
        // Let late events land.
        Thread.Sleep(600);
        currentPhase = null;
        PostThreadMessage(hookThreadId, 0x0400, IntPtr.Zero, IntPtr.Zero);

        // The report goes to the console AND to a file. The dry-run harness read
        // this program's stdout through a pipe it drained only at the end, the
        // pipe filled, and four of phase C's rows never arrived - so the report
        // I read was missing exactly the rows a reader would check. A file is
        // not a convenience here: a truncated report is a wrong one.
        var transcript = new List<string>();
        Action<string> Say = line => { Console.WriteLine(line); transcript.Add(line); };

        Say("");
        Say("  ================ B22 PROBE REPORT ================");
        Say("  run " + H.run_id + "   holder pid " + H.pid + "   finished " + DateTime.Now.ToString("o"));
        Say("  WinEvent deliveries from that pid, all phases: " + hookEventsTotal);
        Say("  Desktop-wide UIA deliveries from that pid: " + allWinTargetDeliveries
            + "; from other processes, dropped without reading any property: " + allWinForeignDropped);
        Say("  HWNDs skipped for belonging to another process: " + foreignHwndsSkipped + " (counted per lookup, so it grows with the 10 Hz poll)");
        Say("  Captured text is never printed.");
        foreach (string phase in new[] { "A", "B", "C" })
        {
            string label = phase == "A" ? "A  plain edit, no ES_PASSWORD   [INSTRUMENT CONTROL]"
                         : phase == "B" ? "B  bare edit, ES_PASSWORD set"
                                        : "C  the real Liro Bridge PIN dialog";
            Say("");
            Say("  --- phase " + label);
            if (!seen.Contains(phase)) { Say("      never seen - the window did not appear while the probe was running."); continue; }
            foreach (var k in Rows.Keys.Where(x => x.StartsWith(phase + "/", StringComparison.Ordinal)).OrderBy(x => x))
            {
                Row r = Rows[k];
                string m = k.Substring(2);
                string line = "      " + m.PadRight(20) + " : ";
                bool hookRow = m.StartsWith("2 MSAA WinEvent", StringComparison.Ordinal)
                            || m.StartsWith("2c MSAA accName", StringComparison.Ordinal);
                bool allWinRow = m.StartsWith("1d UIA all-win", StringComparison.Ordinal);
                if (hookRow && r.Events == 0 && r.Rank == 0 && !r.Attempted)
                    line += (hookSetupError == null
                        ? "NO EVENT DELIVERED for this phase (hook was installed; 0 deliveries while this window was up)"
                        : "hook was never installed - " + hookSetupError);
                else if (allWinRow && r.Events == 0 && r.Rank == 0 && !r.Attempted)
                    line += (allWinSetupError == null
                        ? "NO EVENT DELIVERED for this phase (desktop-wide subscription was live; 0 from this pid while this window was up)"
                        : "subscription failed - " + allWinSetupError);
                else if (!r.Attempted && r.Error != null)
                {
                    line += "COULD NOT RUN - " + r.Error;
                    // An event that arrived and then could not be read is a
                    // different statement from no event at all, and the count
                    // is the only thing that separates them.
                    if (r.Events > 0) line += "  (but events=" + r.Events + " DID arrive)";
                }
                else if (!r.Attempted && r.Events > 0)
                    line += "events=" + r.Events + " delivered, NONE carried a string value"
                          + (string.IsNullOrEmpty(r.Extra) ? "" : "  [properties seen:" + r.Extra + "]");
                else if (!r.Attempted && r.Error == null)
                    line += "did not run and recorded no reason - read this as a defect in the probe, not as a result";
                else
                {
                    line += r.Verdict;
                    if (r.BestLen > 0) line += "  len=" + r.BestLen;
                    if (r.Events > 0) line += "  events=" + r.Events;
                    if (!string.IsNullOrEmpty(r.Extra)) line += "  [" + r.Extra + "]";
                    if (r.Error != null) line += "  (also: " + r.Error + ")";
                }
                Say(line);
            }
        }
        Say("");
        Say("  Read phase A first. If any method there reports no observation, an");
        Say("  empty string, or that it could not run, then that method is broken");
        Say("  and its silence in B and C is not evidence of anything.");
        Say("  ==================================================");

        string outPath = Path.Combine(dir, "b22-report-" + H.run_id + ".txt");
        try
        {
            File.WriteAllLines(outPath, transcript);
            Console.WriteLine();
            Console.WriteLine("  This report was also written to:");
            Console.WriteLine("    " + outPath);
        }
        catch (Exception e) { Console.WriteLine("  could not write the report file: " + e.Message); }
        Console.WriteLine();
        Console.WriteLine("  Press Enter to close.");
        Console.ReadLine();
        return 0;
    }

    // Tiny hand-rolled reader: no Newtonsoft on this machine, and
    // JavaScriptSerializer would be another assembly to depend on.
    static Handover ParseHandover(string s)
    {
        var h = new Handover();
        Func<string, string> str = key =>
        {
            int i = s.IndexOf("\"" + key + "\"", StringComparison.Ordinal);
            if (i < 0) return null;
            int c = s.IndexOf(':', i); int q1 = s.IndexOf('"', c + 1); int q2 = s.IndexOf('"', q1 + 1);
            return s.Substring(q1 + 1, q2 - q1 - 1);
        };
        h.run_id = str("run_id"); h.started = str("started");
        h.needle_a = str("needle_a"); h.needle_b = str("needle_b"); h.needle_c = str("needle_c");
        h.class_a = str("class_a"); h.class_b = str("class_b"); h.class_c = str("class_c");
        int p = s.IndexOf("\"pid\"", StringComparison.Ordinal);
        int col = s.IndexOf(':', p);
        int end = s.IndexOfAny(new[] { ',', '\n', '\r', '}' }, col + 1);
        h.pid = int.Parse(s.Substring(col + 1, end - col - 1).Trim(), CultureInfo.InvariantCulture);
        return h;
    }
}
