package handlers_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/epistemer/genx/genxtest"
	"github.com/epistemer/handlers/internal/routesv1"
)

var updateGoldens = flag.Bool("update", false, "update generator golden files")

func TestGeneratorsGolden(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "generators", "*.routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(inputs)
	if len(inputs) == 0 {
		t.Fatal("no testdata/generators/*.routes.json files found")
	}

	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/epistemer/i18n").Output()
	if err != nil {
		t.Fatalf("go list i18n dir: %v", err)
	}
	i18nDir := strings.TrimSpace(string(out))

	for _, inPath := range inputs {
		name := strings.TrimSuffix(filepath.Base(inPath), ".routes.json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatal(err)
			}
			var decoded any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			if err := routesv1.Check(decoded); err != nil {
				t.Fatalf("routesv1.Check(%s): %v", inPath, err)
			}

			in := genxtest.Input{Path: inPath}
			routesFiles := genxtest.Render(t, ".", "routes", in)
			labelsFiles := genxtest.Render(t, ".", "labels", in)
			permFiles := genxtest.Render(t, ".", "permissions", in)

			// Second render must produce the exact same bytes.
			routesFiles2 := genxtest.Render(t, ".", "routes", in)
			labelsFiles2 := genxtest.Render(t, ".", "labels", in)
			permFiles2 := genxtest.Render(t, ".", "permissions", in)
			assertMapsEqual(t, "routes second render", routesFiles, routesFiles2)
			assertMapsEqual(t, "labels second render", labelsFiles, labelsFiles2)
			assertMapsEqual(t, "permissions second render", permFiles, permFiles2)

			if len(routesFiles) != 1 {
				t.Fatalf("expected 1 routes file, got %v", mapKeys(routesFiles))
			}

			if len(labelsFiles) != 1 {
				t.Fatalf("expected 1 labels file, got %v", mapKeys(labelsFiles))
			}

			switch name {
			case "empty", "strings_only":
				if len(permFiles) != 0 {
					t.Fatalf("expected no permissions file, got %v", mapKeys(permFiles))
				}
			default:
				if len(permFiles) != 1 {
					t.Fatalf("expected 1 permissions file, got %v", mapKeys(permFiles))
				}
			}

			// Every labels output must pass i18n's labels generator without error.
			tmpDir := t.TempDir()
			var i18nInputs []genxtest.Input
			for rel, content := range labelsFiles {
				dst := filepath.Join(tmpDir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(dst, content, 0o644); err != nil {
					t.Fatal(err)
				}
				i18nInputs = append(i18nInputs, genxtest.Input{Path: dst})
			}
			_ = genxtest.Render(t, i18nDir, "labels", i18nInputs...)

			got := make(map[string][]byte, len(routesFiles)+len(labelsFiles)+len(permFiles))
			for k, v := range routesFiles {
				got[k] = v
			}
			for k, v := range labelsFiles {
				got[k] = v
			}
			for k, v := range permFiles {
				got[k] = v
			}

			goldenDir := filepath.Join("testdata", "generators", name)
			if *updateGoldens {
				if err := os.RemoveAll(goldenDir); err != nil {
					t.Fatal(err)
				}
				for k, v := range got {
					p := filepath.Join(goldenDir, filepath.FromSlash(k))
					if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, v, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				return
			}

			want := readGoldenDir(t, goldenDir)
			assertMapsEqual(t, "golden", want, got)
		})
	}
}

func readGoldenDir(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertMapsEqual(t *testing.T, label string, want, got map[string][]byte) {
	t.Helper()
	wantKeys := mapKeys(want)
	gotKeys := mapKeys(got)
	if !slices.Equal(wantKeys, gotKeys) {
		t.Fatalf("%s keys mismatch:\nwant: %v\ngot:  %v", label, wantKeys, gotKeys)
	}
	for _, k := range wantKeys {
		if !bytes.Equal(want[k], got[k]) {
			t.Fatalf("%s mismatch for %s:\ngot:\n%s\nwant:\n%s", label, k, got[k], want[k])
		}
	}
}

func mapKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

type fatalRecorder struct {
	testing.TB
	msg string
}

func (r *fatalRecorder) Helper() {}

func (r *fatalRecorder) Fatalf(format string, args ...any) {
	r.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func (r *fatalRecorder) Fatal(args ...any) {
	r.msg = fmt.Sprint(args...)
	runtime.Goexit()
}

func renderFails(t *testing.T, fn func(testing.TB)) string {
	t.Helper()
	rec := &fatalRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(rec)
	}()
	<-done
	return rec.msg
}

func TestPermissionsTwoFilesError(t *testing.T) {
	tmpDir := t.TempDir()
	doc := func(dir, pkg string) string {
		return fmt.Sprintf(`{
  "domain": "routes/v1",
  "producer": "handlers",
  "source": %q,
  "package": {"name": %q, "dir": %q},
  "surface": "html",
  "prefix": "/%s",
  "routes": [
    {
      "key": "orders.manage",
      "method": "GET",
      "pattern": "/",
      "name": "Orders",
      "var": "L10nOrders",
      "text": "Manage orders",
      "auth": "permission",
      "permission": "orders.manage",
      "view": false,
      "search": false,
      "params": []
    }
  ],
  "permissions": [
    {
      "key": "orders.manage",
      "var": "PermOrdersManage",
      "text": "Manage orders",
      "level": "verified"
    }
  ]
}
`, dir, pkg, dir, pkg)
	}

	aPath := filepath.Join(tmpDir, "a", "zz_aroutes.routes.json")
	bPath := filepath.Join(tmpDir, "b", "zz_broutes.routes.json")
	for _, f := range []struct {
		path string
		body string
	}{
		{aPath, doc("internal/a", "aroutes")},
		{bPath, doc("internal/b", "broutes")},
	} {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f.path, []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Pass bPath before aPath to confirm output reports dirs in path order.
	got := renderFails(t, func(tb testing.TB) {
		genxtest.Render(tb, ".", "permissions",
			genxtest.Input{Path: bPath},
			genxtest.Input{Path: aPath})
	})
	want := `permission "orders.manage" is in internal/a and internal/b; a permission belongs to one routes package`
	if !strings.Contains(got, want) {
		t.Fatalf("error = %q, want substring %q", got, want)
	}
}

func TestPermissionsUnknownLevelError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "zz_bad.routes.json")
	body := `{
  "domain": "routes/v1",
  "producer": "handlers",
  "source": "internal/bad",
  "package": {"name": "badroutes", "dir": "internal/bad"},
  "surface": "html",
  "prefix": "/bad",
  "routes": [],
  "permissions": [
    {"key": "x.y", "var": "PermXY", "text": "XY", "level": "admin"}
  ]
}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := renderFails(t, func(tb testing.TB) {
		genxtest.Render(tb, ".", "permissions", genxtest.Input{Path: p})
	})
	if !strings.Contains(got, "permissions: unknown level admin") {
		t.Fatalf("error = %q, want unknown level", got)
	}
}
