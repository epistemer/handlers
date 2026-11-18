package handlers_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
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

			// Second render must produce the exact same bytes.
			routesFiles2 := genxtest.Render(t, ".", "routes", in)
			labelsFiles2 := genxtest.Render(t, ".", "labels", in)
			assertMapsEqual(t, "routes second render", routesFiles, routesFiles2)
			assertMapsEqual(t, "labels second render", labelsFiles, labelsFiles2)

			if name == "api" {
				if len(routesFiles) != 0 {
					t.Fatalf("json surface produced routes files: %v", mapKeys(routesFiles))
				}
			} else if len(routesFiles) != 1 {
				t.Fatalf("expected 1 routes file, got %v", mapKeys(routesFiles))
			}

			if len(labelsFiles) != 1 {
				t.Fatalf("expected 1 labels file, got %v", mapKeys(labelsFiles))
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

			got := make(map[string][]byte, len(routesFiles)+len(labelsFiles))
			for k, v := range routesFiles {
				got[k] = v
			}
			for k, v := range labelsFiles {
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
