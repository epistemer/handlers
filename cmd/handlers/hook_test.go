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

func TestPluginAndExtensionHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX sh hook scripts are tested on Unix")
	}

	bin := buildHandlers(t)
	binDir := filepath.Dir(bin)

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	testdataTypes, err := filepath.Abs(filepath.Join("..", "internal", "directive", "testdata", "types"))
	if err != nil {
		t.Fatal(err)
	}

	errFile := filepath.Join(testdataTypes, "rest_non_string", "handlers.go")
	cleanFile := filepath.Join(testdataTypes, "clean", "handlers.go")

	runHookScript := func(t *testing.T, script, dir, pathEnv, stdin string) (int, string, string) {
		t.Helper()
		cmd := exec.Command(script)
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(stdin)
		cmd.Env = append(os.Environ(), "PATH="+pathEnv)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				t.Fatalf("exec %s: %v", script, err)
			}
		}
		return code, stdout.String(), stderr.String()
	}

	pathWithHandlers := binDir + string(os.PathListSeparator) + os.Getenv("PATH")

	t.Run("claude_plugin_PostToolUse", func(t *testing.T) {
		hookScript := filepath.Join(repoRoot, "dev", "claude-plugin", "bin", "handlers-hook")

		faultPayload, _ := json.Marshal(map[string]any{
			"hook_event_name": "PostToolUse",
			"tool_name":       "Edit",
			"tool_input":      map[string]any{"file_path": errFile},
		})
		code, stdout, stderr := runHookScript(t, hookScript, testdataTypes, pathWithHandlers, string(faultPayload))
		if code != 0 || stderr != "" {
			t.Fatalf("fault = (%d, %q, %q)", code, stdout, stderr)
		}
		var resp struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
			t.Fatalf("unmarshal %q: %v", stdout, err)
		}
		if resp.Decision != "block" {
			t.Errorf("decision = %q, want block", resp.Decision)
		}
		if !strings.Contains(resp.Reason, "wildcard needs a string type") {
			t.Errorf("reason = %q", resp.Reason)
		}

		cleanPayload, _ := json.Marshal(map[string]any{
			"hook_event_name": "PostToolUse",
			"tool_name":       "Write",
			"tool_input":      map[string]any{"file_path": cleanFile},
		})
		code, stdout, stderr = runHookScript(t, hookScript, testdataTypes, pathWithHandlers, string(cleanPayload))
		if code != 0 || stdout != "" || stderr != "" {
			t.Errorf("clean = (%d, %q, %q), want (0, \"\", \"\")", code, stdout, stderr)
		}

		// Without handlers on PATH and without go tool handlers in module -> exit 0 silently.
		emptyPath := "/bin" + string(os.PathListSeparator) + "/usr/bin"
		code, stdout, stderr = runHookScript(t, hookScript, t.TempDir(), emptyPath, string(faultPayload))
		if code != 0 || stdout != "" || stderr != "" {
			t.Errorf("no handlers = (%d, %q, %q), want (0, \"\", \"\")", code, stdout, stderr)
		}
	})

	t.Run("gemini_extension_AfterTool", func(t *testing.T) {
		hookScript := filepath.Join(repoRoot, "dev", "gemini-extension", "bin", "handlers-hook")

		faultPayload, _ := json.Marshal(map[string]any{
			"session_id":      "sess-1",
			"cwd":             testdataTypes,
			"hook_event_name": "AfterTool",
			"timestamp":       "2026-11-18T00:00:00Z",
			"tool_name":       "write_file",
			"tool_input":      map[string]any{"file_path": errFile, "content": "..."},
			"tool_response":   map[string]any{"llmContent": "wrote file", "returnDisplay": "wrote file"},
		})
		code, stdout, stderr := runHookScript(t, hookScript, testdataTypes, pathWithHandlers, string(faultPayload))
		if code != 0 || stderr != "" {
			t.Fatalf("fault = (%d, %q, %q)", code, stdout, stderr)
		}
		var resp struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
			t.Fatalf("unmarshal %q: %v", stdout, err)
		}
		if resp.Decision != "block" {
			t.Errorf("decision = %q, want block", resp.Decision)
		}
		if !strings.Contains(resp.Reason, "wildcard needs a string type") {
			t.Errorf("reason = %q", resp.Reason)
		}

		cleanPayload, _ := json.Marshal(map[string]any{
			"session_id":      "sess-1",
			"cwd":             testdataTypes,
			"hook_event_name": "AfterTool",
			"timestamp":       "2026-11-18T00:00:00Z",
			"tool_name":       "replace",
			"tool_input":      map[string]any{"file_path": cleanFile},
			"tool_response":   map[string]any{"llmContent": "edited file", "returnDisplay": "edited file"},
		})
		code, stdout, stderr = runHookScript(t, hookScript, testdataTypes, pathWithHandlers, string(cleanPayload))
		if code != 0 || stdout != "" || stderr != "" {
			t.Errorf("clean = (%d, %q, %q), want (0, \"\", \"\")", code, stdout, stderr)
		}
	})
}
