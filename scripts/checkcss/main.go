package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := "internal/ui/assets"
	var violations []Violation

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".css") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(b), "\n")
		violations = append(violations, checkCSS(filepath.Base(path), lines)...)
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "checkcss:", err)
		os.Exit(1)
	}

	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Fprintf(os.Stderr, "HEX COLOUR LITERAL\n  %s:%d\n  %s\n\n", v.File, v.Line, strings.TrimSpace(v.Text))
		}
		os.Exit(1)
	}
	fmt.Println("checkcss: OK")
}
