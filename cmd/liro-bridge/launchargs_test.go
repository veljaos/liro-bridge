package main

import (
	"reflect"
	"testing"
)

func TestABundleLaunchedByTheSystemIsTheAgent(t *testing.T) {
	const bundle = "/Applications/Liro Bridge.app/Contents/MacOS/liro-bridge"
	for _, tc := range []struct {
		goos, exe  string
		args, want []string
	}{
		{"darwin", bundle, nil, []string{"tray"}},
		{"darwin", bundle, []string{"-psn_0_12345"}, []string{"tray"}},
		{"darwin", bundle, []string{"certs"}, []string{"certs"}},
		{"darwin", "/usr/local/bin/liro-bridge", nil, nil},
		{"linux", "/usr/bin/liro-bridge", nil, nil},
		{"windows", `C:\liro-bridge.exe`, []string{"-psn_0_1"}, []string{"-psn_0_1"}},
	} {
		if got := launchArgs(tc.goos, tc.exe, tc.args); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("launchArgs(%s, %q, %q) = %q, want %q", tc.goos, tc.exe, tc.args, got, tc.want)
		}
	}
}
