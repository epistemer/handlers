package api

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	if !bytes.Equal(got, want) {
		t.Fatal("api/zz_apigen.go is stale; run go generate ./api")
	}
}
