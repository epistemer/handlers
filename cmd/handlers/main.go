package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/build"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/epistemer/handlers/cmd/internal/directive"
	"github.com/epistemer/handlers/cmd/internal/lsp"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: handlers <command> [flags] [args]")
		return 2
	}

	sub := args[0]
	switch sub {
	case "vet":
		return runVet(args[1:], stdin, stdout, stderr)
	case "gen":
		return runGen(args[1:], stdout, stderr)
	case "openapi":
		return runOpenAPI(args[1:], stdout, stderr)
	case "lsp":
		return runLSP(args[1:], stdin, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "handlers: unknown command %q\n", sub)
		return 2
	}
}

func runLSP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lsp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: handlers lsp")
		return 2
	}
	return lsp.Run(stdin, stdout)
}

func runVet(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vet", flag.ContinueOnError)
	fs.SetOutput(stderr)
	strict := fs.Bool("strict", false, "treat warnings as errors")
	hook := fs.Bool("hook", false, "run as a Claude Code PostToolUse hook")
	baseline := fs.String("baseline", "", "git ref baseline for removal warnings")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *hook {
		runHook(stdin, stdout)
		return 0
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"."}
	}

	dirs, err := expandPackages(patterns)
	if err != nil {
		fmt.Fprintf(stderr, "handlers vet: %v\n", err)
		return 1
	}

	var allDiags []directive.Diagnostic
	var vetted []vettedPkg
	for _, dir := range dirs {
		routesPkg, extCounts := vetContext(dir)
		pkg, diags, err := analyzePackageDir(dir, "", routesPkg, extCounts)
		if err != nil {
			fmt.Fprintf(stderr, "handlers vet: %v\n", err)
			return 1
		}
		allDiags = append(allDiags, diags...)
		vetted = append(vetted, vettedPkg{
			dir: dir,
			pkg: pkg,
		})
	}

	var fallback string
	if len(dirs) > 0 {
		fallback = dirs[0]
	}
	if modRoot, err := findModuleRootFromCwdOr(fallback); err == nil {
		for i := range vetted {
			if rel, err := relSlash(modRoot, vetted[i].dir); err == nil {
				vetted[i].relDir = rel
			}
		}
		crossDiags, err := crossVet(modRoot, vetted, *baseline)
		if err != nil {
			fmt.Fprintf(stderr, "handlers vet: %v\n", err)
			return 1
		}
		allDiags = append(allDiags, crossDiags...)
	}

	sortDiags(allDiags)

	hasFail := false
	for _, d := range allDiags {
		fmt.Fprintln(stdout, d.String())
		if d.Severity == directive.Error || (*strict && d.Severity == directive.Warning) {
			hasFail = true
		}
	}

	if hasFail {
		return 1
	}
	return 0
}

type hookPayload struct {
	ToolInput struct {
		FilePath string `json:"file_path"`
	} `json:"tool_input"`
}

type hookBlockResponse struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func runHook(stdin io.Reader, stdout io.Writer) {
	var payload hookPayload
	if err := json.NewDecoder(stdin).Decode(&payload); err != nil {
		return
	}
	fp := payload.ToolInput.FilePath
	if fp == "" || !strings.HasSuffix(fp, ".go") {
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	cwdReal, err := resolvePath(cwd)
	if err != nil {
		return
	}
	fileReal, err := resolvePath(fp)
	if err != nil {
		return
	}

	rel, err := filepath.Rel(cwdReal, fileReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}

	pkgDir := filepath.Dir(fileReal)
	diags, err := checkPackageDir(pkgDir)
	if err != nil {
		return
	}

	var errLines []string
	for _, d := range diags {
		if d.Severity == directive.Error {
			errLines = append(errLines, d.String())
		}
	}
	if len(errLines) == 0 {
		return
	}

	resp := hookBlockResponse{
		Decision: "block",
		Reason:   "handlers vet found faults in the route directives:\n" + strings.Join(errLines, "\n"),
	}
	_ = json.NewEncoder(stdout).Encode(resp)
}

func resolvePath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(abs)
	if real, err := filepath.EvalSymlinks(clean); err == nil {
		return real, nil
	}
	return clean, nil
}

func expandPackages(patterns []string) ([]string, error) {
	args := append([]string{"list", "-f", "{{.Dir}}"}, patterns...)
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list: %s", strings.TrimSpace(string(out)))
	}
	var dirs []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			dirs = append(dirs, line)
		}
	}
	return dirs, nil
}

func checkPackageDir(dir string) ([]directive.Diagnostic, error) {
	routesPkg, extCounts := vetContext(dir)
	_, diags, err := analyzePackageDir(dir, "", routesPkg, extCounts)
	return diags, err
}

// vetContext finds, for a handler package directory checked without gen's
// flags, the name of its routes package (from the committed routes file
// whose source is dir) and the routes of other committed files that take
// its permissions, as gen computes them.
func vetContext(dir string) (string, map[string]int) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", nil
	}
	modRoot, ok := findGoModUp(abs)
	if !ok {
		return "", nil
	}
	sourceRel, err := relSlash(modRoot, abs)
	if err != nil {
		return "", nil
	}
	pkgName, outRel := committedRoutesFor(modRoot, sourceRel)
	return pkgName, countCommittedTakingRoutes(modRoot, sourceRel, outRel)
}

func analyzePackageDir(dir, out, routesPkg string, extCounts map[string]int) (*directive.Package, []directive.Diagnostic, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	files := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		matched, err := build.Default.MatchFile(dir, name)
		if err != nil || !matched {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		files[filepath.Join(dir, name)] = b
	}

	if len(files) == 0 {
		return nil, nil, nil
	}

	var opts []directive.SyntaxOption
	if routesPkg != "" {
		opts = append(opts, directive.WithRoutesPackage(routesPkg))
	}
	if extCounts != nil {
		opts = append(opts, directive.WithExternalPermCounts(extCounts))
	}
	pkg, diags := directive.Syntax(files, opts...)
	if len(pkg.Routes) > 0 {
		tDiags, err := directive.Types(dir, pkg, out)
		if err != nil {
			return nil, nil, err
		}
		diags = append(diags, tDiags...)
	}
	sortDiags(diags)
	return pkg, diags, nil
}

func sortDiags(diags []directive.Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		if a.Pos.Filename != b.Pos.Filename {
			return a.Pos.Filename < b.Pos.Filename
		}
		if a.Pos.Line != b.Pos.Line {
			return a.Pos.Line < b.Pos.Line
		}
		if a.Pos.Column != b.Pos.Column {
			return a.Pos.Column < b.Pos.Column
		}
		return a.Message < b.Message
	})
}
