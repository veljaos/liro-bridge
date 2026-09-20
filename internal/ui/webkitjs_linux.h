/* Go -> page, by hand, because the binding does not offer it.
 *
 * gotk4-webkitgtk generates the *finishing* half of every asynchronous
 * WebKitGTK operation and none of the starting halves: 29 *Finish
 * functions across 1273, and not one parameter of type
 * GAsyncReadyCallback anywhere in the package. So
 * webkit_web_view_evaluate_javascript() -- the only way to run script
 * in the page, and what both Window.PostJSON and Window.Eval are --
 * cannot be reached through it at all. See D-330.
 *
 * This is the same answer F5 SS2.1 reached on Windows for the same
 * reason: where no binding exists, the interop is written by hand
 * against the C API rather than the program being redesigned around
 * what a binding happens to expose.
 *
 * Every pointer crosses this boundary as an integer. That is not
 * squeamishness: gotk4 hands out the view as a uintptr from
 * Object.Native(), Go's cgo pointer check rejected passing it back as a
 * typed pointer, and an integer is what it honestly is on the Go side
 * until C reconstitutes it. It also means this file needs no
 * unsafe.Pointer conversion on the Go side at all.
 */

#ifndef LIRO_WEBKITJS_H
#define LIRO_WEBKITJS_H

#include <webkit/webkit.h>

/* Starts an evaluation. view is a WebKitWebView*, token an opaque
 * number the Go side uses to find the waiting goroutine again; it is
 * never a Go pointer, which C memory may not hold. */
void liro_eval_start(unsigned long long view, const char *script,
                     unsigned long long token);

/* Completes one. Returns the value as JSON, which the caller frees with
 * g_free, or NULL with *errmsg set to a string the caller also frees. */
char *liro_eval_finish(unsigned long long view, unsigned long long res,
                       char **errmsg);

#endif
