package main_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/epistemer/handlers/internal/routesv1"
)

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenGolden(t *testing.T) {
	bin := buildHandlers(t)

	goldenRoot, err := filepath.Abs(filepath.Join("testdata", "golden"))
	if err != nil {
		t.Fatal(err)
	}
	typesRoot, err := filepath.Abs(filepath.Join("..", "internal", "directive", "testdata", "types"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		modSrc   string
		pkgArg   string
		outRel   string
		prefix   string
		wantFile string
	}{
		{
			name:     "shop",
			modSrc:   goldenRoot,
			pkgArg:   "./shop",
			outRel:   "shoproutes",
			prefix:   "/admin",
			wantFile: filepath.Join(goldenRoot, "shop", "want.routes.json"),
		},
		{
			name:     "aliases",
			modSrc:   goldenRoot,
			pkgArg:   "./aliases",
			outRel:   "aliasesroutes",
			prefix:   "",
			wantFile: filepath.Join(goldenRoot, "aliases", "want.routes.json"),
		},
		{
			name:     "types_clean",
			modSrc:   typesRoot,
			pkgArg:   "./clean",
			outRel:   "cleanroutes",
			prefix:   "",
			wantFile: filepath.Join("testdata", "types_clean", "want.routes.json"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpMod := t.TempDir()
			copyDir(t, tc.modSrc, tmpMod)

			args := []string{"gen", "-out", tc.outRel}
			if tc.prefix != "" {
				args = append(args, "-prefix", tc.prefix)
			}
			args = append(args, tc.pkgArg)

			code, stdout, stderr := runBin(t, bin, tmpMod, "", args...)
			if code != 0 {
				t.Fatalf("gen failed (%d): stdout=%q stderr=%q", code, stdout, stderr)
			}

			outPath := filepath.Join(tmpMod, tc.outRel, "zz_"+tc.outRel+".routes.json")
			got, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}

			want, err := os.ReadFile(tc.wantFile)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
			}

			var v any
			if err := json.Unmarshal(got, &v); err != nil {
				t.Fatal(err)
			}
			if err := routesv1.Check(v); err != nil {
				t.Fatalf("routesv1.Check failed: %v", err)
			}

			// Second run gives the same bytes.
			code, stdout, stderr = runBin(t, bin, tmpMod, "", args...)
			if code != 0 {
				t.Fatalf("second gen failed (%d): stdout=%q stderr=%q", code, stdout, stderr)
			}
			got2, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, got2) {
				t.Fatalf("second run bytes differ")
			}
		})
	}
}

func TestGenFlagsAndErrors(t *testing.T) {
	bin := buildHandlers(t)

	goldenRoot, err := filepath.Abs(filepath.Join("testdata", "golden"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("check_missing_current_stale", func(t *testing.T) {
		tmpMod := t.TempDir()
		copyDir(t, goldenRoot, tmpMod)

		// Missing
		code, _, stderr := runBin(t, bin, tmpMod, "", "gen", "-check", "-out", "shoproutes", "-prefix", "/admin", "./shop")
		if code != 1 || !strings.Contains(stderr, "is missing") {
			t.Fatalf("check missing = (%d, %q), want (1, *is missing*)", code, stderr)
		}

		// Generate
		code, _, stderr = runBin(t, bin, tmpMod, "", "gen", "-out", "shoproutes", "-prefix", "/admin", "./shop")
		if code != 0 {
			t.Fatalf("gen = (%d, %q)", code, stderr)
		}

		// Current
		code, _, stderr = runBin(t, bin, tmpMod, "", "gen", "-check", "-out", "shoproutes", "-prefix", "/admin", "./shop")
		if code != 0 || stderr != "" {
			t.Fatalf("check current = (%d, %q), want (0, \"\")", code, stderr)
		}

		// Stale
		code, _, stderr = runBin(t, bin, tmpMod, "", "gen", "-check", "-out", "shoproutes", "-prefix", "/other", "./shop")
		if code != 1 || !strings.Contains(stderr, "is stale") {
			t.Fatalf("check stale = (%d, %q), want (1, *is stale*)", code, stderr)
		}
	})

	t.Run("strict_warnings_and_bad_prefix_and_errors", func(t *testing.T) {
		tmpMod := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmpMod, "go.mod"), []byte("module example.com/w\n\ngo 1.26.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmpMod, "h.go"), []byte(`package w

import "net/http"

type H struct{}

//h:key home
//h:method GET
//h:path /
//h:auth public
func (h *H) Home(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}

		// Bad prefix -> exit 2, writes nothing
		for _, bad := range []string{"admin", "/admin/", "/admin//x", "/admin/{id}"} {
			code, _, _ := runBin(t, bin, tmpMod, "", "gen", "-out", "out", "-prefix", bad, ".")
			if code != 2 {
				t.Errorf("bad prefix %q exit = %d, want 2", bad, code)
			}
		}
		if _, err := os.Stat(filepath.Join(tmpMod, "out")); !os.IsNotExist(err) {
			t.Fatalf("bad prefix wrote output dir")
		}

		// Warning with -strict -> exit 1, writes nothing (interface §5.1)
		code, stdout, _ := runBin(t, bin, tmpMod, "", "gen", "-strict", "-out", "out", ".")
		if code != 1 || !strings.Contains(stdout, "(warning)") {
			t.Fatalf("strict warning = (%d, %q), want (1, *warning*)", code, stdout)
		}
		if _, err := os.Stat(filepath.Join(tmpMod, "out")); !os.IsNotExist(err) {
			t.Fatalf("-strict with warning wrote output dir")
		}

		// Warning without -strict -> exit 0, writes file
		code, stdout, _ = runBin(t, bin, tmpMod, "", "gen", "-out", "out", ".")
		if code != 0 || !strings.Contains(stdout, "(warning)") {
			t.Fatalf("non-strict warning = (%d, %q)", code, stdout)
		}
		if _, err := os.Stat(filepath.Join(tmpMod, "out", "zz_out.routes.json")); err != nil {
			t.Fatalf("expected output file: %v", err)
		}

		// Error in package -> exit 1, writes nothing
		if err := os.WriteFile(filepath.Join(tmpMod, "h.go"), []byte(`package w

import "net/http"

type H struct{}

//h:key home
//h:method BAD
//h:path /
func (h *H) Home(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ = runBin(t, bin, tmpMod, "", "gen", "-out", "out2", ".")
		if code != 1 || !strings.Contains(stdout, "is not GET") {
			t.Fatalf("error gen = (%d, %q)", code, stdout)
		}
		if _, err := os.Stat(filepath.Join(tmpMod, "out2")); !os.IsNotExist(err) {
			t.Fatalf("error run wrote output dir")
		}

		// Missing out package linked before gen -> exit 1 with guidance
		if err := os.WriteFile(filepath.Join(tmpMod, "h.go"), []byte(`package w

import (
	"net/http"

	"example.com/w/adminroutes"
)

type H struct{}

//h:key home
//h:method GET
//h:path /
func (h *H) Home(w http.ResponseWriter, r *http.Request) error {
	_ = adminroutes.EventsList()
	return nil
}
`), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ = runBin(t, bin, tmpMod, "", "gen", "-out", "adminroutes", ".")
		if code != 1 || !strings.Contains(stdout, "example.com/w/adminroutes doesn't exist yet: generate the routes before linking to them") {
			t.Fatalf("missing out gen = (%d, %q), want message *example.com/w/adminroutes doesn't exist yet: generate the routes before linking to them*", code, stdout)
		}
	})
}

func TestGenBaseline(t *testing.T) {
	bin := buildHandlers(t)

	setupRepo := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/base\n\ngo 1.26.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "admin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "admin", "h.go"), []byte(`package admin

import "net/http"

type H struct{}

//h:key home
//h:method GET
//h:path /
//h:desc Home
//h:auth public
func (h *H) Home(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key users.show
//h:method GET
//h:path /users/{id}
//h:desc Show user
//h:perm users.read
func (h *H) Show(w http.ResponseWriter, r *http.Request, id int64) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	t.Run("disk_baseline", func(t *testing.T) {
		dir := setupRepo(t)
		code, _, stderr := runBin(t, bin, dir, "", "gen", "-out", "adminroutes", "./admin")
		if code != 0 {
			t.Fatalf("initial gen failed (%d): %s", code, stderr)
		}

		// Remove users.show
		if err := os.WriteFile(filepath.Join(dir, "admin", "h.go"), []byte(`package admin

import "net/http"

type H struct{}

//h:key home
//h:method GET
//h:path /
//h:desc Home
//h:auth public
func (h *H) Home(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := runBin(t, bin, dir, "", "gen", "-out", "adminroutes", "./admin")
		if code != 0 {
			t.Fatalf("gen after removal failed (%d): %s", code, stdout)
		}
		for _, want := range []string{
			`zz_adminroutes.routes.json:1:1: key "users.show" is gone since the committed file: its translations and permission grants are orphaned (warning)`,
			`zz_adminroutes.routes.json:1:1: permission "users.read" is gone since the committed file: its grants are orphaned (warning)`,
			`zz_adminroutes.routes.json:1:1: parameter label "route_param.users.show.id" is gone since the committed file: its translations are orphaned (warning)`,
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout missing %q\ngot:\n%s", want, stdout)
			}
		}
	})

	t.Run("git_ref_baseline", func(t *testing.T) {
		dir := setupRepo(t)
		code, _, stderr := runBin(t, bin, dir, "", "gen", "-out", "adminroutes", "./admin")
		if code != 0 {
			t.Fatalf("initial gen failed (%d): %s", code, stderr)
		}

		git := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		git("init")
		git("config", "user.email", "test@example.com")
		git("config", "user.name", "Test")
		git("add", ".")
		git("commit", "-m", "initial")

		// Remove users.show
		if err := os.WriteFile(filepath.Join(dir, "admin", "h.go"), []byte(`package admin

import "net/http"

type H struct{}

//h:key home
//h:method GET
//h:path /
//h:desc Home
//h:auth public
func (h *H) Home(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := runBin(t, bin, dir, "", "gen", "-baseline", "HEAD", "-out", "adminroutes", "./admin")
		if code != 0 {
			t.Fatalf("gen -baseline HEAD failed (%d)", code)
		}
		for _, want := range []string{
			`key "users.show" is gone since HEAD: its translations and permission grants are orphaned (warning)`,
			`permission "users.read" is gone since HEAD: its grants are orphaned (warning)`,
			`parameter label "route_param.users.show.id" is gone since HEAD: its translations are orphaned (warning)`,
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout missing %q\ngot:\n%s", want, stdout)
			}
		}
	})
}

func TestVetAcrossPackages(t *testing.T) {
	bin := buildHandlers(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/multi\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkgA"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pkgB"), 0o755); err != nil {
		t.Fatal(err)
	}

	// pkgA: clean routes, generated into pkgAroutes with prefix /a
	if err := os.WriteFile(filepath.Join(dir, "pkgA", "h.go"), []byte(`package pkgA

import "net/http"

type H struct{}

//h:key a.home
//h:method GET
//h:path /
//h:desc A Home
//h:perm shared.perm
func (h *H) Home(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.item
//h:method GET
//h:path /items/{id}
//h:desc A Item
//h:perm shared.perm
func (h *H) Item(w http.ResponseWriter, r *http.Request, id int64) error { return nil }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runBin(t, bin, dir, "", "gen", "-out", "pkgAroutes", "-prefix", "/a", "./pkgA")
	if code != 0 {
		t.Fatalf("gen pkgA failed (%d): stdout=%q stderr=%q", code, stdout, stderr)
	}

	// Vetting pkgA alone should NOT self-duplicate against pkgAroutes/zz_pkgAroutes.routes.json.
	code, stdout, stderr = runBin(t, bin, dir, "", "vet", "./pkgA")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("vet pkgA alone = (%d, %q, %q), want (0, \"\", \"\")", code, stdout, stderr)
	}

	// pkgB: has routes that conflict with pkgA's committed routes file when pkgB has prefix /a in its own routes file:
	// 1) duplicate key a.home
	// 2) duplicate permission shared.perm
	// 3) conflicting pattern GET /a/items/{name} vs GET /a/items/{id}
	if err := os.WriteFile(filepath.Join(dir, "pkgB", "h.go"), []byte(`package pkgB

import "net/http"

type H struct{}

//h:key a.home
//h:method GET
//h:path /bhome
//h:desc B Home
//h:perm shared.perm
func (h *H) BHome(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key b.item
//h:method GET
//h:path /items/{name}
//h:desc B Item
//h:perm b.perm
func (h *H) BItem(w http.ResponseWriter, r *http.Request, name string) error { return nil }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a committed routes file for pkgB with prefix /a so pattern conflict triggers.
	if err := os.MkdirAll(filepath.Join(dir, "pkgBroutes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkgBroutes", "zz_pkgBroutes.routes.json"), []byte(`{
  "domain": "routes/v1",
  "producer": "handlers",
  "source": "pkgB",
  "package": { "name": "pkgBroutes", "dir": "pkgBroutes" },
  "surface": "html",
  "prefix": "/a",
  "routes": [],
  "permissions": []
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ = runBin(t, bin, dir, "", "vet", "./pkgB")
	if code != 1 {
		t.Fatalf("vet pkgB exit = %d, want 1\nstdout:\n%s", code, stdout)
	}
	for _, want := range []string{
		`key "a.home" also at pkgAroutes/zz_pkgAroutes.routes.json:1`,
		`permission "shared.perm" is in pkgAroutes and pkgBroutes; declare it in one and take it from there with "from <package>"`,
		`GET /a/items/{name} conflicts with GET /a/items/{id} at pkgAroutes/zz_pkgAroutes.routes.json:1`,
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("vet stdout missing %q\ngot:\n%s", want, stdout)
		}
	}
}

func TestVetA7CrossPackageChecks(t *testing.T) {
	bin := buildHandlers(t)

	setupMod := func(t *testing.T) string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/veta7\n\ngo 1.26.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	writeCommittedRoutes := func(t *testing.T, dir, routesRel, pkgName, permKey, permLevel string) {
		p := filepath.Join(dir, routesRel, "zz_"+pkgName+".routes.json")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		var perms []map[string]any
		if permKey != "" {
			perms = append(perms, map[string]any{
				"key":   permKey,
				"var":   "Perm" + strings.ReplaceAll(permKey, ".", ""),
				"text":  "Perm Text",
				"level": permLevel,
			})
		}
		body := map[string]any{
			"domain":      "routes/v1",
			"producer":    "handlers",
			"source":      routesRel,
			"package":     map[string]any{"name": pkgName, "dir": routesRel},
			"surface":     "html",
			"prefix":      "/" + pkgName,
			"routes":      []any{},
			"permissions": perms,
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("no_routes_package", func(t *testing.T) {
		dir := setupMod(t)
		if err := os.MkdirAll(filepath.Join(dir, "admin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "admin", "h.go"), []byte(`package admin
import "net/http"
type H struct{}
//h:key admin.list
//h:method GET
//h:path /list
//h:desc List
//h:perm orders.read from apiroutes
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := runBin(t, bin, dir, "", "vet", "./admin")
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		want := `permission "orders.read" in admin: no routes package apiroutes`
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("two_routes_packages", func(t *testing.T) {
		dir := setupMod(t)
		writeCommittedRoutes(t, dir, "api1routes", "apiroutes", "perm.one", "verified")
		writeCommittedRoutes(t, dir, "api2routes", "apiroutes", "perm.two", "verified")
		if err := os.MkdirAll(filepath.Join(dir, "admin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "admin", "h.go"), []byte(`package admin
import "net/http"
type H struct{}
//h:key admin.list
//h:method GET
//h:path /list
//h:desc List
//h:perm perm.one from apiroutes
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := runBin(t, bin, dir, "", "vet", "./admin")
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		want := `permission "perm.one" in admin: two routes packages are named apiroutes, in api1routes and api2routes`
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("doesnt_declare_it", func(t *testing.T) {
		dir := setupMod(t)
		writeCommittedRoutes(t, dir, "apiroutes", "apiroutes", "other.perm", "verified")
		if err := os.MkdirAll(filepath.Join(dir, "admin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "admin", "h.go"), []byte(`package admin
import "net/http"
type H struct{}
//h:key admin.list
//h:method GET
//h:path /list
//h:desc List
//h:perm orders.read from apiroutes
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := runBin(t, bin, dir, "", "vet", "./admin")
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		want := `permission "orders.read" in admin: apiroutes doesn't declare it`
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("unverified_mismatch", func(t *testing.T) {
		dir := setupMod(t)
		writeCommittedRoutes(t, dir, "apiroutes", "apiroutes", "orders.read", "verified")
		if err := os.MkdirAll(filepath.Join(dir, "admin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "admin", "h.go"), []byte(`package admin
import "net/http"
type H struct{}
//h:key admin.list
//h:method GET
//h:path /list
//h:desc List
//h:auth unverified
//h:perm orders.read from apiroutes
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := runBin(t, bin, dir, "", "vet", "./admin")
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		want := `permission "orders.read" in admin: admin.list is unverified and the declaration in apiroutes isn't`
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})
}
