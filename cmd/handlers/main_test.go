package main_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildHandlers(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "handlers")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func runBin(t *testing.T, bin, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("exec %v: %v", args, err)
		}
	}
	return code, stdout.String(), stderr.String()
}

func TestHandlersSubcommands(t *testing.T) {
	bin := buildHandlers(t)

	code, _, _ := runBin(t, bin, "", "")
	if code != 2 {
		t.Errorf("no args exit code = %d, want 2", code)
	}

	for _, sub := range []string{"gen", "openapi", "lsp"} {
		code, _, stderr := runBin(t, bin, "", "", sub)
		if code != 2 {
			t.Errorf("%s exit code = %d, want 2", sub, code)
		}
		want := "handlers: " + sub + " is not implemented yet"
		if !strings.Contains(stderr, want) {
			t.Errorf("%s stderr = %q, want %q", sub, stderr, want)
		}
	}
}

func TestHandlersVetEndToEnd(t *testing.T) {
	bin := buildHandlers(t)
	testdataRoot, err := filepath.Abs(filepath.Join("..", "internal", "directive", "testdata"))
	if err != nil {
		t.Fatal(err)
	}

	// Clean case (using types/clean which has a valid go.mod)
	cleanDir := filepath.Join(testdataRoot, "types", "clean")
	code, stdout, stderr := runBin(t, bin, cleanDir, "", "vet", "-baseline", "HEAD", ".")
	if code != 0 || stdout != "" || stderr != "" {
		t.Errorf("clean vet = (%d, %q, %q), want (0, \"\", \"\")", code, stdout, stderr)
	}

	// Warning-only case: exit 0 without -strict, exit 1 with -strict
	warnDir := filepath.Join(testdataRoot, "warn_missing_desc")
	// Copy to temp module so `go list` works cleanly
	warnMod := setupTempMod(t, warnDir)
	code, stdout, _ = runBin(t, bin, warnMod, "", "vet", ".")
	if code != 0 {
		t.Errorf("warn vet without -strict exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "(warning)") {
		t.Errorf("warn vet stdout = %q, want warning", stdout)
	}
	code, _, _ = runBin(t, bin, warnMod, "", "vet", "-strict", ".")
	if code != 1 {
		t.Errorf("warn vet with -strict exit = %d, want 1", code)
	}

	// Error case: exit 1
	errDir := filepath.Join(testdataRoot, "types", "rest_non_string")
	code, stdout, _ = runBin(t, bin, errDir, "", "vet", ".")
	if code != 1 {
		t.Errorf("error vet exit = %d, want 1", code)
	}
	if !strings.Contains(stdout, "wildcard needs a string type") {
		t.Errorf("error vet stdout = %q", stdout)
	}
}

func TestHandlersVetHook(t *testing.T) {
	bin := buildHandlers(t)
	testdataRoot, err := filepath.Abs(filepath.Join("..", "internal", "directive", "testdata"))
	if err != nil {
		t.Fatal(err)
	}

	// Error case inside cwd -> block JSON, exit 0
	typesRoot := filepath.Join(testdataRoot, "types")
	errFile := filepath.Join(typesRoot, "rest_non_string", "handlers.go")
	payload, _ := json.Marshal(map[string]any{
		"tool_input": map[string]any{"file_path": errFile},
	})
	code, stdout, stderr := runBin(t, bin, typesRoot, string(payload), "vet", "-hook")
	if code != 0 || stderr != "" {
		t.Fatalf("hook error case = (%d, %q, %q)", code, stdout, stderr)
	}
	var resp struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("unmarshal hook output %q: %v", stdout, err)
	}
	if resp.Decision != "block" {
		t.Errorf("decision = %q, want block", resp.Decision)
	}
	if !strings.HasPrefix(resp.Reason, "handlers vet found faults in the route directives:\n") {
		t.Errorf("reason = %q", resp.Reason)
	}
	if !strings.Contains(resp.Reason, "wildcard needs a string type") {
		t.Errorf("reason missing diagnostic: %q", resp.Reason)
	}

	// Clean case inside cwd -> nothing, exit 0
	cleanFile := filepath.Join(typesRoot, "clean", "handlers.go")
	payload, _ = json.Marshal(map[string]any{
		"tool_input": map[string]any{"file_path": cleanFile},
	})
	code, stdout, stderr = runBin(t, bin, typesRoot, string(payload), "vet", "-hook")
	if code != 0 || stdout != "" || stderr != "" {
		t.Errorf("hook clean case = (%d, %q, %q), want (0, \"\", \"\")", code, stdout, stderr)
	}

	// File outside cwd -> nothing, exit 0
	outsideCwd := t.TempDir()
	code, stdout, stderr = runBin(t, bin, outsideCwd, string(payload), "vet", "-hook")
	if code != 0 || stdout != "" || stderr != "" {
		t.Errorf("hook outside cwd = (%d, %q, %q), want (0, \"\", \"\")", code, stdout, stderr)
	}

	// Invalid JSON -> nothing, exit 0
	code, stdout, stderr = runBin(t, bin, typesRoot, "{bad json", "vet", "-hook")
	if code != 0 || stdout != "" || stderr != "" {
		t.Errorf("hook invalid json = (%d, %q, %q), want (0, \"\", \"\")", code, stdout, stderr)
	}
}

func setupTempMod(t *testing.T, srcDir string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tempcase\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
