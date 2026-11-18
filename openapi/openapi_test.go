package openapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/epistemer/errors"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/handlerstest"
	"github.com/epistemer/handlers/openapi"
	"github.com/epistemer/i18n"
	"github.com/getkin/kin-openapi/openapi3"
)

const testSpecYAML = `openapi: 3.0.3
info:
  title: Adapter test
  version: 1.0.0
servers:
  - url: https://other-host.example/wrong-prefix
security:
  - bearerAuth: []
paths:
  /:
    get:
      operationId: rootInfo
      security: []
      responses:
        "200":
          description: ok
  /auth/sign-in:
    post:
      operationId: signIn
      security: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [password]
              properties:
                password:
                  type: string
                  minLength: 8
      responses:
        "200":
          description: ok
  /orgs/{orgId}/orders/{orderId}:
    parameters:
      - name: orgId
        in: path
        required: true
        schema:
          type: integer
          format: int64
          minimum: 1
    get:
      operationId: getOrder
      parameters:
        - name: orderId
          in: path
          required: true
          schema:
            type: integer
            format: int64
            minimum: 1
        - name: status
          in: query
          schema:
            type: string
            enum: [open, paid]
      responses:
        "200":
          description: ok
  /orders:
    post:
      operationId: placeOrder
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [items]
              properties:
                items:
                  type: array
                  minItems: 1
                  items:
                    type: object
                    required: [qty]
                    properties:
                      qty:
                        type: integer
                        minimum: 1
      responses:
        "201":
          description: created
  /orders/{orderId}/receipt:
    post:
      operationId: uploadReceipt
      parameters:
        - name: orderId
          in: path
          required: true
          schema:
            type: integer
            format: int64
            minimum: 1
      requestBody:
        required: true
        content:
          multipart/form-data:
            schema:
              type: object
              required: [file]
              properties:
                file:
                  type: string
                  format: binary
      responses:
        "204":
          description: stored
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
`

func loadTestSpec(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData([]byte(testSpecYAML))
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func testRoutes() []handlers.Route {
	lbl := i18n.Key("test.api.lbl", "Test")
	return []handlers.Route{
		{Key: "api.root", Method: "GET", Prefix: "/api/v1", Pattern: "/", Surface: handlers.JSON, Auth: handlers.Public, Label: lbl, Operation: "rootInfo"},
		{Key: "api.sign_in", Method: "POST", Prefix: "/api/v1", Pattern: "/auth/sign-in", Surface: handlers.JSON, Auth: handlers.Public, Label: lbl, Operation: "signIn"},
		{Key: "api.get_order", Method: "GET", Prefix: "/api/v1", Pattern: "/orgs/{orgId}/orders/{orderId}", Surface: handlers.JSON, Auth: handlers.Permission, Permission: "orders.read", Label: lbl, Operation: "getOrder"},
		{Key: "api.place_order", Method: "POST", Prefix: "/api/v1", Pattern: "/orders", Surface: handlers.JSON, Auth: handlers.Permission, Permission: "orders.write", Label: lbl, Operation: "placeOrder"},
		{Key: "api.upload_receipt", Method: "POST", Prefix: "/api/v1", Pattern: "/orders/{orderId}/receipt", Surface: handlers.JSON, Auth: handlers.Session, Label: lbl, Operation: "uploadReceipt"},
	}
}

func assertPanicsWith(t *testing.T, name, substr string, fn func()) {
	t.Helper()
	defer func() {
		v := recover()
		if v == nil {
			t.Fatalf("%s: expected panic, got none", name)
		}
		if substr != "" && !strings.Contains(fmt.Sprint(v), substr) {
			t.Fatalf("%s: panic = %q, want substring %q", name, v, substr)
		}
	}()
	fn()
}

func TestMountPanics(t *testing.T) {
	spec := loadTestSpec(t)
	routes := testRoutes()[:1]
	g := &handlerstest.Guard{Session: true}

	assertPanicsWith(t, "nil mux", "nil mux", func() {
		openapi.Mount(nil, routes, g, spec, func(*openapi.Router) {})
	})
	assertPanicsWith(t, "nil spec", "nil spec", func() {
		openapi.Mount(http.NewServeMux(), routes, g, nil, func(*openapi.Router) {})
	})
	assertPanicsWith(t, "nil register", "nil register", func() {
		openapi.Mount(http.NewServeMux(), routes, g, spec, nil)
	})
	assertPanicsWith(t, "unknown route pattern", "handlers/openapi: GET /api/v1/missing: no route", func() {
		openapi.Mount(http.NewServeMux(), routes, g, spec, func(rt *openapi.Router) {
			rt.HandleFunc("GET /api/v1/missing", func(http.ResponseWriter, *http.Request) {})
		})
	})
	assertPanicsWith(t, "missing path in spec", "missing from spec", func() {
		badRoutes := []handlers.Route{
			{Key: "x", Method: "GET", Prefix: "/api/v1", Pattern: "/nope", Surface: handlers.JSON, Auth: handlers.Public, Label: i18n.Key("test.x", "X")},
		}
		openapi.Mount(http.NewServeMux(), badRoutes, g, spec, func(rt *openapi.Router) {
			rt.HandleFunc("GET /api/v1/nope", func(http.ResponseWriter, *http.Request) {})
		})
	})
	assertPanicsWith(t, "missing operation in path item", "operation DELETE missing", func() {
		badRoutes := []handlers.Route{
			{Key: "x", Method: "DELETE", Prefix: "/api/v1", Pattern: "/orders", Surface: handlers.JSON, Auth: handlers.Public, Label: i18n.Key("test.x", "X")},
		}
		openapi.Mount(http.NewServeMux(), badRoutes, g, spec, func(rt *openapi.Router) {
			rt.HandleFunc("DELETE /api/v1/orders", func(http.ResponseWriter, *http.Request) {})
		})
	})
	assertPanicsWith(t, "nil handler", "nil handler", func() {
		openapi.Mount(http.NewServeMux(), routes, g, spec, func(rt *openapi.Router) {
			rt.HandleFunc("GET /api/v1/", nil)
		})
	})
	assertPanicsWith(t, "incomplete registration", "was not handled", func() {
		openapi.Mount(http.NewServeMux(), routes, g, spec, func(*openapi.Router) {})
	})
}

type logCapture struct {
	mu      sync.Mutex
	records []string
}

func (l *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (l *logCapture) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, fmt.Sprintf("%s=%v", a.Key, a.Value))
		return true
	})
	l.records = append(l.records, fmt.Sprintf("%s %s %s", r.Level, r.Message, strings.Join(attrs, " ")))
	return nil
}
func (l *logCapture) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logCapture) WithGroup(string) slog.Handler      { return l }

type errorJSON struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeErrorJSON(t *testing.T, rec *httptest.ResponseRecorder) errorJSON {
	t.Helper()
	var e errorJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode error json %q: %v", rec.Body.String(), err)
	}
	return e
}

func TestOpenAPIMountBehaviour(t *testing.T) {
	spec := loadTestSpec(t)
	origServers := append(openapi3.Servers(nil), spec.Servers...)
	routes := testRoutes()
	g := &handlerstest.Guard{
		Session: true,
		Granted: map[string]bool{"orders.read": true, "orders.write": true},
	}

	var mwEvents []string
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mwEvents = append(mwEvents, "mw")
			next.ServeHTTP(w, r)
		})
	}

	var capturedRouter *openapi.Router
	var capturedErrs []string

	mux := http.NewServeMux()
	openapi.Mount(mux, routes, g, spec, func(rt *openapi.Router) {
		capturedRouter = rt
		rt.HandleFunc("GET /api/v1/", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "root")
		})
		rt.HandleFunc("POST /api/v1/auth/sign-in", func(w http.ResponseWriter, r *http.Request) {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				rt.WriteError(w, r, err)
				return
			}
			_, _ = w.Write(b)
		})
		rt.HandleFunc("GET /api/v1/orgs/{orgId}/orders/{orderId}", func(w http.ResponseWriter, r *http.Request) {
			mwEvents = append(mwEvents, "handler")
			route, ok := handlers.From(r.Context())
			if !ok || route.Operation != "getOrder" {
				t.Errorf("route in ctx = (%+v, %v)", route, ok)
			}
			_, _ = fmt.Fprintf(w, "%s:%s", r.PathValue("orgId"), r.PathValue("orderId"))
		})
		rt.HandleFunc("POST /api/v1/orders", func(w http.ResponseWriter, r *http.Request) {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				rt.WriteError(w, r, err)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(b)
		})
		rt.HandleFunc("POST /api/v1/orders/{orderId}/receipt", func(w http.ResponseWriter, r *http.Request) {
			mr, err := r.MultipartReader()
			if err != nil {
				rt.RequestError(w, r, err)
				return
			}
			part, err := mr.NextPart()
			if err != nil {
				rt.RequestError(w, r, err)
				return
			}
			data, err := io.ReadAll(part)
			if err != nil {
				rt.WriteError(w, r, err)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, "uploaded:%d", len(data))
		})
	}, handlers.WithMaxBody(256), handlers.WithMiddleware(mw), handlers.WithErrors(func(w http.ResponseWriter, r *http.Request, err error) {
		capturedErrs = append(capturedErrs, err.Error())
		handlers.WriteJSON(w, r, err)
	}))

	// Caller's spec.Servers must never be mutated.
	if len(spec.Servers) != len(origServers) || spec.Servers[0].URL != origServers[0].URL {
		t.Fatalf("spec.Servers was mutated: %+v", spec.Servers)
	}

	t.Run("root {$} and catch-all 404 and HEAD on GET", func(t *testing.T) {
		rec := httptest.NewRecorder()
		capturedRouter.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/", nil))
		if rec.Code != 200 || rec.Body.String() != "root" {
			t.Errorf("GET /api/v1/ = (%d, %q)", rec.Code, rec.Body.String())
		}

		recHead := httptest.NewRecorder()
		capturedRouter.ServeHTTP(recHead, httptest.NewRequest("HEAD", "/api/v1/", nil))
		if recHead.Code != 200 {
			t.Errorf("HEAD /api/v1/ = %d", recHead.Code)
		}

		recMiss := httptest.NewRecorder()
		capturedRouter.ServeHTTP(recMiss, httptest.NewRequest("GET", "/api/v1/nope", nil))
		if recMiss.Code != 404 || decodeErrorJSON(t, recMiss).Code != "errors.handlers.not_found" {
			t.Errorf("GET /api/v1/nope = (%d, %q)", recMiss.Code, recMiss.Body.String())
		}
	})

	t.Run("guard before validation", func(t *testing.T) {
		g.Session = false
		mwEvents = nil
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/orgs/notint/orders/1", nil))
		if rec.Code != 401 || len(mwEvents) != 0 {
			t.Errorf("no session bad param = (%d, events=%v)", rec.Code, mwEvents)
		}

		g.Session = true
		g.Granted = nil
		rec403 := httptest.NewRecorder()
		mux.ServeHTTP(rec403, httptest.NewRequest("GET", "/api/v1/orgs/1/orders/1", nil))
		if rec403.Code != 403 {
			t.Errorf("no permission = %d", rec403.Code)
		}
		g.Granted = map[string]bool{"orders.read": true, "orders.write": true}
	})

	t.Run("path-item and operation parameter validation", func(t *testing.T) {
		// Path-item param orgId invalid.
		capturedErrs = nil
		recOrg := httptest.NewRecorder()
		mux.ServeHTTP(recOrg, httptest.NewRequest("GET", "/api/v1/orgs/abc/orders/5", nil))
		eOrg := decodeErrorJSON(t, recOrg)
		if recOrg.Code != 400 || eOrg.Code != "errors.handlers.invalid_field" || !strings.Contains(eOrg.Message, "orgId") || strings.Contains(eOrg.Message, "abc") {
			t.Errorf("bad orgId = (%d, %+v)", recOrg.Code, eOrg)
		}
		if len(capturedErrs) != 1 || strings.Contains(capturedErrs[0], "abc") {
			t.Errorf("logged cause leaked value: %v", capturedErrs)
		}

		// Query enum invalid.
		capturedErrs = nil
		recEnum := httptest.NewRecorder()
		mux.ServeHTTP(recEnum, httptest.NewRequest("GET", "/api/v1/orgs/2/orders/5?status=secretval", nil))
		eEnum := decodeErrorJSON(t, recEnum)
		if recEnum.Code != 400 || eEnum.Code != "errors.handlers.invalid_field" || !strings.Contains(eEnum.Message, "status") || strings.Contains(eEnum.Message, "secretval") {
			t.Errorf("bad enum = (%d, %+v)", recEnum.Code, eEnum)
		}
		if len(capturedErrs) != 1 || strings.Contains(capturedErrs[0], "secretval") {
			t.Errorf("logged enum cause leaked value: %v", capturedErrs)
		}

		// Valid with middleware and route in context.
		mwEvents = nil
		recOK := httptest.NewRecorder()
		mux.ServeHTTP(recOK, httptest.NewRequest("GET", "/api/v1/orgs/2/orders/5?status=open", nil))
		if recOK.Code != 200 || recOK.Body.String() != "2:5" || strings.Join(mwEvents, ",") != "mw,handler" {
			t.Errorf("valid getOrder = (%d, %q, events=%v)", recOK.Code, recOK.Body.String(), mwEvents)
		}
	})

	t.Run("JSON body validation, value redaction, body restoration, 413", func(t *testing.T) {
		// Password failing minLength must not appear in response OR wrapped cause error.
		capturedErrs = nil
		req := httptest.NewRequest("POST", "/api/v1/auth/sign-in", strings.NewReader(`{"password":"pw123"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		e := decodeErrorJSON(t, rec)
		if rec.Code != 400 || e.Code != "errors.handlers.invalid_field" || !strings.Contains(e.Message, "/password") || strings.Contains(e.Message, "pw123") {
			t.Errorf("short password = (%d, %+v)", rec.Code, e)
		}
		if len(capturedErrs) != 1 || strings.Contains(capturedErrs[0], "pw123") {
			t.Errorf("logged password cause leaked value: %v", capturedErrs)
		}

		// Nested JSON pointer /items/0/qty.
		reqNested := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(`{"items":[{"qty":0}]}`))
		reqNested.Header.Set("Content-Type", "application/json")
		recNested := httptest.NewRecorder()
		mux.ServeHTTP(recNested, reqNested)
		eNested := decodeErrorJSON(t, recNested)
		if recNested.Code != 400 || eNested.Code != "errors.handlers.invalid_field" || !strings.Contains(eNested.Message, "/items/0/qty") {
			t.Errorf("nested qty = (%d, %+v)", recNested.Code, eNested)
		}

		// Malformed JSON -> ErrInvalidRequest (no location).
		reqBadJSON := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(`{bad`))
		reqBadJSON.Header.Set("Content-Type", "application/json")
		recBadJSON := httptest.NewRecorder()
		mux.ServeHTTP(recBadJSON, reqBadJSON)
		eBadJSON := decodeErrorJSON(t, recBadJSON)
		if recBadJSON.Code != 400 || eBadJSON.Code != "errors.handlers.invalid_request" {
			t.Errorf("malformed JSON = (%d, %+v)", recBadJSON.Code, eBadJSON)
		}

		// Valid body is put back for handler to read.
		payload := `{"items":[{"qty":3}]}`
		reqOK := httptest.NewRequest("POST", "/api/v1/orders", strings.NewReader(payload))
		reqOK.Header.Set("Content-Type", "application/json")
		recOK := httptest.NewRecorder()
		mux.ServeHTTP(recOK, reqOK)
		if recOK.Code != 201 || recOK.Body.String() != payload {
			t.Errorf("valid body = (%d, %q)", recOK.Code, recOK.Body.String())
		}

		// Body over WithMaxBody(256) -> 413 ErrTooLarge.
		big := `{"password":"` + strings.Repeat("a", 300) + `"}`
		reqBig := httptest.NewRequest("POST", "/api/v1/auth/sign-in", strings.NewReader(big))
		reqBig.Header.Set("Content-Type", "application/json")
		recBig := httptest.NewRecorder()
		mux.ServeHTTP(recBig, reqBig)
		eBig := decodeErrorJSON(t, recBig)
		if recBig.Code != 413 || eBig.Code != "errors.handlers.too_large" {
			t.Errorf("over maxBody = (%d, %+v)", recBig.Code, eBig)
		}
	})

	t.Run("multipart ExcludeRequestBody streams and enforces maxBody", func(t *testing.T) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, err := mw.CreateFormFile("file", "receipt.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(fw, "hello")
		_ = mw.Close()

		req := httptest.NewRequest("POST", "/api/v1/orders/9/receipt", bytes.NewReader(body.Bytes()))
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != 200 || rec.Body.String() != "uploaded:5" {
			t.Errorf("multipart = (%d, %q)", rec.Code, rec.Body.String())
		}

		// Path param validation still runs for multipart.
		reqBadParam := httptest.NewRequest("POST", "/api/v1/orders/0/receipt", bytes.NewReader(body.Bytes()))
		reqBadParam.Header.Set("Content-Type", mw.FormDataContentType())
		recBadParam := httptest.NewRecorder()
		mux.ServeHTTP(recBadParam, reqBadParam)
		if recBadParam.Code != 400 || decodeErrorJSON(t, recBadParam).Code != "errors.handlers.invalid_field" {
			t.Errorf("multipart bad param = (%d, %q)", recBadParam.Code, recBadParam.Body.String())
		}

		// Multipart over WithMaxBody(256) -> 413 via RequestError/WriteError.
		var bigBody bytes.Buffer
		mwBig := multipart.NewWriter(&bigBody)
		fwBig, _ := mwBig.CreateFormFile("file", "big.txt")
		_, _ = fwBig.Write(bytes.Repeat([]byte("x"), 300))
		_ = mwBig.Close()
		reqBig := httptest.NewRequest("POST", "/api/v1/orders/9/receipt", bytes.NewReader(bigBody.Bytes()))
		reqBig.Header.Set("Content-Type", mwBig.FormDataContentType())
		recBig := httptest.NewRecorder()
		mux.ServeHTTP(recBig, reqBig)
		if recBig.Code != 413 || decodeErrorJSON(t, recBig).Code != "errors.handlers.too_large" {
			t.Errorf("multipart big = (%d, %q)", recBig.Code, recBig.Body.String())
		}
	})

	t.Run("RequestError logs at debug level and maps MaxBytesError", func(t *testing.T) {
		orig := slog.Default()
		cap := &logCapture{}
		slog.SetDefault(slog.New(cap))
		t.Cleanup(func() { slog.SetDefault(orig) })

		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest("GET", "/", nil)
		capturedRouter.RequestError(rec1, req1, stderrors.New("bad query"))
		if rec1.Code != 400 || decodeErrorJSON(t, rec1).Code != "errors.handlers.invalid_request" {
			t.Errorf("RequestError bad query = (%d, %q)", rec1.Code, rec1.Body.String())
		}

		rec2 := httptest.NewRecorder()
		capturedRouter.RequestError(rec2, req1, fmt.Errorf("wrap: %w", &http.MaxBytesError{Limit: 10}))
		if rec2.Code != 413 || decodeErrorJSON(t, rec2).Code != "errors.handlers.too_large" {
			t.Errorf("RequestError MaxBytesError = (%d, %q)", rec2.Code, rec2.Body.String())
		}

		if len(cap.records) != 2 || !strings.Contains(cap.records[0], "DEBUG") || !strings.Contains(cap.records[0], "bad query") {
			t.Errorf("debug logs = %v", cap.records)
		}

		rec3 := httptest.NewRecorder()
		capturedRouter.WriteError(rec3, req1, errors.Key("test.custom", "custom", errors.Conflict))
		if rec3.Code != 409 || decodeErrorJSON(t, rec3).Code != "test.custom" {
			t.Errorf("WriteError = (%d, %q)", rec3.Code, rec3.Body.String())
		}
	})
}
