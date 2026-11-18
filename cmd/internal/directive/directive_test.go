package directive_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/epistemer/handlers/cmd/internal/directive"
)

func TestReadable(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"categories.profile", "Categories Profile"},
		{"orderID", "Order ID"},
		{"zp_user_role", "Zp User Role"},
		{"getHTTPResponse", "Get HTTP Response"},
		{"", ""},
	}
	for _, tc := range cases {
		got := directive.Readable(tc.in)
		if got != tc.want {
			t.Errorf("Readable(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSyntaxCases(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "types" {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			caseDir := filepath.Join("testdata", name)
			files := readGoFiles(t, caseDir)
			_, diags := directive.Syntax(files)
			assertDiagnostics(t, caseDir, diags)
		})
	}
}

func TestTypesCases(t *testing.T) {
	typesRoot := filepath.Join("testdata", "types")
	entries, err := os.ReadDir(typesRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "ext" {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			caseDir := filepath.Join(typesRoot, name)
			files := readGoFiles(t, caseDir)
			pkg, diags := directive.Syntax(files)
			tDiags, err := directive.Types(caseDir, pkg)
			if err != nil {
				t.Fatalf("Types(%s): %v", caseDir, err)
			}
			diags = append(diags, tDiags...)
			assertDiagnostics(t, caseDir, diags)
		})
	}
}

func TestSyntaxNeverPanicsOnTruncatedInput(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "clean", "handlers.go"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= len(src); i++ {
		files := map[string][]byte{
			"handlers.go": src[:i],
		}
		_, _ = directive.Syntax(files)
	}
}

func readGoFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = b
	}
	return files
}

func assertDiagnostics(t *testing.T, caseDir string, diags []directive.Diagnostic) {
	t.Helper()
	wantBytes, err := os.ReadFile(filepath.Join(caseDir, "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var wants []string
	for line := range strings.SplitSeq(strings.TrimRight(string(wantBytes), "\r\n"), "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" {
			wants = append(wants, line)
		}
	}

	absDir, _ := filepath.Abs(caseDir)
	var gots []string
	for _, d := range diags {
		s := d.String()
		if strings.HasPrefix(s, absDir+string(filepath.Separator)) {
			s = strings.TrimPrefix(s, absDir+string(filepath.Separator))
		} else if strings.HasPrefix(s, caseDir+string(filepath.Separator)) {
			s = strings.TrimPrefix(s, caseDir+string(filepath.Separator))
		}
		s = filepath.ToSlash(s)
		gots = append(gots, s)
	}

	if len(gots) != len(wants) {
		t.Fatalf("diagnostic count mismatch: got %d, want %d\ngot:\n%s\nwant:\n%s",
			len(gots), len(wants), strings.Join(gots, "\n"), strings.Join(wants, "\n"))
	}
	for i := range wants {
		if strings.HasSuffix(wants[i], "*") {
			prefix := strings.TrimSuffix(wants[i], "*")
			if !strings.HasPrefix(gots[i], prefix) {
				t.Errorf("diag[%d] = %q, want prefix %q", i, gots[i], prefix)
			}
		} else if gots[i] != wants[i] {
			t.Errorf("diag[%d] =\n  %q\nwant:\n  %q", i, gots[i], wants[i])
		}
	}
}

func TestTypesParamInfo(t *testing.T) {
	dir := filepath.Join("testdata", "types", "clean")
	b, err := os.ReadFile(filepath.Join(dir, "handlers.go"))
	if err != nil {
		t.Fatal(err)
	}
	pkg, diags := directive.Syntax(map[string][]byte{filepath.Join(dir, "handlers.go"): b})
	if len(diags) != 0 {
		t.Fatalf("unexpected syntax diags: %v", diags)
	}
	tDiags, err := directive.Types(dir, pkg)
	if err != nil {
		t.Fatal(err)
	}
	if len(tDiags) != 0 {
		t.Fatalf("unexpected type diags: %v", tDiags)
	}
	if len(pkg.Routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(pkg.Routes))
	}
	want := []struct {
		name, imp, pkgName, typeName, parse string
	}{
		{"id", "", "", "int64", "int64"},
		{"flag", "", "", "bool", "bool"},
		{"u", "", "", "uint32", "uint32"},
		{"cid", "toy/ext", "ext", "ext.CustomID", "text"},
		{"rest", "toy/ext", "ext", "ext.Slug", "string"},
	}
	got := pkg.Routes[0].Params
	if len(got) != len(want) {
		t.Fatalf("params = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		p := got[i]
		if p.Name != w.name || p.ImportPath != w.imp || p.PkgName != w.pkgName || p.TypeName != w.typeName || p.Parse != w.parse {
			t.Errorf("param[%d] = %+v, want %+v", i, p, w)
		}
	}
}
