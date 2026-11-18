package api

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOapiCodegenCurrent(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "zz_apigen.go")
	cfgPath := filepath.Join(t.TempDir(), "oapi-codegen.yaml")
	cfg := fmt.Sprintf(`package: api
output: %s
generate:
  std-http-server: true
  strict-server: true
  models: true
  embedded-spec: true
`, outPath)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "tool", "oapi-codegen", "-config", cfgPath, "openapi.yaml")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("oapi-codegen: %v\n%s", err, out)
	}

	want, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("zz_apigen.go")
	if err != nil {
		t.Fatal(err)
	}
	gotCode, gotSpec := splitSpec(t, got)
	wantCode, wantSpec := splitSpec(t, want)
	if !bytes.Equal(gotCode, wantCode) || !bytes.Equal(gotSpec, wantSpec) {
		t.Fatal("api/zz_apigen.go is stale; run go generate ./api")
	}
}

// splitSpec separates oapi-codegen's output into its code and its embedded
// spec, decompressed: the DEFLATE bytes differ between Go versions while the
// spec doesn't.
func splitSpec(t *testing.T, src []byte) (code, spec []byte) {
	t.Helper()
	const open = "var swaggerSpec = []string{"
	s := string(src)
	i := strings.Index(s, open)
	if i < 0 {
		t.Fatal("no embedded spec in oapi-codegen's output")
	}
	j := strings.Index(s[i:], "\n}")
	if j < 0 {
		t.Fatal("unterminated embedded spec")
	}
	block := s[i+len(open) : i+j]
	var b64 strings.Builder
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ",")
		if line == "" {
			continue
		}
		unq, err := strconv.Unquote(line)
		if err != nil {
			t.Fatalf("embedded spec line %q: %v", line, err)
		}
		b64.WriteString(unq)
	}
	zipped, err := base64.StdEncoding.DecodeString(b64.String())
	if err != nil {
		t.Fatal(err)
	}
	spec, err = io.ReadAll(flate.NewReader(bytes.NewReader(zipped)))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(s[:i] + s[i+j:]), spec
}
