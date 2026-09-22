//go:build !windows && !linux

package pkcs11

// crashIfAskedForTest does nothing here, and the crash sentinels are not
// declared here at all.
//
// They were, briefly, "so that TestMain and the tests that name them compile
// everywhere" — and nothing on this platform names them, because the tests that
// drive a deliberately dying child are Windows-only. Two constants that exist
// so that a file looks symmetrical are two constants the `unused` linter is
// right about, and the same smell as the info()/moduleInfo stubs D-295 deleted:
// a declaration whose stated purpose is to satisfy a build.
//
// Only the function is needed, because TestMain calls it unconditionally.
//
// # When this grows a body
//
// F12 and F13 will want the equivalent — raise(SIGABRT), or an abort() through
// the same seam — at the point where there is a dlopen binding for a module to
// crash inside. There is not one yet (module_other.go), so a test that killed a
// child here today would measure the parent against a death no module on this
// platform can currently produce, which is a measurement about nothing.
func crashIfAskedForTest(string) {}
