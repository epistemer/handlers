package lsp

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type testClient struct {
	t        *testing.T
	pw       *io.PipeWriter
	br       *bufio.Reader
	done     chan struct{}
	exitCode int
	nextID   int
}

func startTestServer(t *testing.T) *testClient {
	t.Helper()
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	c := &testClient{
		t:    t,
		pw:   cw,
		br:   bufio.NewReader(cr),
		done: make(chan struct{}),
	}
	go func() {
		c.exitCode = Run(sr, sw)
		close(c.done)
		_ = sw.Close()
		_ = sr.Close()
	}()
	t.Cleanup(func() {
		_ = cw.Close()
		_ = cr.Close()
		select {
		case <-c.done:
		case <-time.After(3 * time.Second):
			t.Errorf("server did not exit within timeout")
		}
	})
	return c
}

func (c *testClient) send(msg any) {
	c.t.Helper()
	if err := writeFrame(c.pw, msg); err != nil {
		c.t.Fatalf("writeFrame: %v", err)
	}
}

func (c *testClient) request(method string, params any) int {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	c.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	return id
}

func (c *testClient) notify(method string, params any) {
	c.t.Helper()
	c.send(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

type rawRPC struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

func (c *testClient) readMsg() rawRPC {
	c.t.Helper()
	type res struct {
		msg rawRPC
		err error
	}
	ch := make(chan res, 1)
	go func() {
		body, err := readFrame(c.br)
		if err != nil {
			ch <- res{err: err}
			return
		}
		var m rawRPC
		err = json.Unmarshal(body, &m)
		ch <- res{msg: m, err: err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			c.t.Fatalf("readMsg: %v", r.err)
		}
		return r.msg
	case <-time.After(5 * time.Second):
		c.t.Fatalf("timed out waiting for server message")
		return rawRPC{}
	}
}

func (c *testClient) readResponse(wantID int) rawRPC {
	c.t.Helper()
	for {
		m := c.readMsg()
		if m.ID != nil {
			var gotID int
			if err := json.Unmarshal(*m.ID, &gotID); err == nil && gotID == wantID {
				return m
			}
		}
	}
}

func (c *testClient) readDiagnosticsFor(uri string) PublishDiagnosticsParams {
	c.t.Helper()
	for {
		m := c.readMsg()
		if m.Method == "textDocument/publishDiagnostics" {
			var p PublishDiagnosticsParams
			if err := json.Unmarshal(m.Params, &p); err != nil {
				c.t.Fatalf("unmarshal publishDiagnostics: %v", err)
			}
			if p.URI == uri {
				return p
			}
		}
	}
}

func (c *testClient) waitExit() int {
	c.t.Helper()
	select {
	case <-c.done:
		return c.exitCode
	case <-time.After(5 * time.Second):
		c.t.Fatalf("timed out waiting for server exit")
		return -1
	}
}

func (c *testClient) initialize(encodings []string) InitializeResult {
	c.t.Helper()
	var caps map[string]any
	if encodings != nil {
		caps = map[string]any{
			"general": map[string]any{
				"positionEncodings": encodings,
			},
		}
	} else {
		caps = map[string]any{}
	}
	id := c.request("initialize", map[string]any{"capabilities": caps})
	resp := c.readResponse(id)
	if resp.Error != nil {
		c.t.Fatalf("initialize error: %+v", resp.Error)
	}
	var res InitializeResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		c.t.Fatalf("unmarshal InitializeResult: %v", err)
	}
	c.notify("initialized", map[string]any{})
	return res
}

func TestInitializeNegotiationAndPositions(t *testing.T) {
	// Line with non-ASCII character (é = 2 UTF-8 bytes, 1 UTF-16 code unit; 😀 = 4 UTF-8 bytes, 2 UTF-16 code units).
	// "//h:desc café 😀\n//h:method BAD\n" -> line 0 has non-ASCII, line 1 has error;
	// Also test a diagnostic ON a non-ASCII line itself: "//h:auth café😀" (unsupported auth value).
	src := "package admin\n\nimport \"net/http\"\n\ntype H struct{}\n\n//h:key k1\n//h:method GET\n//h:path /a\n//h:desc A\n//h:auth café😀\nfunc (h *H) A(w http.ResponseWriter, r *http.Request) error { return nil }\n"

	dir := t.TempDir()
	filePath := filepath.Join(dir, "handlers.go")
	if err := os.WriteFile(filePath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := pathToURI(filePath)

	line10 := "//h:auth café😀"
	wantUTF8End := len(line10)
	wantUTF16End := utf16Len(line10)

	t.Run("utf-8", func(t *testing.T) {
		c := startTestServer(t)
		res := c.initialize([]string{"utf-8", "utf-16"})
		if res.Capabilities.PositionEncoding != "utf-8" {
			t.Fatalf("positionEncoding = %q, want utf-8", res.Capabilities.PositionEncoding)
		}
		if res.Capabilities.TextDocumentSync != syncFull {
			t.Fatalf("textDocumentSync = %d, want %d", res.Capabilities.TextDocumentSync, syncFull)
		}
		if res.Capabilities.CompletionProvider == nil || !slices.Equal(res.Capabilities.CompletionProvider.TriggerCharacters, []string{":", " "}) {
			t.Fatalf("completionProvider = %+v", res.Capabilities.CompletionProvider)
		}
		if !res.Capabilities.HoverProvider || !res.Capabilities.DefinitionProvider {
			t.Fatalf("hover/definition = %v/%v", res.Capabilities.HoverProvider, res.Capabilities.DefinitionProvider)
		}

		c.notify("textDocument/didOpen", DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{URI: uri, LanguageID: "go", Version: 1, Text: src},
		})
		pub := c.readDiagnosticsFor(uri)
		if len(pub.Diagnostics) != 1 {
			t.Fatalf("got %d diagnostics, want 1: %+v", len(pub.Diagnostics), pub.Diagnostics)
		}
		d := pub.Diagnostics[0]
		if d.Severity != severityError || d.Source != "handlers" {
			t.Errorf("severity/source = %d/%q", d.Severity, d.Source)
		}
		if d.Range.Start.Line != 10 || d.Range.Start.Character != 0 || d.Range.End.Line != 10 || d.Range.End.Character != wantUTF8End {
			t.Errorf("utf-8 range = %+v, want line 10 [0..%d]", d.Range, wantUTF8End)
		}

		sid := c.request("shutdown", nil)
		_ = c.readResponse(sid)
		c.notify("exit", nil)
		if code := c.waitExit(); code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("utf-16_fallback", func(t *testing.T) {
		c := startTestServer(t)
		res := c.initialize([]string{"utf-16"})
		if res.Capabilities.PositionEncoding != "utf-16" {
			t.Fatalf("positionEncoding = %q, want utf-16", res.Capabilities.PositionEncoding)
		}

		c.notify("textDocument/didOpen", DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{URI: uri, LanguageID: "go", Version: 1, Text: src},
		})
		pub := c.readDiagnosticsFor(uri)
		if len(pub.Diagnostics) != 1 {
			t.Fatalf("got %d diagnostics, want 1: %+v", len(pub.Diagnostics), pub.Diagnostics)
		}
		d := pub.Diagnostics[0]
		if d.Range.Start.Line != 10 || d.Range.Start.Character != 0 || d.Range.End.Line != 10 || d.Range.End.Character != wantUTF16End {
			t.Errorf("utf-16 range = %+v, want line 10 [0..%d]", d.Range, wantUTF16End)
		}

		sid := c.request("shutdown", nil)
		_ = c.readResponse(sid)
		c.notify("exit", nil)
		if code := c.waitExit(); code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})
}

func TestShutdownExitAndMethodNotFound(t *testing.T) {
	t.Run("exit_without_shutdown", func(t *testing.T) {
		c := startTestServer(t)
		_ = c.initialize(nil)
		c.notify("exit", nil)
		if code := c.waitExit(); code != 1 {
			t.Errorf("exit without shutdown code = %d, want 1", code)
		}
	})

	t.Run("method_not_found_and_request_after_shutdown", func(t *testing.T) {
		c := startTestServer(t)
		_ = c.initialize([]string{"utf-8"})

		id1 := c.request("workspace/unknownMethod", map[string]any{})
		r1 := c.readResponse(id1)
		if r1.Error == nil || r1.Error.Code != codeMethodNotFound {
			t.Fatalf("unknown method error = %+v, want code %d", r1.Error, codeMethodNotFound)
		}

		// Unknown notification produces no response.
		c.notify("workspace/unknownNotify", map[string]any{})

		sid := c.request("shutdown", nil)
		sresp := c.readResponse(sid)
		if sresp.Error != nil || string(sresp.Result) != "null" {
			t.Fatalf("shutdown response = error:%+v result:%s", sresp.Error, sresp.Result)
		}

		// Requests after shutdown must be refused with InvalidRequest.
		id2 := c.request("textDocument/hover", map[string]any{})
		r2 := c.readResponse(id2)
		if r2.Error == nil || r2.Error.Code != codeInvalidRequest {
			t.Fatalf("request after shutdown = %+v, want code %d", r2.Error, codeInvalidRequest)
		}

		c.notify("exit", nil)
		if code := c.waitExit(); code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})
}

func TestDiagnosticsOpenChangeCloseAndDisappear(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	testFile := filepath.Join(dir, "a_test.go")

	srcA := `package admin

import "net/http"

type H struct{}

//h:key users.list
//h:method GET
//h:path /users
//h:desc Users
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }
`
	srcB := `package admin

import "net/http"

//h:key users.list
//h:method GET
//h:path /users2
//h:desc Users 2
func (h *H) List2(w http.ResponseWriter, r *http.Request) error { return nil }
`
	// _test.go with invalid directive should be ignored when choosing files.
	srcTest := `package admin
//h:bogus bad
`
	if err := os.WriteFile(fileA, []byte(srcA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte(srcB), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte(srcTest), 0o644); err != nil {
		t.Fatal(err)
	}

	uriA := pathToURI(fileA)
	uriB := pathToURI(fileB)

	c := startTestServer(t)
	_ = c.initialize([]string{"utf-8"})

	// Open only a.go: b.go is on disk with duplicate key, so b.go gets diagnostic in syntax pass,
	// ONLY a.go is open -> only a.go gets publishDiagnostics (with empty list, because b.go has the dup line!).
	c.notify("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uriA, LanguageID: "go", Version: 1, Text: srcA},
	})
	pubA := c.readDiagnosticsFor(uriA)
	if len(pubA.Diagnostics) != 0 {
		t.Fatalf("a.go diagnostics = %+v, want empty (dup reported on b.go, which is unopened)", pubA.Diagnostics)
	}

	// Now open b.go: b.go is open, so it receives the duplicate-key diagnostic.
	c.notify("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uriB, LanguageID: "go", Version: 1, Text: srcB},
	})
	pubB := c.readDiagnosticsFor(uriB)
	if len(pubB.Diagnostics) != 1 || !strings.Contains(pubB.Diagnostics[0].Message, `key "users.list" also at a.go:7`) {
		t.Fatalf("b.go diagnostics = %+v, want duplicate key error", pubB.Diagnostics)
	}

	// didChange batch: last change wins (fixes duplicate key, adds missing h:desc warning).
	fixedB := `package admin

import "net/http"

//h:key users.edit
//h:method GET
//h:path /users2
func (h *H) List2(w http.ResponseWriter, r *http.Request) error { return nil }
`
	c.notify("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uriB, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: "//h:broken"},
			{Text: fixedB},
		},
	})
	pubB = c.readDiagnosticsFor(uriB)
	if len(pubB.Diagnostics) != 1 || pubB.Diagnostics[0].Severity != severityWarning || !strings.Contains(pubB.Diagnostics[0].Message, "missing h:desc") {
		t.Fatalf("b.go after batch didChange = %+v, want missing h:desc warning", pubB.Diagnostics)
	}

	// Fix warning too -> diagnostics cleared (empty slice).
	cleanB := strings.Replace(fixedB, "//h:path /users2", "//h:path /users2\n//h:desc Edit", 1)
	c.notify("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uriB, Version: 3},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: cleanB}},
	})
	pubB = c.readDiagnosticsFor(uriB)
	if len(pubB.Diagnostics) != 0 {
		t.Fatalf("b.go after fix = %+v, want empty", pubB.Diagnostics)
	}

	// Introduce an error in a.go via didChange, then close a.go -> diagnostics cleared on close.
	badA := strings.Replace(srcA, "//h:method GET", "//h:method NOPE", 1)
	c.notify("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uriA, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: badA}},
	})
	pubA = c.readDiagnosticsFor(uriA)
	if len(pubA.Diagnostics) != 1 {
		t.Fatalf("a.go error = %+v, want 1", pubA.Diagnostics)
	}

	c.notify("textDocument/didClose", DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uriA},
	})
	pubA = c.readDiagnosticsFor(uriA)
	if len(pubA.Diagnostics) != 0 {
		t.Fatalf("a.go on close = %+v, want cleared []", pubA.Diagnostics)
	}
}

func TestTypePassOnSaveAndStaleDrop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/lsptest\n\ngo 1.26.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(dir, "handlers.go")
	// Syntax-valid, but {rest...} requires string, not int64 (caught only by type pass on save!).
	srcErr := `package admin

import "net/http"

type H struct{}

//h:key files.get
//h:method GET
//h:path /files/{rest...}
//h:desc Files
func (h *H) Get(w http.ResponseWriter, r *http.Request, rest int64) error { return nil }
`
	if err := os.WriteFile(filePath, []byte(srcErr), 0o644); err != nil {
		t.Fatal(err)
	}
	uri := pathToURI(filePath)

	c := startTestServer(t)
	_ = c.initialize([]string{"utf-8"})

	// Open -> syntax pass only -> 0 diagnostics.
	c.notify("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "go", Version: 1, Text: srcErr},
	})
	pub := c.readDiagnosticsFor(uri)
	if len(pub.Diagnostics) != 0 {
		t.Fatalf("didOpen diagnostics = %+v, want 0 (syntax pass only)", pub.Diagnostics)
	}

	// Save -> type pass runs on worker and publishes the type error.
	c.notify("textDocument/didSave", DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	pub = c.readDiagnosticsFor(uri)
	if len(pub.Diagnostics) != 1 || !strings.Contains(pub.Diagnostics[0].Message, "wildcard needs a string type") {
		t.Fatalf("didSave diagnostics = %+v, want type error", pub.Diagnostics)
	}

	// Trigger didSave then immediately didChange: stale type pass result must NOT overwrite the didChange diagnostics.
	c.notify("textDocument/didSave", DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	fixedSrc := strings.Replace(srcErr, "rest int64", "rest string", 1)
	fixedSrc = strings.Replace(fixedSrc, "//h:method GET", "//h:method BAD", 1)
	c.notify("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: fixedSrc}},
	})
	pub = c.readDiagnosticsFor(uri)
	if len(pub.Diagnostics) != 1 || !strings.Contains(pub.Diagnostics[0].Message, "h:method:") {
		t.Fatalf("didChange diagnostics = %+v, want h:method error", pub.Diagnostics)
	}
	// Send a hover request to ensure server finishes processing any pending typeResCh messages without sending stale diagnostics.
	hid := c.request("textDocument/hover", HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 6, Character: 3},
	})
	// Read until hover response; verify no unexpected diagnostics with "wildcard needs a string type" arrive.
	for {
		m := c.readMsg()
		if m.Method == "textDocument/publishDiagnostics" {
			var p PublishDiagnosticsParams
			_ = json.Unmarshal(m.Params, &p)
			for _, d := range p.Diagnostics {
				if strings.Contains(d.Message, "wildcard needs a string type") {
					t.Fatalf("received stale type-pass diagnostic after didChange: %+v", p)
				}
			}
			continue
		}
		if m.ID != nil {
			var gotID int
			_ = json.Unmarshal(*m.ID, &gotID)
			if gotID == hid {
				break
			}
		}
	}
}

func TestCompletionHoverAndDefinition(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")

	srcA := `package admin

import "net/http"

type H struct{}

//h:key categories.list
//h:method GET
//h:path /categories
//h:desc Categories
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }

//h:
//h:method P
//h:auth s
//h:view t
//h:param 
//h:param id from categories.
//h:param id from categories.list
//h:body 
//h:body 20
//h:body 20MiB
func (h *H) Detail(w http.ResponseWriter, r *http.Request, id int64, slug string) error { return nil }
`
	srcB := `package admin

import "net/http"

//h:key orders.show
//h:method GET
//h:path /orders/{id}
//h:desc Show order
func (h *H) ShowOrder(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

//h:key orders.list
//h:method GET
//h:path /orders
//h:desc Orders
func (h *H) ListOrders(w http.ResponseWriter, r *http.Request) error { return nil }
`
	if err := os.WriteFile(fileA, []byte(srcA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte(srcB), 0o644); err != nil {
		t.Fatal(err)
	}

	uriA := pathToURI(fileA)
	c := startTestServer(t)
	_ = c.initialize([]string{"utf-8"})

	c.notify("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uriA, LanguageID: "go", Version: 1, Text: srcA},
	})
	_ = c.readDiagnosticsFor(uriA)

	completeAt := func(line, char int) []string {
		t.Helper()
		id := c.request("textDocument/completion", CompletionParams{
			TextDocument: TextDocumentIdentifier{URI: uriA},
			Position:     Position{Line: line, Character: char},
		})
		resp := c.readResponse(id)
		var list CompletionList
		if err := json.Unmarshal(resp.Result, &list); err != nil {
			t.Fatalf("unmarshal completion: %v", err)
		}
		var labels []string
		for _, it := range list.Items {
			labels = append(labels, it.Label)
		}
		return labels
	}

	// Line 12: "//h:"
	if got := completeAt(12, 4); len(got) != len(directiveNames) {
		t.Errorf("line 12 completion = %v, want %d directive names", got, len(directiveNames))
	}
	// Line 13: "//h:method P"
	if got := completeAt(13, 12); !slices.Equal(got, []string{"POST", "PUT", "PATCH"}) {
		t.Errorf("line 13 method completion = %v, want [POST PUT PATCH]", got)
	}
	// Line 14: "//h:auth s"
	if got := completeAt(14, 10); !slices.Equal(got, []string{"session"}) {
		t.Errorf("line 14 auth completion = %v, want [session]", got)
	}
	// Line 15: "//h:view t"
	if got := completeAt(15, 10); !slices.Equal(got, []string{"true"}) {
		t.Errorf("line 15 view completion = %v, want [true]", got)
	}
	// Line 16: "//h:param "
	if got := completeAt(16, 10); !slices.Equal(got, []string{"id", "slug"}) {
		t.Errorf("line 16 param completion = %v, want [id slug]", got)
	}
	// Line 17: "//h:param id from categories."
	if got := completeAt(17, 29); !slices.Equal(got, []string{"categories.list"}) {
		t.Errorf("line 17 from completion = %v, want [categories.list]", got)
	}
	// Line 17 with empty prefix ("//h:param id from ") -> the GET routes without
	// parameters, b.go's orders.list included and orders.show (it has {id}) left out
	if got := completeAt(17, 18); !slices.Equal(got, []string{"categories.list", "orders.list"}) {
		t.Errorf("line 17 empty from completion = %v, want [categories.list orders.list]", got)
	}
	// Line 17 after the name ("//h:param id ") -> the two keywords
	if got := completeAt(17, 13); !slices.Equal(got, []string{"from", "label"}) {
		t.Errorf("line 17 keyword completion = %v, want [from label]", got)
	}
	// Line 17 inside the keyword ("//h:param id fr") -> from
	if got := completeAt(17, 15); !slices.Equal(got, []string{"from"}) {
		t.Errorf("line 17 partial keyword completion = %v, want [from]", got)
	}
	// Line 19: "//h:body "
	if got := completeAt(19, 9); !slices.Equal(got, []string{"512B", "64KiB", "20MiB", "1GiB"}) {
		t.Errorf("line 19 empty body completion = %v, want [512B 64KiB 20MiB 1GiB]", got)
	}
	// Line 20: "//h:body 20"
	if got := completeAt(20, 11); !slices.Equal(got, []string{"20B", "20KiB", "20MiB", "20GiB"}) {
		t.Errorf("line 20 numeric body completion = %v, want [20B 20KiB 20MiB 20GiB]", got)
	}

	// Hover on "//h:body 20MiB" (line 21) -> shows bytes grouped by three with spaces.
	hidBody := c.request("textDocument/hover", HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 21, Character: 4},
	})
	hrespBody := c.readResponse(hidBody)
	var hovBody Hover
	if err := json.Unmarshal(hrespBody.Result, &hovBody); err != nil {
		t.Fatalf("unmarshal body hover: %v", err)
	}
	if !strings.Contains(hovBody.Contents.Value, "20MiB = 20 971 520 bytes") {
		t.Errorf("body hover = %q, want substring %q", hovBody.Contents.Value, "20MiB = 20 971 520 bytes")
	}

	// Hover on "//h:key categories.list" (line 6)
	hid := c.request("textDocument/hover", HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 6, Character: 4},
	})
	hresp := c.readResponse(hid)
	var hov Hover
	if err := json.Unmarshal(hresp.Result, &hov); err != nil {
		t.Fatalf("unmarshal hover: %v", err)
	}
	if !strings.Contains(hov.Contents.Value, "h:key") || strings.Contains(hov.Contents.Value, "\n") {
		t.Errorf("hover = %q, want one line for h:key", hov.Contents.Value)
	}

	// Hover on ordinary Go code returns null.
	hid2 := c.request("textDocument/hover", HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 0, Character: 2},
	})
	hresp2 := c.readResponse(hid2)
	if string(hresp2.Result) != "null" {
		t.Errorf("hover on package line = %s, want null", hresp2.Result)
	}

	// Definition on "//h:param id from categories.list" (line 18) -> func List at line 10.
	did := c.request("textDocument/definition", DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 18, Character: 20},
	})
	dresp := c.readResponse(did)
	var loc Location
	if err := json.Unmarshal(dresp.Result, &loc); err != nil {
		t.Fatalf("unmarshal definition: %v", err)
	}
	if loc.URI != uriA || loc.Range.Start.Line != 10 {
		t.Errorf("definition = %+v, want uri %s line 10", loc, uriA)
	}

	// Definition on a cross-file source (orders.show in b.go) and an unknown one (returns null).
	crossSrc := strings.Replace(srcA, "//h:param id from categories.list", "//h:param id from orders.show", 1)
	c.notify("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uriA, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: crossSrc}},
	})
	_ = c.readDiagnosticsFor(uriA)

	did2 := c.request("textDocument/definition", DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 18, Character: 20},
	})
	dresp2 := c.readResponse(did2)
	var loc2 Location
	if err := json.Unmarshal(dresp2.Result, &loc2); err != nil {
		t.Fatalf("unmarshal cross-file definition: %v", err)
	}
	uriB := pathToURI(fileB)
	if loc2.URI != uriB || loc2.Range.Start.Line != 8 {
		t.Errorf("cross-file definition = %+v, want uri %s line 8", loc2, uriB)
	}

	// An unknown source returns null.
	unknownSrc := strings.Replace(crossSrc, "//h:param id from orders.show", "//h:param id from missing.key", 1)
	c.notify("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uriA, Version: 3},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: unknownSrc}},
	})
	_ = c.readDiagnosticsFor(uriA)

	did3 := c.request("textDocument/definition", DefinitionParams{
		TextDocument: TextDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 18, Character: 20},
	})
	dresp3 := c.readResponse(did3)
	if string(dresp3.Result) != "null" {
		t.Errorf("unknown source definition = %s, want null", dresp3.Result)
	}
}

func TestFileCacheMtimeAndBuildConstraints(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.go")
	fileB := filepath.Join(dir, "b.go")
	fileIgnored := filepath.Join(dir, "ignored.go")

	srcA := `package admin

import "net/http"

type H struct{}

//h:key a.route
//h:method GET
//h:path /a
//h:desc A
func (h *H) A(w http.ResponseWriter, r *http.Request) error { return nil }
`
	srcB := `package admin

import "net/http"

//h:key b.route
//h:method GET
//h:path /b
//h:desc B
func (h *H) B(w http.ResponseWriter, r *http.Request) error { return nil }
`
	srcIgnored := `//go:build ignore

package admin
//h:bad ignored
`
	if err := os.WriteFile(fileA, []byte(srcA), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte(srcB), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileIgnored, []byte(srcIgnored), 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewServer(strings.NewReader(""), io.Discard)
	uriA := pathToURI(fileA)
	s.onDidOpen(DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uriA, LanguageID: "go", Version: 1, Text: srcA},
	})

	cleanB := filepath.Clean(fileB)
	cached1, ok := s.fileCache[cleanB]
	if !ok {
		t.Fatalf("expected b.go to be cached")
	}

	// Overwrite b.go with exact same mtime and size -> cache must NOT reread from disk.
	sameLenB := strings.Replace(srcB, "b.route", "c.route", 1)
	if len(sameLenB) != len(srcB) {
		t.Fatalf("test setup length mismatch")
	}
	if err := os.WriteFile(fileB, []byte(sameLenB), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fileB, cached1.mtime, cached1.mtime); err != nil {
		t.Fatal(err)
	}

	s.onDidChange(DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uriA, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: srcA}},
	})
	if string(s.fileCache[cleanB].content) != srcB {
		t.Fatalf("expected b.go to be served from cache when mtime/size unchanged")
	}

	// Bump mtime -> cache MUST reread b.go.
	newTime := cached1.mtime.Add(2 * time.Second)
	if err := os.Chtimes(fileB, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	s.onDidChange(DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uriA, Version: 3},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: srcA}},
	})
	if string(s.fileCache[cleanB].content) != sameLenB {
		t.Fatalf("expected b.go to be reread when mtime changed")
	}

	// Verify ignored.go was excluded by build constraints.
	ps := s.pkgStates[pkgKey{dir: filepath.Clean(dir), pkgName: "admin"}]
	if ps == nil || len(ps.syntaxDiags) != 0 {
		t.Fatalf("expected ignored.go to be excluded, got diags: %+v", ps.syntaxDiags)
	}

	// File with a different package name in the same directory must also be excluded.
	fileOther := filepath.Join(dir, "other.go")
	if err := os.WriteFile(fileOther, []byte("package other\n//h:bad otherpkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.onDidChange(DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uriA, Version: 4},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: srcA}},
	})
	if len(ps.syntaxDiags) != 0 {
		t.Fatalf("expected other.go (package other) to be excluded, got diags: %+v", ps.syntaxDiags)
	}

	// Remove b.go from disk -> cache entry pruned and duplicate key cleared if a.go had one.
	if err := os.Remove(fileB); err != nil {
		t.Fatal(err)
	}
	s.onDidChange(DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uriA, Version: 5},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: srcA}},
	})
	if _, stillCached := s.fileCache[cleanB]; stillCached {
		t.Fatalf("expected removed b.go to be pruned from fileCache")
	}
}

func TestPortableFileURIs(t *testing.T) {
	cases := []struct {
		goos string
		path string
		uri  string
	}{
		{"linux", "/home/user/my project/handlers.go", "file:///home/user/my%20project/handlers.go"},
		{"windows", `C:\Users\dev\my project\handlers.go`, "file:///C:/Users/dev/my%20project/handlers.go"},
		{"windows", `\\server\share\dir\handlers.go`, "file://server/share/dir/handlers.go"},
	}
	for _, tc := range cases {
		gotURI := fsPathToURI(tc.path, tc.goos)
		if gotURI != tc.uri {
			t.Errorf("fsPathToURI(%q, %s) = %q, want %q", tc.path, tc.goos, gotURI, tc.uri)
		}
		gotPath, ok := uriToFSPath(tc.uri, tc.goos)
		if !ok || gotPath != tc.path {
			t.Errorf("uriToFSPath(%q, %s) = (%q, %v), want (%q, true)", tc.uri, tc.goos, gotPath, ok, tc.path)
		}
	}
	// Windows lowercase percent-encoded colon
	if p, ok := uriToFSPath("file:///c%3A/repo/a.go", "windows"); !ok || p != `c:\repo\a.go` {
		t.Errorf("percent-encoded drive = (%q, %v)", p, ok)
	}
	if _, ok := uriToFSPath("untitled:Untitled-1", "linux"); ok {
		t.Errorf("non-file URI should return false")
	}
}
