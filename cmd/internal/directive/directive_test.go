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

func TestParseBodySize(t *testing.T) {
	valid := []struct {
		in   string
		want int64
	}{
		{"1B", 1},
		{"512B", 512},
		{"64KiB", 64 << 10},
		{"512KiB", 512 << 10},
		{"20MiB", 20 << 20},
		{"2GiB", 2 << 30},
		{"9223372036854775807B", 9223372036854775807},
		{"8589934591GiB", 8589934591 << 30},
	}
	for _, tc := range valid {
		got, errMsg := directive.ParseBodySize(tc.in, "h:body")
		if errMsg != "" || got != tc.want {
			t.Errorf("ParseBodySize(%q) = (%d, %q), want (%d, \"\")", tc.in, got, errMsg, tc.want)
		}
	}

	notASize := []string{"", "0B", "0MiB", "20", "20MB", "20mb", "20mib", "20 MiB", "-1MiB", "1.5MiB", "01MiB"}
	for _, in := range notASize {
		_, errMsg := directive.ParseBodySize(in, "h:body")
		want := `h:body: "` + in + `" is not a size: write a positive whole number and B, KiB, MiB or GiB, as 20MiB`
		if errMsg != want {
			t.Errorf("ParseBodySize(%q) err = %q, want %q", in, errMsg, want)
		}
	}

	overflow := []string{"9223372036854775808B", "8589934592GiB", "999999999999999999999MiB"}
	for _, in := range overflow {
		_, errMsg := directive.ParseBodySize(in, "h:body")
		want := `h:body: "` + in + `" is larger than a body can be`
		if errMsg != want {
			t.Errorf("ParseBodySize(%q) err = %q, want %q", in, errMsg, want)
		}
	}
}

func TestBodyDirectiveDiagnostics(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantDiag string
		wantBody int64
	}{
		{
			name: "valid_20MiB",
			src: `package admin
import "net/http"
type H struct{}
//h:key orders.receipt
//h:method POST
//h:path /orders/receipt
//h:desc Upload receipt
//h:body 20MiB
func (h *H) Receipt(w http.ResponseWriter, r *http.Request) error { return nil }
`,
			wantBody: 20 << 20,
		},
		{
			name: "not_a_size",
			src: `package admin
import "net/http"
type H struct{}
//h:key orders.receipt
//h:method POST
//h:path /orders/receipt
//h:desc Upload receipt
//h:body 0B
func (h *H) Receipt(w http.ResponseWriter, r *http.Request) error { return nil }
`,
			wantDiag: `handlers.go:8:1: h:body: "0B" is not a size: write a positive whole number and B, KiB, MiB or GiB, as 20MiB`,
		},
		{
			name: "larger_than_a_body_can_be",
			src: `package admin
import "net/http"
type H struct{}
//h:key orders.receipt
//h:method POST
//h:path /orders/receipt
//h:desc Upload receipt
//h:body 9999999999GiB
func (h *H) Receipt(w http.ResponseWriter, r *http.Request) error { return nil }
`,
			wantDiag: `handlers.go:8:1: h:body: "9999999999GiB" is larger than a body can be`,
		},
		{
			name: "given_twice",
			src: `package admin
import "net/http"
type H struct{}
//h:key orders.receipt
//h:method POST
//h:path /orders/receipt
//h:desc Upload receipt
//h:body 1MiB
//h:body 20MiB
func (h *H) Receipt(w http.ResponseWriter, r *http.Request) error { return nil }
`,
			wantDiag: `handlers.go:9:1: h:body given twice`,
		},
		{
			name: "on_get_route",
			src: `package admin
import "net/http"
type H struct{}
//h:key orders.list
//h:method GET
//h:path /orders
//h:desc Orders
//h:body 20MiB
func (h *H) Orders(w http.ResponseWriter, r *http.Request) error { return nil }
`,
			wantDiag: `handlers.go:8:1: h:body on a GET route, which has no body`,
		},
		{
			name: "near_miss",
			src: `package admin
import "net/http"
type H struct{}
//h:key orders.receipt
//h:method POST
//h:path /orders/receipt
//h:desc Upload receipt
// h:body 20MiB
func (h *H) Receipt(w http.ResponseWriter, r *http.Request) error { return nil }
`,
			wantDiag: `handlers.go:8:1: not a directive: write "//h:body 20MiB"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg, diags := directive.Syntax(map[string][]byte{"handlers.go": []byte(tc.src)})
			if tc.wantDiag == "" {
				if len(diags) != 0 {
					t.Fatalf("unexpected diagnostics: %v", diags)
				}
				if len(pkg.Routes) != 1 || pkg.Routes[0].MaxBody != tc.wantBody {
					t.Fatalf("routes = %+v, want MaxBody=%d", pkg.Routes, tc.wantBody)
				}
				return
			}
			if len(diags) != 1 || diags[0].String() != tc.wantDiag {
				t.Fatalf("diags = %v, want [%s]", diags, tc.wantDiag)
			}
		})
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
			out := ""
			if b, err := os.ReadFile(filepath.Join(caseDir, "out.txt")); err == nil {
				outRel := strings.TrimSpace(string(b))
				if outRel != "" {
					out = filepath.Join(caseDir, filepath.FromSlash(outRel))
				}
			}
			tDiags, err := directive.Types(caseDir, pkg, out)
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
	tDiags, err := directive.Types(dir, pkg, "")
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
