//go:build darwin && cgo

package ui

// The macOS host (F13, [[D-463]]): AppKit and WKWebView, in Objective-C,
// and the Go functions that call into it. The Go functions AppKit calls
// back are in darwin_export.go — cgo allows only declarations in the
// preamble of a file that exports, so the two halves live apart.
//
// **Written on a Linux machine with no Mac, and compiled first by CI's
// macOS runner.** What the runner can and cannot show is in F13 §4: it
// opens a window, loads a page, carries a message each way and
// photographs it (darwin_window_test.go); it cannot show a person
// clicking Approve, a card, or Gatekeeper.
//
// The shape is the Linux host's, deliberately, so that one set of pages
// serves both: assets come from a liro:// scheme handler resolved by the
// same resolveAsset, and the page talks back through
// window.webkit.messageHandlers.liro — the name bridge.js already uses on
// WebKitGTK, because both are WebKit.

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

// The Go side (darwin_export.go).
extern void goRunOnMain(uintptr_t h);
extern void goWindowMessage(uintptr_t wid, char *json);
extern void goWindowLoaded(uintptr_t wid, char *err);
extern void goWindowClosed(uintptr_t wid);
extern void goWindowDropped(uintptr_t wid, char *paths);
extern int goAllowNavigation(uintptr_t wid, char *url);
extern void *goResolveAsset(uintptr_t wid, char *url, size_t *len, char **mime);
extern void goEvalDone(uintptr_t token, char *json);
extern void goTrayAction(int item);
extern char *goTrayLabels(void);
extern void goOpenURL(char *url);

// A liro-bridge:// link reaches a macOS application as an Apple Event,
// not in argv: LaunchServices starts the bundle (or finds it running)
// and sends kAEGetURL. The handler is installed before the loop runs, so
// the event that launched the application is not missed.
@interface LiroURLHandler : NSObject
- (void)handle:(NSAppleEventDescriptor *)event withReply:(NSAppleEventDescriptor *)reply;
@end

@implementation LiroURLHandler
- (void)handle:(NSAppleEventDescriptor *)event withReply:(NSAppleEventDescriptor *)reply {
	NSString *url = [[event paramDescriptorForKeyword:keyDirectObject] stringValue];
	if (url != nil) {
		goOpenURL((char *)url.UTF8String);
	}
}
@end

static LiroURLHandler *liroURLHandler;

static void liro_trampoline(void *ctx) { goRunOnMain((uintptr_t)ctx); }

void liro_dispatch(uintptr_t h) {
	dispatch_async_f(dispatch_get_main_queue(), (void *)h, liro_trampoline);
}

int liro_is_main(void) { return [NSThread isMainThread] ? 1 : 0; }

// The application runs as an accessory: no Dock icon and no menu bar of
// its own, which is what an agent that lives in the status bar is. It
// can still own windows that take the keyboard.
void liro_app_run(void) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		liroURLHandler = [LiroURLHandler new];
		[[NSAppleEventManager sharedAppleEventManager] setEventHandler:liroURLHandler
		                                                   andSelector:@selector(handle:withReply:)
		                                                 forEventClass:kInternetEventClass
		                                                    andEventID:kAEGetURL];
		[NSApp run];
	}
}

// -[NSApp stop:] takes effect after the next event, so one is posted.
void liro_app_stop(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp stop:nil];
		NSEvent *e = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
		                                location:NSZeroPoint
		                           modifierFlags:0
		                               timestamp:0
		                            windowNumber:0
		                                 context:nil
		                                 subtype:0
		                                   data1:0
		                                   data2:0];
		[NSApp postEvent:e atStart:YES];
	});
}

// liro_json encodes a JavaScript result or a message body as JSON. A
// value NSJSONSerialization cannot take at the top level (a string, a
// number) is wrapped in an array and unwrapped again; nil is "null".
static NSString *liro_json(id obj) {
	if (obj == nil || obj == [NSNull null]) {
		return @"null";
	}
	NSData *d = [NSJSONSerialization dataWithJSONObject:@[ obj ] options:0 error:nil];
	if (d == nil) {
		return @"null";
	}
	NSString *s = [[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding];
	if (s.length < 2) {
		return @"null";
	}
	return [s substringWithRange:NSMakeRange(1, s.length - 2)];
}

// WKWebView handles drags; these declarations only make its own
// NSDraggingDestination methods visible to the subclass below.
@interface WKWebView (LiroDragging)
- (NSDragOperation)draggingEntered:(id<NSDraggingInfo>)sender;
- (NSDragOperation)draggingUpdated:(id<NSDraggingInfo>)sender;
- (BOOL)performDragOperation:(id<NSDraggingInfo>)sender;
@end

// LiroWebView takes dropped files itself when the window accepts them,
// rather than letting WebKit navigate to them: a file:// navigation is
// refused by the policy below anyway, and a page never sees a path.
@interface LiroWebView : WKWebView
@property(nonatomic) uintptr_t wid;
@property(nonatomic) BOOL acceptsFiles;
@end

@implementation LiroWebView
- (NSArray<NSURL *> *)liroFiles:(id<NSDraggingInfo>)sender {
	NSArray *urls = [sender.draggingPasteboard readObjectsForClasses:@[ [NSURL class] ]
	                                                         options:@{NSPasteboardURLReadingFileURLsOnlyKey : @YES}];
	return urls ?: @[];
}
- (NSDragOperation)draggingEntered:(id<NSDraggingInfo>)sender {
	if (self.acceptsFiles && [self liroFiles:sender].count > 0) {
		return NSDragOperationCopy;
	}
	return [super draggingEntered:sender];
}
- (NSDragOperation)draggingUpdated:(id<NSDraggingInfo>)sender {
	if (self.acceptsFiles && [self liroFiles:sender].count > 0) {
		return NSDragOperationCopy;
	}
	return [super draggingUpdated:sender];
}
- (BOOL)performDragOperation:(id<NSDraggingInfo>)sender {
	NSArray<NSURL *> *urls = [self liroFiles:sender];
	if (self.acceptsFiles && urls.count > 0) {
		NSMutableArray *paths = [NSMutableArray array];
		for (NSURL *u in urls) {
			if (u.path) [paths addObject:u.path];
		}
		goWindowDropped(self.wid, (char *)[[paths componentsJoinedByString:@"\n"] UTF8String]);
		return YES;
	}
	return [super performDragOperation:sender];
}
@end

// LiroSchemeHandler serves liro://<host>/<path> from the window's own
// asset hosts, through Go's resolveAsset.
@interface LiroSchemeHandler : NSObject <WKURLSchemeHandler>
@property(nonatomic) uintptr_t wid;
@end

@implementation LiroSchemeHandler
- (void)webView:(WKWebView *)webView startURLSchemeTask:(id<WKURLSchemeTask>)task {
	NSURL *url = task.request.URL;
	size_t n = 0;
	char *mime = NULL;
	void *data = goResolveAsset(self.wid, (char *)url.absoluteString.UTF8String, &n, &mime);
	if (data == NULL) {
		[task didFailWithError:[NSError errorWithDomain:@"rs.liro.bridge" code:404 userInfo:nil]];
		return;
	}
	NSString *type = [NSString stringWithUTF8String:mime];
	free(mime);
	NSHTTPURLResponse *resp = [[NSHTTPURLResponse alloc]
	     initWithURL:url
	      statusCode:200
	     HTTPVersion:@"HTTP/1.1"
	    headerFields:@{@"Content-Type" : type, @"Content-Length" : [NSString stringWithFormat:@"%zu", n]}];
	[task didReceiveResponse:resp];
	[task didReceiveData:[NSData dataWithBytesNoCopy:data length:n freeWhenDone:YES]];
	[task didFinish];
}
- (void)webView:(WKWebView *)webView stopURLSchemeTask:(id<WKURLSchemeTask>)task {
}
@end

// LiroController is one window's delegate for everything: the page's
// messages, navigation, new-window requests and the window closing.
@interface LiroController : NSObject <WKScriptMessageHandler, WKNavigationDelegate, WKUIDelegate, NSWindowDelegate>
@property(nonatomic) uintptr_t wid;
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) LiroWebView *view;
@property(nonatomic) BOOL closingFromGo;
@end

static NSMutableDictionary<NSNumber *, LiroController *> *liroWindows;

static void liro_forget(LiroController *c) {
	[c.view.configuration.userContentController removeScriptMessageHandlerForName:@"liro"];
	c.view.navigationDelegate = nil;
	c.view.UIDelegate = nil;
	c.window.delegate = nil;
	[liroWindows removeObjectForKey:@(c.wid)];
}

@implementation LiroController
- (void)userContentController:(WKUserContentController *)ucc didReceiveScriptMessage:(WKScriptMessage *)message {
	goWindowMessage(self.wid, (char *)liro_json(message.body).UTF8String);
}
- (void)webView:(WKWebView *)webView
    decidePolicyForNavigationAction:(WKNavigationAction *)action
                    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	if (action.targetFrame == nil) {
		decisionHandler(WKNavigationActionPolicyCancel); // a second window: never
		return;
	}
	NSString *url = action.request.URL.absoluteString ?: @"";
	decisionHandler(goAllowNavigation(self.wid, (char *)url.UTF8String) ? WKNavigationActionPolicyAllow
	                                                                    : WKNavigationActionPolicyCancel);
}
- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
	goWindowLoaded(self.wid, NULL);
}
- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
	goWindowLoaded(self.wid, (char *)error.localizedDescription.UTF8String);
}
- (void)webView:(WKWebView *)webView
    didFailProvisionalNavigation:(WKNavigation *)navigation
                       withError:(NSError *)error {
	goWindowLoaded(self.wid, (char *)error.localizedDescription.UTF8String);
}
- (WKWebView *)webView:(WKWebView *)webView
    createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
               forNavigationAction:(WKNavigationAction *)navigationAction
                    windowFeatures:(WKWindowFeatures *)windowFeatures {
	return nil;
}
- (void)windowWillClose:(NSNotification *)notification {
	if (self.closingFromGo) {
		return;
	}
	uintptr_t wid = self.wid;
	LiroController *me = self;
	// Forgotten after this callback returns, not during it: the
	// dictionary holds the last reference to this delegate.
	dispatch_async(dispatch_get_main_queue(), ^{ liro_forget(me); });
	goWindowClosed(wid);
}
@end

uintptr_t liro_window_new(uintptr_t wid, const char *title, int width, int height, int onTop, uintptr_t owner,
                          int acceptsFiles, const char *url) {
	if (liroWindows == nil) {
		liroWindows = [NSMutableDictionary dictionary];
	}
	WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
	LiroSchemeHandler *scheme = [LiroSchemeHandler new];
	scheme.wid = wid;
	[cfg setURLSchemeHandler:scheme forURLScheme:@"liro"];
	LiroController *c = [LiroController new];
	c.wid = wid;
	[cfg.userContentController addScriptMessageHandler:c name:@"liro"];

	NSRect r = NSMakeRect(0, 0, width, height);
	NSWindow *win = [[NSWindow alloc]
	    initWithContentRect:r
	              styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable)
	                backing:NSBackingStoreBuffered
	                  defer:NO];
	win.releasedWhenClosed = NO;
	win.title = [NSString stringWithUTF8String:title];
	LiroWebView *view = [[LiroWebView alloc] initWithFrame:r configuration:cfg];
	view.wid = wid;
	view.acceptsFiles = acceptsFiles != 0;
	view.navigationDelegate = c;
	view.UIDelegate = c;
	win.contentView = view;
	win.delegate = c;
	if (onTop) {
		win.level = NSFloatingWindowLevel;
	}
	[win center];
	if (owner != 0) {
		[(__bridge NSWindow *)(void *)owner addChildWindow:win ordered:NSWindowAbove];
	}
	c.window = win;
	c.view = view;
	liroWindows[@(wid)] = c;
	[view loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:[NSString stringWithUTF8String:url]]]];
	[NSApp activateIgnoringOtherApps:YES];
	[win makeKeyAndOrderFront:nil];
	return (uintptr_t)(__bridge void *)win;
}

void liro_window_load(uintptr_t wid, const char *url) {
	LiroController *c = liroWindows[@(wid)];
	[c.view loadRequest:[NSURLRequest requestWithURL:[NSURL URLWithString:[NSString stringWithUTF8String:url]]]];
}

void liro_window_eval(uintptr_t wid, uintptr_t token, const char *script) {
	LiroController *c = liroWindows[@(wid)];
	if (c == nil) {
		goEvalDone(token, "null");
		return;
	}
	[c.view evaluateJavaScript:[NSString stringWithUTF8String:script]
	         completionHandler:^(id result, NSError *error) {
		           goEvalDone(token, (char *)(error ? @"null" : liro_json(result)).UTF8String);
	         }];
}

// The window's centre is kept, not re-centred on the screen: a window
// that jumps at every step of a flow has to be found again each time.
void liro_window_resize(uintptr_t wid, int width, int height) {
	NSWindow *win = liroWindows[@(wid)].window;
	NSRect f = win.frame;
	NSPoint centre = NSMakePoint(NSMidX(f), NSMidY(f));
	NSRect content = [win contentRectForFrameRect:f];
	content.size = NSMakeSize(width, height);
	NSRect nf = [win frameRectForContentRect:content];
	nf.origin = NSMakePoint(centre.x - nf.size.width / 2, centre.y - nf.size.height / 2);
	[win setFrame:nf display:YES animate:NO];
}

void liro_window_close(uintptr_t wid) {
	LiroController *c = liroWindows[@(wid)];
	if (c == nil) {
		return;
	}
	c.closingFromGo = YES;
	[c.view stopLoading];
	NSWindow *parent = c.window.parentWindow;
	if (parent != nil) {
		[parent removeChildWindow:c.window];
	}
	[c.window close];
	liro_forget(c);
}

long liro_window_number(uintptr_t wid) { return (long)liroWindows[@(wid)].window.windowNumber; }

void liro_alert(const char *title, const char *body, int informational) {
	NSAlert *a = [NSAlert new];
	a.messageText = [NSString stringWithUTF8String:title];
	a.informativeText = [NSString stringWithUTF8String:body];
	a.alertStyle = informational ? NSAlertStyleInformational : NSAlertStyleWarning;
	[NSApp activateIgnoringOtherApps:YES];
	[a runModal];
}

// liro_pin asks for a PIN in a modal alert with a secure field. It
// returns the number of bytes written to dst, -1 for a cancel, or -2 for
// a PIN longer than max (refused rather than cut short: a truncated PIN
// is a wrong PIN, and a wrong PIN costs an attempt).
//
// What macOS keeps of it is not this program's to control: the field's
// value is an NSString, which cannot be wiped, and the Linux dialog's
// locked page has no equivalent here. The field is emptied when the
// dialog ends; that is all that can be done from outside AppKit.
int liro_pin(const char *title, const char *heading, const char *subject, const char *label, const char *hint,
             const char *ok, const char *cancel, char *dst, int max) {
	NSAlert *a = [NSAlert new];
	a.messageText = [NSString stringWithUTF8String:heading];
	a.informativeText = [NSString stringWithFormat:@"%s\n\n%s", subject, hint];
	[a addButtonWithTitle:[NSString stringWithUTF8String:ok]];
	[a addButtonWithTitle:[NSString stringWithUTF8String:cancel]];
	a.window.title = [NSString stringWithUTF8String:title];
	NSSecureTextField *field = [[NSSecureTextField alloc] initWithFrame:NSMakeRect(0, 0, 260, 24)];
	field.placeholderString = [NSString stringWithUTF8String:label];
	a.accessoryView = field;
	[a layout];
	[a.window makeFirstResponder:field];
	[NSApp activateIgnoringOtherApps:YES];
	NSModalResponse r = [a runModal];
	int n = -1;
	if (r == NSAlertFirstButtonReturn) {
		const char *s = field.stringValue.UTF8String;
		size_t len = s ? strlen(s) : 0;
		if ((int)len > max) {
			n = -2;
		} else {
			memcpy(dst, s, len);
			n = (int)len;
		}
	}
	field.stringValue = @"";
	return n;
}

// liro_open_panel returns the chosen paths joined by newlines, malloc'd,
// or NULL for a cancel.
char *liro_open_panel(const char *title, int directories, int multiple, const char *initial) {
	NSOpenPanel *p = [NSOpenPanel openPanel];
	p.message = [NSString stringWithUTF8String:title];
	p.canChooseDirectories = directories != 0;
	p.canChooseFiles = directories == 0;
	p.canCreateDirectories = directories != 0;
	p.allowsMultipleSelection = multiple != 0;
	if (initial != NULL && initial[0] != 0) {
		p.directoryURL = [NSURL fileURLWithPath:[NSString stringWithUTF8String:initial]];
	}
	[NSApp activateIgnoringOtherApps:YES];
	if ([p runModal] != NSModalResponseOK) {
		return NULL;
	}
	NSMutableArray *paths = [NSMutableArray array];
	for (NSURL *u in p.URLs) {
		if (u.path) [paths addObject:u.path];
	}
	return strdup([[paths componentsJoinedByString:@"\n"] UTF8String]);
}

static NSString *liro_webkit_version(void) {
	return [[NSBundle bundleForClass:[WKWebView class]] objectForInfoDictionaryKey:@"CFBundleShortVersionString"] ?: @"";
}

char *liro_webkit_version_c(void) { return strdup(liro_webkit_version().UTF8String); }

// The status-bar item (the tray). Its menu is rebuilt each time it is
// opened, from the labels Go supplies then, so a language changed in
// Settings is the menu's language at once (the tray's rule on the other
// two platforms).
@interface LiroTray : NSObject <NSMenuDelegate>
@property(nonatomic, strong) NSStatusItem *item;
@end

@implementation LiroTray
- (void)act:(NSMenuItem *)sender {
	goTrayAction((int)sender.tag);
}
- (void)menuNeedsUpdate:(NSMenu *)menu {
	[menu removeAllItems];
	char *raw = goTrayLabels();
	NSArray<NSString *> *labels = [[NSString stringWithUTF8String:raw] componentsSeparatedByString:@"\n"];
	free(raw);
	for (NSUInteger i = 0; i < labels.count; i++) {
		if (i == labels.count - 1) {
			[menu addItem:[NSMenuItem separatorItem]];
		}
		NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:labels[i] action:@selector(act:) keyEquivalent:@""];
		it.target = self;
		it.tag = (NSInteger)i;
		[menu addItem:it];
	}
}
@end

static LiroTray *liroTray;

void liro_tray_new(const void *png, size_t pngLen, const char *tooltip) {
	liroTray = [LiroTray new];
	liroTray.item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
	NSImage *img = [[NSImage alloc] initWithData:[NSData dataWithBytes:png length:pngLen]];
	img.size = NSMakeSize(18, 18);
	liroTray.item.button.image = img;
	liroTray.item.button.toolTip = [NSString stringWithUTF8String:tooltip];
	NSMenu *menu = [NSMenu new];
	menu.delegate = liroTray;
	liroTray.item.menu = menu;
}

void liro_tray_close(void) {
	if (liroTray != nil) {
		[[NSStatusBar systemStatusBar] removeStatusItem:liroTray.item];
		liroTray = nil;
	}
}
*/
import "C"

import (
	"errors"
	"runtime"
	"runtime/cgo"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

// AppKit belongs to the process's main thread, and Go runs main.main on
// it only if the main goroutine is kept there from the start: an init
// that locks the thread is the documented way (runtime.LockOSThread).
func init() { runtime.LockOSThread() }

// ErrNoMainLoop is NewWindow, a dialog or a tray asked for in a process
// that never called RunMain — a test binary without its TestMain, or a
// command that has no business opening windows. It is an error rather
// than a wait, because nothing would ever run the work.
var ErrNoMainLoop = errors.New("ui: the macOS main loop is not running (ui.RunMain was not called)")

var mainLoopRunning atomic.Bool

// runMain runs f on a goroutine and AppKit's loop on this, the main,
// thread until f returns.
func runMain(f func() int) int {
	var code int
	done := make(chan struct{})
	go func() {
		defer close(done)
		code = f()
		mainLoopRunning.Store(false)
		C.liro_app_stop()
	}()
	mainLoopRunning.Store(true)
	C.liro_app_run()
	<-done
	return code
}

type uiThread struct{}

var theUIThread uiThread

func (uiThread) start() error {
	if !mainLoopRunning.Load() {
		return ErrNoMainLoop
	}
	return nil
}

// do runs f on the main thread and waits for it.
func (t uiThread) do(f func()) error {
	if C.liro_is_main() != 0 {
		f()
		return nil
	}
	if err := t.start(); err != nil {
		return err
	}
	done := make(chan struct{})
	h := cgo.NewHandle(func() {
		defer close(done)
		f()
	})
	C.liro_dispatch(C.uintptr_t(h))
	<-done
	return nil
}

func runOnMainHandle(h uintptr) {
	handle := cgo.Handle(h)
	f := handle.Value().(func())
	handle.Delete()
	f()
}

// evals is Eval's pending results, keyed by a token: cgo forbids handing
// C a Go pointer to keep.
var evals = struct {
	sync.Mutex
	next uintptr
	m    map[uintptr]chan string
}{m: map[uintptr]chan string{}}

func evalDone(token uintptr, json string) {
	evals.Lock()
	ch := evals.m[token]
	delete(evals.m, token)
	evals.Unlock()
	if ch != nil {
		ch <- json
	}
}

func nativeEval(wid uintptr, script string) (string, error) {
	if C.liro_is_main() != 0 {
		return "", errors.New("ui: Eval on the main thread would wait for itself")
	}
	ch := make(chan string, 1)
	evals.Lock()
	evals.next++
	token := evals.next
	evals.m[token] = ch
	evals.Unlock()
	cs := C.CString(script)
	defer C.free(unsafe.Pointer(cs))
	if err := theUIThread.do(func() { C.liro_window_eval(C.uintptr_t(wid), C.uintptr_t(token), cs) }); err != nil {
		return "", err
	}
	return <-ch, nil
}

func nativeWindowNew(wid uintptr, title string, width, height int, onTop bool, owner uintptr, acceptsFiles bool, url string) uintptr {
	ct, cu := C.CString(title), C.CString(url)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cu))
	return uintptr(C.liro_window_new(C.uintptr_t(wid), ct, C.int(width), C.int(height), cBool(onTop),
		C.uintptr_t(owner), cBool(acceptsFiles), cu))
}

func nativeWindowLoad(wid uintptr, url string) {
	cu := C.CString(url)
	defer C.free(unsafe.Pointer(cu))
	C.liro_window_load(C.uintptr_t(wid), cu)
}

func nativeWindowResize(wid uintptr, width, height int) {
	C.liro_window_resize(C.uintptr_t(wid), C.int(width), C.int(height))
}

func nativeWindowClose(wid uintptr) { C.liro_window_close(C.uintptr_t(wid)) }

func nativeWindowNumber(wid uintptr) int { return int(C.liro_window_number(C.uintptr_t(wid))) }

func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func detectRuntime() (bool, string, error) {
	var v string
	err := theUIThread.do(func() {
		cv := C.liro_webkit_version_c()
		v = C.GoString(cv)
		C.free(unsafe.Pointer(cv))
	})
	if err != nil {
		return false, "", err
	}
	return true, v, nil
}

func showNativeMessage(title, body string, informational bool) {
	ct, cb := C.CString(title), C.CString(body)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cb))
	_ = theUIThread.do(func() { C.liro_alert(ct, cb, cBool(informational)) })
}

func openPanel(title string, directories, multiple bool, initial string) ([]string, bool, error) {
	ct, ci := C.CString(title), C.CString(initial)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(ci))
	var raw *C.char
	if err := theUIThread.do(func() { raw = C.liro_open_panel(ct, cBool(directories), cBool(multiple), ci) }); err != nil {
		return nil, false, err
	}
	if raw == nil {
		return nil, false, nil
	}
	s := C.GoString(raw)
	C.free(unsafe.Pointer(raw))
	return strings.Split(s, "\n"), true, nil
}

// pickFiles is the open panel. **No PDF filter**, which F6 §1 asks for:
// NSOpenPanel's type filter is all-or-nothing without an accessory view
// of its own, and a person choosing a file deliberately is not someone
// to filter away — the signing step names a file that is not a PDF.
func pickFiles(owner uintptr, title, _, _ string) ([]string, bool, error) {
	_ = owner
	return openPanel(title, false, true, "")
}

func pickFolder(owner uintptr, title, initial string) (string, bool, error) {
	_ = owner
	paths, ok, err := openPanel(title, true, false, initial)
	if err != nil || !ok || len(paths) == 0 {
		return "", ok, err
	}
	return paths[0], true, nil
}

// CollectPIN asks for a PIN in a modal alert (see liro_pin for what
// macOS keeps of it). owner is not used: the alert is application-modal.
func CollectPIN(owner uintptr, prompt PINPrompt, maxLen int, dst []byte) (n int, ok bool, err error) {
	_ = owner
	if maxLen > len(dst) {
		maxLen = len(dst)
	}
	if maxLen <= 0 {
		return 0, false, errors.New("ui: no room for a PIN")
	}
	strs := []string{prompt.Title, prompt.Heading, prompt.Subject, prompt.Label, prompt.Hint, prompt.OK, prompt.Cancel}
	cs := make([]*C.char, len(strs))
	for i, s := range strs {
		cs[i] = C.CString(s)
	}
	defer func() {
		for _, c := range cs {
			C.free(unsafe.Pointer(c))
		}
	}()
	buf := (*C.char)(C.malloc(C.size_t(maxLen)))
	defer func() {
		C.memset(unsafe.Pointer(buf), 0, C.size_t(maxLen))
		C.free(unsafe.Pointer(buf))
	}()
	var r C.int
	if err := theUIThread.do(func() {
		r = C.liro_pin(cs[0], cs[1], cs[2], cs[3], cs[4], cs[5], cs[6], buf, C.int(maxLen))
	}); err != nil {
		return 0, false, err
	}
	switch {
	case r == -1:
		return 0, false, nil
	case r < 0:
		return 0, false, ErrPINTooLong
	}
	copy(dst, unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(r)))
	return int(r), true, nil
}

func nativeTrayNew(png []byte, tooltip string) error {
	ct := C.CString(tooltip)
	defer C.free(unsafe.Pointer(ct))
	cp := C.CBytes(png)
	defer C.free(cp)
	return theUIThread.do(func() { C.liro_tray_new(cp, C.size_t(len(png)), ct) })
}

func nativeTrayClose() error { return theUIThread.do(func() { C.liro_tray_close() }) }
