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

func TestOpenAPIGolden(t *testing.T) {
	bin := buildHandlers(t)
	openapiRoot, err := filepath.Abs(filepath.Join("testdata", "openapi"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		specRel   string
		outRel    string
		pkgName   string
		keyPrefix string
		wantFile  string
		wantWarn  string
	}{
		{
			name:      "toy",
			specRel:   "toy/spec.yaml",
			outRel:    "apiroutes",
			keyPrefix: "api",
			wantFile:  filepath.Join(openapiRoot, "toy", "want.routes.json"),
			wantWarn:  `toy/spec.yaml: GET /orders: x-permission "orders.read" is used by this operation only (warning)`,
		},
		{
			name:      "features",
			specRel:   "features/spec.yaml",
			outRel:    "out",
			pkgName:   "feat",
			keyPrefix: "api",
			wantFile:  filepath.Join(openapiRoot, "features", "want.routes.json"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpMod := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpMod, "go.mod"), []byte("module example.com/api\n\ngo 1.26.0\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			copyDir(t, filepath.Join(openapiRoot, tc.name), filepath.Join(tmpMod, tc.name))

			args := []string{"openapi", "-out", tc.outRel}
			if tc.pkgName != "" {
				args = append(args, "-package", tc.pkgName)
			}
			if tc.keyPrefix != "" {
				args = append(args, "-key-prefix", tc.keyPrefix)
			}
			args = append(args, tc.specRel)

			code, stdout, stderr := runBin(t, bin, tmpMod, "", args...)
			if code != 0 {
				t.Fatalf("openapi failed (%d): stdout=%q stderr=%q", code, stdout, stderr)
			}
			if tc.wantWarn != "" {
				if !strings.Contains(stdout, tc.wantWarn) {
					t.Errorf("stdout = %q, want warning %q", stdout, tc.wantWarn)
				}
			} else if stdout != "" {
				t.Errorf("unexpected stdout: %q", stdout)
			}

			pkg := tc.pkgName
			if pkg == "" {
				pkg = filepath.Base(tc.outRel)
			}
			outPath := filepath.Join(tmpMod, tc.outRel, "zz_"+pkg+".routes.json")
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

			// Second run gives identical bytes.
			code, _, stderr = runBin(t, bin, tmpMod, "", args...)
			if code != 0 {
				t.Fatalf("second run failed (%d): %s", code, stderr)
			}
			got2, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, got2) {
				t.Fatal("second run bytes differ")
			}
		})
	}
}

func TestOpenAPI72Rows(t *testing.T) {
	bin := buildHandlers(t)
	errDir, err := filepath.Abs(filepath.Join("testdata", "openapi", "errors"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		spec string
		want string
	}{
		{"not_30x.yaml", `openapi: "3.1.0" is not 3.0.x`},
		{"server_not_url.yaml", `servers[0].url: "://bad" is not a URL`},
		{"server_trailing_slash.yaml", `servers[0].url: "https://shop.example/api/v1/": the path must not end with "/"`},
		{"server_variable.yaml", `servers[0].url: "https://shop.example/{ver}/api": the path must not hold a variable`},
		{"unsupported_method.yaml", `HEAD /orders: method HEAD isn't supported`},
		{"missing_operation_id.yaml", `GET /orders: no operationId`},
		{"dup_operation_id.yaml", `POST /orders: operationId "listOrders" also at GET /orders`},
		{"op_id_not_ident.yaml", `GET /orders: operationId "list-orders" is not a Go identifier`},
		{"dup_go_name.yaml", `POST /orders: operationId "GetOrder" gives the Go name GetOrder, also from GET /orders`},
		{"reserved_op_name.yaml", `GET /routes: Routes is a name the generated package declares`},
		{"perm_go_name.yaml", `GET /orders/all: keys "orders.readAll" and "orders.read_all" both give the Go name PermOrdersReadAll`},
		{"no_security.yaml", `GET /orders: no security; write security: [] for a public operation`},
		{"optional_security.yaml", `GET /orders: optional security isn't supported`},
		{"unknown_scheme.yaml", `GET /orders: security names undeclared scheme "missingAuth"`},
		{"not_http_bearer.yaml", `GET /orders: scheme "apiKeyAuth" isn't http bearer`},
		{"public_x_permission.yaml", `GET /orders: x-permission on a public operation`},
		{"public_x_auth.yaml", `GET /orders: x-auth on a public operation`},
		{"x_auth_with_x_permission.yaml", `GET /orders: x-auth: session takes no x-permission`},
		{"x_auth_invalid.yaml", `GET /orders: x-auth must be session`},
		{"bad_x_permission.yaml", `GET /orders: x-permission "Bad" is not a key`},
		{"bad_x_route_key.yaml", `GET /orders: x-route-key "Bad" is not a key`},
		{"dup_key.yaml", `POST /orders: key "list_orders" also from operationId "listOrders"`},
		{"empty_x_label.yaml", `GET /orders/{id}: x-label is empty`},
		{"op_name_label_var.yaml", `GET /other: L10nHome is also the label variable of home`},
		{"op_name_perm_var.yaml", `GET /other: PermOrdersRead is also the variable of permission orders.read`},
		{"bad_error_response.yaml", `GET /orders: response 400 doesn't use #/components/schemas/Error`},
		{"missing_error_schema.yaml", `components/schemas/Error: needs string properties code and message`},
		{"wrong_error_schema.yaml", `components/schemas/Error: needs string properties code and message`},
		{"path_param_not_ident.yaml", `GET /orders/{order-id}: path parameter "order-id" is not a Go identifier`},
		{"path_param_partial_segment.yaml", `GET /orders/{id}.json: parameter {id} doesn't fill its segment`},
		{"path_param_allow_reserved.yaml", `GET /orders/{id}: parameter "id" sets allowReserved`},
		{"path_param_unsupported_schema.yaml", `GET /orders/{id}: path parameter "id": number isn't supported`},
		{"path_param_x_go_type.yaml", `GET /orders/{id}: path parameter "id": x-go-type isn't supported`},
	}

	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			tmpMod := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpMod, "go.mod"), []byte("module example.com/e\n\ngo 1.26.0\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(errDir, tc.spec))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tmpMod, tc.spec), b, 0o644); err != nil {
				t.Fatal(err)
			}

			code, stdout, stderr := runBin(t, bin, tmpMod, "", "openapi", "-out", "out", tc.spec)
			if code != 1 {
				t.Fatalf("exit = %d, want 1 (stdout=%q stderr=%q)", code, stdout, stderr)
			}
			wantLine := tc.spec + ": " + tc.want
			if !strings.Contains(stdout, wantLine) {
				t.Errorf("stdout = %q, want line %q", stdout, wantLine)
			}
			if _, err := os.Stat(filepath.Join(tmpMod, "out")); !os.IsNotExist(err) {
				t.Fatalf("error run wrote output dir")
			}
		})
	}

	t.Run("bad_key_prefix", func(t *testing.T) {
		tmpMod := t.TempDir()
		code, _, stderr := runBin(t, bin, tmpMod, "", "openapi", "-out", "out", "-key-prefix", "Bad", "spec.yaml")
		if code != 2 || !strings.Contains(stderr, `-key-prefix "Bad" is not a key`) {
			t.Fatalf("bad -key-prefix = (%d, %q)", code, stderr)
		}
	})

	t.Run("sorted_by_location_then_message", func(t *testing.T) {
		tmpMod := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmpMod, "go.mod"), []byte("module example.com/s\n\ngo 1.26.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(errDir, "sorted_multi_errors.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmpMod, "spec.yaml"), b, 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ := runBin(t, bin, tmpMod, "", "openapi", "-out", "out", "spec.yaml")
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
		want := strings.Join([]string{
			`spec.yaml: GET /a: operationId "a-op" is not a Go identifier`,
			`spec.yaml: GET /a: x-permission on a public operation`,
			`spec.yaml: GET /z: no security; write security: [] for a public operation`,
			`spec.yaml: components/schemas/Error: needs string properties code and message`,
			`spec.yaml: openapi: "3.1.0" is not 3.0.x`,
			`spec.yaml: servers[0].url: "://bad" is not a URL`,
		}, "\n") + "\n"
		if stdout != want {
			t.Fatalf("sorted diagnostics mismatch:\ngot:\n%s\nwant:\n%s", stdout, want)
		}
	})
}

func TestOpenAPIFlagsAndBaselineAndVet(t *testing.T) {
	bin := buildHandlers(t)
	toySpec, err := os.ReadFile(filepath.Join("testdata", "openapi", "toy", "spec.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("check_missing_current_stale_and_strict", func(t *testing.T) {
		tmpMod := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmpMod, "go.mod"), []byte("module example.com/c\n\ngo 1.26.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmpMod, "openapi.yaml"), toySpec, 0o644); err != nil {
			t.Fatal(err)
		}

		// -strict with single-use x-permission warning -> exit 1, writes nothing.
		code, stdout, _ := runBin(t, bin, tmpMod, "", "openapi", "-strict", "-key-prefix", "api", "-out", "apiroutes", "openapi.yaml")
		if code != 1 || !strings.Contains(stdout, "(warning)") {
			t.Fatalf("strict warning = (%d, %q)", code, stdout)
		}
		if _, err := os.Stat(filepath.Join(tmpMod, "apiroutes")); !os.IsNotExist(err) {
			t.Fatalf("-strict with warning wrote output")
		}

		// -check missing.
		code, _, stderr := runBin(t, bin, tmpMod, "", "openapi", "-check", "-key-prefix", "api", "-out", "apiroutes", "openapi.yaml")
		if code != 1 || !strings.Contains(stderr, "is missing") {
			t.Fatalf("check missing = (%d, %q)", code, stderr)
		}

		// Generate.
		code, _, stderr = runBin(t, bin, tmpMod, "", "openapi", "-key-prefix", "api", "-out", "apiroutes", "openapi.yaml")
		if code != 0 {
			t.Fatalf("generate = (%d, %q)", code, stderr)
		}

		// -check current.
		code, _, stderr = runBin(t, bin, tmpMod, "", "openapi", "-check", "-key-prefix", "api", "-out", "apiroutes", "openapi.yaml")
		if code != 0 || stderr != "" {
			t.Fatalf("check current = (%d, %q)", code, stderr)
		}

		// -check stale (different key-prefix).
		code, _, stderr = runBin(t, bin, tmpMod, "", "openapi", "-check", "-key-prefix", "v2", "-out", "apiroutes", "openapi.yaml")
		if code != 1 || !strings.Contains(stderr, "is stale") {
			t.Fatalf("check stale = (%d, %q)", code, stderr)
		}
	})

	t.Run("baseline_git_ref_removal_warnings", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/b\n\ngo 1.26.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "openapi.yaml"), toySpec, 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runBin(t, bin, dir, "", "openapi", "-key-prefix", "api", "-out", "apiroutes", "openapi.yaml")
		if code != 0 {
			t.Fatalf("initial openapi = (%d, %q)", code, stderr)
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
		git("commit", "-m", "init")

		// Replace spec with minimal one removing routes and permissions.
		minSpec := `openapi: 3.0.3
info: {title: Toy, version: 1.0.0}
servers: [{url: https://shop.example/api/v1}]
paths:
  /auth/sign-in:
    post:
      operationId: signIn
      security: []
      responses: {"200": {description: ok}}
components:
  schemas:
    Error:
      type: object
      properties:
        code: {type: string}
        message: {type: string}
`
		if err := os.WriteFile(filepath.Join(dir, "openapi.yaml"), []byte(minSpec), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, _ := runBin(t, bin, dir, "", "openapi", "-baseline", "HEAD", "-key-prefix", "api", "-out", "apiroutes", "openapi.yaml")
		if code != 0 {
			t.Fatalf("openapi -baseline HEAD = (%d, %q)", code, stdout)
		}
		for _, want := range []string{
			`key "api.get_order" is gone since HEAD: its translations and permission grants are orphaned (warning)`,
			`permission "orders.read" is gone since HEAD: its grants are orphaned (warning)`,
			`parameter label "route_param.api.get_order.orderId" is gone since HEAD: its translations are orphaned (warning)`,
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout missing %q\ngot:\n%s", want, stdout)
			}
		}
	})

	t.Run("vet_checks_admin_against_api_routes_json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/mix\n\ngo 1.26.0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "openapi.yaml"), toySpec, 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, stderr := runBin(t, bin, dir, "", "openapi", "-key-prefix", "api", "-out", "apiroutes", "openapi.yaml")
		if code != 0 {
			t.Fatalf("openapi failed (%d): %s", code, stderr)
		}

		if err := os.MkdirAll(filepath.Join(dir, "admin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "admin", "h.go"), []byte(`package admin

import "net/http"

type H struct{}

//h:key api.list_orders
//h:method GET
//h:path /orders
//h:desc Orders
//h:perm orders.read
func (h *H) Orders(w http.ResponseWriter, r *http.Request) error { return nil }
`), 0o644); err != nil {
			t.Fatal(err)
		}

		code, stdout, _ := runBin(t, bin, dir, "", "vet", "./admin")
		if code != 1 {
			t.Fatalf("vet exit = %d, want 1", code)
		}
		for _, want := range []string{
			`key "api.list_orders" also at apiroutes/zz_apiroutes.routes.json:1`,
			`permission "orders.read" is in apiroutes and admin; a permission belongs to one routes package`,
		} {
			if !strings.Contains(stdout, want) {
				t.Errorf("vet stdout missing %q\ngot:\n%s", want, stdout)
			}
		}
	})
}
