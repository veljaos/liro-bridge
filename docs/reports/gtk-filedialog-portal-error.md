# GtkFileDialog never answers when the file chooser portal returns an error

*Text for the GNOME GitLab issue (GTK). To be filed by the owner — this
machine has no account there (open-items A26, D-385). Measured in D-408.*

---

**GtkFileDialog never answers when the FileChooser portal returns an error**

GTK 4.22.5 (Fedora 44, GNOME 50, Wayland), xdg-desktop-portal 1.22.1,
xdg-desktop-portal-gnome 50.0.

When `org.freedesktop.portal.FileChooser.OpenFile` is answered with a D-Bus
error, `gtk_file_dialog_open_multiple()` never calls its callback. No dialog
appears, nothing is shown, and the `GAsyncReadyCallback` is not invoked until
the caller cancels the `GCancellable` itself (then with
`GTK_DIALOG_ERROR_CANCELLED`). A caller that waits for the answer waits for
ever.

In `gtk/gtkfilechoosernativeportal.c`, `open_file_msg_cb` frees the request
and returns when the reply is an error:

```c
      /* FIXME: Show an error dialog here ? */
      g_error_free (error);
      return;
```

Commit d515311b59 ("filechoosernative: Make portals not fall back", 4.17.1)
removed the fallback to the GTK dialog that 4.14 had
(`portal_error_handler`), with the message "if it fails, show an error
instead of falling back". No error is shown, and the response is never
emitted, so `GtkFileDialog`'s task is never completed.

Measured: a minimal PyGObject program calls `open_multiple` 0.8 s after its
window is presented. To make the portal refuse, the process calls
`prctl(PR_SET_DUMPABLE, 0)` first; xdg-desktop-portal then cannot open
`/proc/PID/root` and answers every call from it with an error. The bus
trace of that run:

```
method call … interface=org.freedesktop.portal.FileChooser; member=OpenFile
error … error_name=org.freedesktop.DBus.Error.AccessDenied reply_serial=108
   string "Portal operation not allowed: Unable to open /proc/6647/root"
```

The error arrives within 1 ms; the callback was not called in the 25 s
before the program cancelled. The same program without the `prctl` gets
the portal's dialog. Any portal error takes the same path — the refusal is
only the reliable way to produce one.

Reproducer: `chooser_control.py`, attached (from `scripts/portalprobe/` in
the reporting project). `python3 chooser_control.py nodump` shows the
missing callback; `dumpable` is the control.

Expected: the callback is called with an error (or, as the commit message
says, an error is shown), so that the caller learns the dialog did not
open. Folder selection (`select_folder`) sends the same `OpenFile` call
through the same callback; it was not measured separately.
