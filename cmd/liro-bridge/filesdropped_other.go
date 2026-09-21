//go:build !windows

package main

// filesDroppedHandler is nil on a platform whose window host does not
// implement drops yet, and asking anyway is a refusal rather than a
// silence.
//
// **This is what stopped the first Linux signing window from opening at
// all**, which is the useful half of the story: internal/ui's Linux
// host returns ErrDropNotImplemented when Options.OnFilesDropped is
// set (D-331, deliberately — "dropped files are refused rather than
// ignored"), and the signing window has always set it. So the refusal
// worked exactly as designed and the window it refused was the one the
// program needed.
//
// It is not implemented rather than not wanted, and the reason it is
// not implemented is a third instance of D-330's shape: gtk.DropTarget
// and its Drop signal are in the binding, gdk.GTypeFileList is in the
// binding, and gdk.FileList is a type with no methods at all —
// gdk_file_list_get_files, the one call that turns a drop into paths,
// is not generated. Reading a drop therefore needs hand-written cgo
// beside webkitjs_linux.c, which is F6 §1's port on this platform and
// is not F12 §3's window.
//
// What a person loses until then is dragging documents onto the window.
// **The window's own page still invites it**, which is a lie this seam
// does not fix and which belongs with whoever writes the drop target or
// the page's platform text — recorded rather than papered over.
func filesDroppedHandler(func([]string)) func([]string) { return nil }
