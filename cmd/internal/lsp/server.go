package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/epistemer/handlers/cmd/internal/directive"
)

type readEvent struct {
	raw []byte
	err error
}

type typePassJob struct {
	pkgKey pkgKey
	dir    string
	gen    uint64
	pkg    *directive.Package
}

type typePassResult struct {
	pkgKey pkgKey
	gen    uint64
	diags  []directive.Diagnostic
	err    error
}

type pkgKey struct {
	dir     string
	pkgName string
}

type openDoc struct {
	uri      string
	path     string
	realPath string
	dir      string
	pkgName  string
	content  []byte
	lines    []string
}

type cachedFile struct {
	mtime   time.Time
	size    int64
	content []byte
	pkgName string
}

type pkgState struct {
	gen         uint64
	pkg         *directive.Package
	syntaxDiags []directive.Diagnostic
	typeDiags   []directive.Diagnostic
}

// Server implements the handlers language server over stdin/stdout JSON-RPC.
type Server struct {
	in  io.Reader
	out io.Writer

	encoding    string
	initialized bool
	shutdown    bool

	openDocs  map[string]*openDoc   // keyed by URI
	fileCache map[string]cachedFile // keyed by cleaned path
	pkgStates map[pkgKey]*pkgState  // keyed by (dir, pkgName)

	// Type-pass worker coordination (never touches document state).
	workerMu      sync.Mutex
	workerCond    *sync.Cond
	workerPending map[pkgKey]typePassJob
	workerOrder   []pkgKey
	workerClosed  bool
	typeResCh     chan typePassResult
	doneCh        chan struct{}
	workerWG      sync.WaitGroup
}

// Run runs the language server on in and out and returns the process exit code.
func Run(in io.Reader, out io.Writer) int {
	return NewServer(in, out).Serve()
}

// NewServer constructs a language server reading from in and writing to out.
func NewServer(in io.Reader, out io.Writer) *Server {
	s := &Server{
		in:            in,
		out:           out,
		encoding:      encUTF16,
		openDocs:      make(map[string]*openDoc),
		fileCache:     make(map[string]cachedFile),
		pkgStates:     make(map[pkgKey]*pkgState),
		workerPending: make(map[pkgKey]typePassJob),
		typeResCh:     make(chan typePassResult, 8),
		doneCh:        make(chan struct{}),
	}
	s.workerCond = sync.NewCond(&s.workerMu)
	return s
}

// Serve runs the single-goroutine message loop until exit or EOF.
func (s *Server) Serve() int {
	readCh := make(chan readEvent, 8)
	go s.readLoop(readCh)

	s.workerWG.Add(1)
	go s.typeWorker()

	defer func() {
		close(s.doneCh)
		s.workerMu.Lock()
		s.workerClosed = true
		s.workerCond.Broadcast()
		s.workerMu.Unlock()
		s.workerWG.Wait()
	}()

	for {
		select {
		case ev, ok := <-readCh:
			if !ok {
				if s.shutdown {
					return 0
				}
				return 1
			}
			if ev.err != nil {
				if errors.Is(ev.err, io.EOF) {
					if s.shutdown {
						return 0
					}
					return 1
				}
				// Framing/parse error: continue if possible, exit 1 on fatal read error.
				if errors.Is(ev.err, errInvalidHeader) {
					continue
				}
				return 1
			}
			done, code := s.handleRawMessage(ev.raw)
			if done {
				return code
			}

		case res := <-s.typeResCh:
			s.handleTypePassResult(res)
		}
	}
}

func (s *Server) readLoop(ch chan<- readEvent) {
	defer close(ch)
	br := bufio.NewReader(s.in)
	for {
		body, err := readFrame(br)
		if err != nil {
			ch <- readEvent{err: err}
			return
		}
		ch <- readEvent{raw: body}
	}
}

func (s *Server) typeWorker() {
	defer s.workerWG.Done()
	for {
		s.workerMu.Lock()
		for len(s.workerOrder) == 0 && !s.workerClosed {
			s.workerCond.Wait()
		}
		if s.workerClosed {
			s.workerMu.Unlock()
			return
		}
		key := s.workerOrder[0]
		s.workerOrder = s.workerOrder[1:]
		job := s.workerPending[key]
		delete(s.workerPending, key)
		s.workerMu.Unlock()

		diags, err := directive.Types(job.dir, job.pkg)
		select {
		case s.typeResCh <- typePassResult{
			pkgKey: job.pkgKey,
			gen:    job.gen,
			diags:  diags,
			err:    err,
		}:
		case <-s.doneCh:
			return
		}
	}
}

func (s *Server) enqueueTypePass(job typePassJob) {
	s.workerMu.Lock()
	if _, exists := s.workerPending[job.pkgKey]; !exists {
		s.workerOrder = append(s.workerOrder, job.pkgKey)
	}
	s.workerPending[job.pkgKey] = job
	s.workerCond.Signal()
	s.workerMu.Unlock()
}

func (s *Server) handleRawMessage(raw []byte) (exit bool, code int) {
	var msg rpcMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		s.sendError(nil, codeParseError, "parse error")
		return false, 0
	}

	isRequest := msg.ID != nil && string(*msg.ID) != "null"

	if msg.Method == "exit" {
		if s.shutdown {
			return true, 0
		}
		return true, 1
	}

	if s.shutdown {
		if isRequest {
			s.sendError(msg.ID, codeInvalidRequest, "server is shutting down")
		}
		return false, 0
	}

	if !s.initialized && msg.Method != "initialize" {
		if isRequest {
			s.sendError(msg.ID, codeServerNotInit, "server not initialized")
		}
		return false, 0
	}

	switch msg.Method {
	case "initialize":
		var params InitializeParams
		if len(msg.Params) > 0 {
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				if isRequest {
					s.sendError(msg.ID, codeInvalidParams, "invalid initialize params")
				}
				return false, 0
			}
		}
		s.encoding = negotiateEncoding(params)
		s.initialized = true
		res := InitializeResult{
			Capabilities: ServerCapabilities{
				PositionEncoding: s.encoding,
				TextDocumentSync: syncFull,
				CompletionProvider: &CompletionOptions{
					TriggerCharacters: []string{":", " "},
				},
				HoverProvider:      true,
				DefinitionProvider: true,
			},
			ServerInfo: &ServerInfo{Name: "handlers"},
		}
		if isRequest {
			s.sendResult(msg.ID, res)
		}

	case "initialized":
		// Notification, no-op.

	case "shutdown":
		s.shutdown = true
		if isRequest {
			s.sendResult(msg.ID, nil)
		}

	case "textDocument/didOpen":
		var params DidOpenTextDocumentParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return false, 0
		}
		s.onDidOpen(params)

	case "textDocument/didChange":
		var params DidChangeTextDocumentParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return false, 0
		}
		s.onDidChange(params)

	case "textDocument/didSave":
		var params DidSaveTextDocumentParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return false, 0
		}
		s.onDidSave(params)

	case "textDocument/didClose":
		var params DidCloseTextDocumentParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return false, 0
		}
		s.onDidClose(params)

	case "textDocument/completion":
		var params CompletionParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			if isRequest {
				s.sendError(msg.ID, codeInvalidParams, "invalid completion params")
			}
			return false, 0
		}
		doc := s.openDocs[params.TextDocument.URI]
		var routes []directive.Route
		if doc != nil {
			if ps := s.pkgStates[pkgKey{dir: doc.dir, pkgName: doc.pkgName}]; ps != nil && ps.pkg != nil {
				routes = ps.pkg.Routes
			}
		}
		list := completeDocument(doc, routes, params.Position, s.encoding)
		if isRequest {
			s.sendResult(msg.ID, list)
		}

	case "textDocument/hover":
		var params HoverParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			if isRequest {
				s.sendError(msg.ID, codeInvalidParams, "invalid hover params")
			}
			return false, 0
		}
		doc := s.openDocs[params.TextDocument.URI]
		hover := hoverDocument(doc, params.Position, s.encoding)
		if isRequest {
			s.sendResult(msg.ID, hover)
		}

	case "textDocument/definition":
		var params DefinitionParams
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			if isRequest {
				s.sendError(msg.ID, codeInvalidParams, "invalid definition params")
			}
			return false, 0
		}
		doc := s.openDocs[params.TextDocument.URI]
		var routes []directive.Route
		if doc != nil {
			if ps := s.pkgStates[pkgKey{dir: doc.dir, pkgName: doc.pkgName}]; ps != nil && ps.pkg != nil {
				routes = ps.pkg.Routes
			}
		}
		loc := definitionForEntity(doc, routes, params.Position, s.encoding, s.linesForFile)
		if isRequest {
			s.sendResult(msg.ID, loc)
		}

	default:
		if isRequest {
			s.sendError(msg.ID, codeMethodNotFound, "method not found: "+msg.Method)
		}
	}

	return false, 0
}

func (s *Server) onDidOpen(params DidOpenTextDocumentParams) {
	path, ok := uriToPath(params.TextDocument.URI)
	if !ok {
		return
	}
	content := []byte(params.TextDocument.Text)
	doc := &openDoc{
		uri:      params.TextDocument.URI,
		path:     path,
		realPath: resolveRealPath(path),
		dir:      filepath.Dir(path),
		pkgName:  parsePkgName(path, content),
		content:  content,
		lines:    splitLines(content),
	}
	s.openDocs[doc.uri] = doc

	key := pkgKey{dir: doc.dir, pkgName: doc.pkgName}
	ps := s.ensurePkgState(key)
	ps.gen++
	ps.typeDiags = nil
	s.runSyntaxForPackage(key, ps)
	s.publishForPackage(key, ps)
}

func (s *Server) onDidChange(params DidChangeTextDocumentParams) {
	doc, ok := s.openDocs[params.TextDocument.URI]
	if !ok || len(params.ContentChanges) == 0 {
		return
	}
	// Full document sync: use the last change of a batch only.
	last := params.ContentChanges[len(params.ContentChanges)-1]
	oldKey := pkgKey{dir: doc.dir, pkgName: doc.pkgName}

	content := []byte(last.Text)
	doc.content = content
	doc.lines = splitLines(content)
	doc.pkgName = parsePkgName(doc.path, content)

	newKey := pkgKey{dir: doc.dir, pkgName: doc.pkgName}
	if oldKey != newKey {
		if oldPS := s.pkgStates[oldKey]; oldPS != nil {
			oldPS.gen++
			oldPS.typeDiags = nil
			s.runSyntaxForPackage(oldKey, oldPS)
			s.publishForPackage(oldKey, oldPS)
		}
	}

	ps := s.ensurePkgState(newKey)
	ps.gen++
	ps.typeDiags = nil
	s.runSyntaxForPackage(newKey, ps)
	s.publishForPackage(newKey, ps)
}

func (s *Server) onDidSave(params DidSaveTextDocumentParams) {
	doc, ok := s.openDocs[params.TextDocument.URI]
	if !ok {
		return
	}
	if params.Text != nil {
		content := []byte(*params.Text)
		doc.content = content
		doc.lines = splitLines(content)
		doc.pkgName = parsePkgName(doc.path, content)
	}
	// Refresh realPath in case file was just created on disk.
	doc.realPath = resolveRealPath(doc.path)

	key := pkgKey{dir: doc.dir, pkgName: doc.pkgName}
	ps := s.ensurePkgState(key)
	ps.gen++
	ps.typeDiags = nil
	s.runSyntaxForPackage(key, ps)

	if ps.pkg != nil && len(ps.pkg.Routes) > 0 {
		s.enqueueTypePass(typePassJob{
			pkgKey: key,
			dir:    doc.dir,
			gen:    ps.gen,
			pkg:    ps.pkg,
		})
		return
	}
	s.publishForPackage(key, ps)
}

func (s *Server) onDidClose(params DidCloseTextDocumentParams) {
	doc, ok := s.openDocs[params.TextDocument.URI]
	if !ok {
		return
	}
	delete(s.openDocs, params.TextDocument.URI)
	// Clear diagnostics for the closed document.
	s.publishDiagnostics(doc.uri, []Diagnostic{})

	key := pkgKey{dir: doc.dir, pkgName: doc.pkgName}
	if s.hasOpenDocInPkg(key) {
		ps := s.ensurePkgState(key)
		ps.gen++
		ps.typeDiags = nil
		s.runSyntaxForPackage(key, ps)
		s.publishForPackage(key, ps)
	} else {
		delete(s.pkgStates, key)
	}
}

func (s *Server) handleTypePassResult(res typePassResult) {
	ps, ok := s.pkgStates[res.pkgKey]
	if !ok || ps.gen != res.gen {
		// Stale result from before a change/save/close: drop it.
		return
	}
	if res.err == nil {
		ps.typeDiags = res.diags
	} else {
		ps.typeDiags = nil
	}
	s.publishForPackage(res.pkgKey, ps)
}

func (s *Server) ensurePkgState(key pkgKey) *pkgState {
	ps, ok := s.pkgStates[key]
	if !ok {
		ps = &pkgState{}
		s.pkgStates[key] = ps
	}
	return ps
}

func (s *Server) hasOpenDocInPkg(key pkgKey) bool {
	for _, doc := range s.openDocs {
		if doc.dir == key.dir && doc.pkgName == key.pkgName {
			return true
		}
	}
	return false
}

func (s *Server) runSyntaxForPackage(key pkgKey, ps *pkgState) {
	files := s.collectPackageFiles(key)
	if len(files) == 0 {
		ps.pkg = nil
		ps.syntaxDiags = nil
		return
	}
	ps.pkg, ps.syntaxDiags = directive.Syntax(files)
}

func (s *Server) collectPackageFiles(key pkgKey) map[string][]byte {
	dir := key.dir
	memFiles := make(map[string][]byte)
	diskNames := make(map[string]bool)

	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			fullPath := filepath.Clean(filepath.Join(dir, name))
			diskNames[fullPath] = true

			// If open in editor, the open buffer overrides disk.
			if doc := s.findOpenDocByPath(fullPath); doc != nil {
				memFiles[fullPath] = doc.content
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}
			cached, ok := s.fileCache[fullPath]
			if !ok || !cached.mtime.Equal(info.ModTime()) || cached.size != info.Size() {
				b, err := os.ReadFile(fullPath)
				if err != nil {
					continue
				}
				cached = cachedFile{
					mtime:   info.ModTime(),
					size:    info.Size(),
					content: b,
					pkgName: parsePkgName(fullPath, b),
				}
				s.fileCache[fullPath] = cached
			}
			memFiles[fullPath] = cached.content
		}
	}

	// Prune disappeared disk files from cache for this directory.
	for cachedPath := range s.fileCache {
		if filepath.Dir(cachedPath) == dir && !diskNames[cachedPath] {
			delete(s.fileCache, cachedPath)
		}
	}

	// Ensure all open docs in this directory are in memFiles (even if not yet on disk).
	for _, doc := range s.openDocs {
		if doc.dir == dir {
			memFiles[doc.path] = doc.content
		}
	}

	ctxt := build.Default
	ctxt.OpenFile = func(p string) (io.ReadCloser, error) {
		clean := filepath.Clean(p)
		if b, ok := memFiles[clean]; ok {
			return io.NopCloser(bytes.NewReader(b)), nil
		}
		return os.Open(p)
	}

	selected := make(map[string][]byte)

	for _, doc := range s.openDocs {
		if doc.dir != dir {
			continue
		}
		base := filepath.Base(doc.path)
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		if key.pkgName != "" && doc.pkgName != "" && doc.pkgName != key.pkgName {
			continue
		}
		matched, err := ctxt.MatchFile(dir, base)
		if err != nil || !matched {
			continue
		}
		selected[doc.path] = doc.content
	}

	for fullPath := range diskNames {
		if _, already := selected[fullPath]; already {
			continue
		}
		if s.findOpenDocByPath(fullPath) != nil {
			continue
		}
		cached, ok := s.fileCache[fullPath]
		if !ok {
			continue
		}
		if key.pkgName != "" && cached.pkgName != "" && cached.pkgName != key.pkgName {
			continue
		}
		base := filepath.Base(fullPath)
		matched, err := ctxt.MatchFile(dir, base)
		if err != nil || !matched {
			continue
		}
		selected[fullPath] = cached.content
	}

	return selected
}

func (s *Server) findOpenDocByPath(cleanPath string) *openDoc {
	for _, doc := range s.openDocs {
		if doc.path == cleanPath {
			return doc
		}
	}
	return nil
}

func (s *Server) publishForPackage(key pkgKey, ps *pkgState) {
	allDiags := make([]directive.Diagnostic, 0, len(ps.syntaxDiags)+len(ps.typeDiags))
	allDiags = append(allDiags, ps.syntaxDiags...)
	allDiags = append(allDiags, ps.typeDiags...)

	// Sort open URIs deterministically.
	var docs []*openDoc
	for _, doc := range s.openDocs {
		if doc.dir == key.dir && doc.pkgName == key.pkgName {
			docs = append(docs, doc)
		}
	}
	slices.SortFunc(docs, func(a, b *openDoc) int {
		return strings.Compare(a.uri, b.uri)
	})

	for _, doc := range docs {
		out := make([]Diagnostic, 0)
		for _, d := range allDiags {
			if !sameFilePath(d.Pos.Filename, doc) {
				continue
			}
			sev := severityError
			if d.Severity == directive.Warning {
				sev = severityWarning
			}
			out = append(out, Diagnostic{
				Range:    tokenPosToLineRange(d.Pos, doc.lines, s.encoding),
				Severity: sev,
				Source:   "handlers",
				Message:  d.Message,
			})
		}
		s.publishDiagnostics(doc.uri, out)
	}
}

func (s *Server) linesForFile(path string) []string {
	clean := filepath.Clean(path)
	if doc := s.findOpenDocByPath(clean); doc != nil {
		return doc.lines
	}
	if cached, ok := s.fileCache[clean]; ok {
		return splitLines(cached.content)
	}
	b, err := os.ReadFile(clean)
	if err != nil {
		return nil
	}
	return splitLines(b)
}

func (s *Server) publishDiagnostics(uri string, diags []Diagnostic) {
	if diags == nil {
		diags = []Diagnostic{}
	}
	_ = writeFrame(s.out, rpcNotification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: diags,
		},
	})
}

func (s *Server) sendResult(id *json.RawMessage, res any) {
	var raw json.RawMessage
	if res == nil {
		raw = json.RawMessage("null")
	} else {
		b, err := json.Marshal(res)
		if err != nil {
			s.sendError(id, codeInternalError, err.Error())
			return
		}
		raw = b
	}
	_ = writeFrame(s.out, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  raw,
	})
}

func (s *Server) sendError(id *json.RawMessage, code int, msg string) {
	_ = writeFrame(s.out, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: msg,
		},
	})
}

func parsePkgName(filename string, src []byte) string {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, filename, src, parser.PackageClauseOnly)
	if f != nil && f.Name != nil {
		return f.Name.Name
	}
	return ""
}

func resolveRealPath(p string) string {
	clean := filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(clean); err == nil {
		return real
	}
	return clean
}

func sameFilePath(diagFile string, doc *openDoc) bool {
	if diagFile == "" {
		return false
	}
	clean := filepath.Clean(diagFile)
	if clean == doc.path || clean == doc.realPath {
		return true
	}
	return resolveRealPath(clean) == doc.realPath
}
