module github.com/veljaos/liro-bridge

go 1.26.5

require (
	github.com/godbus/dbus/v5 v5.2.2
	golang.org/x/sys v0.47.0
	software.sslmate.com/src/go-pkcs12 v0.7.3
)

require (
	// A pseudo-version because the module has no tags at all. It is
	// generated against WebKitGTK 2.42 while the floor runs 2.52.6, and
	// it is missing every async operation's *starting* half — 29 *Finish
	// methods out of 1273 functions and no GAsyncReadyCallback anywhere
	// — which is why internal/ui carries webkitjs_linux.{h,c,go}. D-330.
	github.com/diamondburned/gotk4-webkitgtk/pkg v0.0.0-20240108031600-dee1973cf440
	// Pinned, and the pin is load-bearing. v0.4.x names five GLib
	// functions Ubuntu 24.04's GLib 2.80 does not have, and its cgo line
	// declares no minimum version — so pkg-config succeeds, the build
	// proceeds, and it fails fifteen minutes later as unresolved C
	// references in generated code. `go get -u` takes v0.4.x. Do not
	// raise it without a machine on the floor platform to build on.
	// D-327.
	github.com/diamondburned/gotk4/pkg v0.3.1
	golang.org/x/crypto v0.11.0
	golang.org/x/image v0.45.0
)

require (
	github.com/KarpelesLab/weak v0.1.1 // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20231121144256-b99613f794b6 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
