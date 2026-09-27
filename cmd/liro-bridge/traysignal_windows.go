//go:build windows

package main

// quitOnTerminate does nothing on Windows, and returns a stop that does
// nothing.
//
// D20 (D-393) was measured on Linux, where a logout sends SIGTERM. What a
// logoff does to the Windows agent, and to its discovery file, has not been
// looked at, and F12 changes nothing on Windows that was not decided — so
// this stays as Windows has always behaved until someone measures it.
func quitOnTerminate(quit func()) (stop func()) {
	return func() {}
}
