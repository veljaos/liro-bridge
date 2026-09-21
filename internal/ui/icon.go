package ui

import _ "embed"

// trayIconICO is scripts/genicon's output: this program's mark at eight
// sizes, every one of them a PNG inside a Windows .ico container.
//
// **One icon, embedded once, for every platform that has to draw one.**
// It lived beside the Windows loader until this platform needed the same
// bytes, and a second embed of the same file would have been the start
// of two icons that drift (D-285 and D-286 are what it cost to get one
// right). Windows hands the container to the shell, which picks a size;
// linux decodes the sizes itself and hands the pixels to a panel
// (trayicon_linux.go), because StatusNotifierItem has no notion of a
// file.

//go:embed assets/icon.ico
var trayIconICO []byte
