package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// listPkg mirrors the fields of `go list -json` output that this checker
// needs. Deps is the full transitive dependency closure of the package,
// which is what makes checkPackage catch indirect violations, not only
// direct imports.
type listPkg struct {
	ImportPath string
	Deps       []string
}

func main() {
	graph, err := buildGraph()
	if err != nil {
		fmt.Fprintln(os.Stderr, "checkdeps:", err)
		os.Exit(1)
	}

	violations := checkGraph(graph)
	if len(violations) > 0 {
		fmt.Fprint(os.Stderr, formatViolations(violations))
		os.Exit(1)
	}

	fmt.Printf("checkdeps: OK (%d packages checked)\n", len(graph))
}

// buildGraph runs `go list -json ./...` and returns, for every package in
// this module, its module-relative import path mapped to the
// module-relative import paths of every package (in this module) it
// depends on, transitively.
func buildGraph() (map[string][]string, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	graph := make(map[string][]string)
	dec := json.NewDecoder(stdout)
	for {
		var pkg listPkg
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decoding go list output: %w", err)
		}

		rel, ok := relative(pkg.ImportPath)
		if !ok {
			continue
		}

		var deps []string
		for _, d := range pkg.Deps {
			if relDep, ok := relative(d); ok {
				deps = append(deps, relDep)
			}
		}
		graph[rel] = deps
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	return graph, nil
}
