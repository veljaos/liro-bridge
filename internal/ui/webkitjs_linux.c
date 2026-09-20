#include "webkitjs_linux.h"
#include "_cgo_export.h"

/* Adapts GAsyncReadyCallback's shape to the one Go exports. The
 * GAsyncResult goes to Go as a number and comes straight back to
 * liro_eval_finish; nothing Go-allocated crosses in either direction. */
static void liro_eval_cb(GObject *source, GAsyncResult *res,
                         gpointer user_data) {
    (void)source;
    liroEvalDone((unsigned long long)(uintptr_t)res,
                 (unsigned long long)(uintptr_t)user_data);
}

void liro_eval_start(unsigned long long view, const char *script,
                     unsigned long long token) {
    /* length -1: script is NUL-terminated. world_name and source_uri
     * NULL: the page's own world, and no source name to attribute a
     * stack trace to. No GCancellable -- the window closing is what
     * cancels, and that is handled on the Go side by the registry
     * entry going away. */
    webkit_web_view_evaluate_javascript((WebKitWebView *)(uintptr_t)view,
                                        script, -1, NULL, NULL, NULL,
                                        liro_eval_cb,
                                        (gpointer)(uintptr_t)token);
}

char *liro_eval_finish(unsigned long long view, unsigned long long res,
                       char **errmsg) {
    GError *error = NULL;
    JSCValue *value;
    char *json;

    value = webkit_web_view_evaluate_javascript_finish(
        (WebKitWebView *)(uintptr_t)view, (GAsyncResult *)(uintptr_t)res,
        &error);
    if (value == NULL) {
        *errmsg = g_strdup(error != NULL ? error->message
                                         : "evaluate_javascript failed");
        if (error != NULL) {
            g_error_free(error);
        }
        return NULL;
    }

    /* jsc_value_to_json is what makes Eval's contract expressible: the
     * Windows side returns "the JSON-encoded result", and this is the
     * same thing computed by the engine rather than by us.
     *
     * It returns NULL for a value it cannot encode -- `undefined` is
     * the everyday case -- and does so *without* setting a GError. That
     * is not a failure and must not be reported as one: NULL with no
     * error is indistinguishable, from Go, from NULL with an error
     * nobody set. So the distinction is made here, where the two cases
     * are still apart, and an unencodable value becomes the JSON "null"
     * that WebView2 returns for exactly the same thing. */
    json = jsc_value_to_json(value, 0);
    g_object_unref(value);
    if (json == NULL) {
        return g_strdup("null");
    }
    return json;
}
