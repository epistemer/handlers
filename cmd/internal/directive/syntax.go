package directive

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

type parsedLine struct {
	pos      token.Position
	isDir    bool
	name     string
	val      string
	nearMiss string
}

type routeRec struct {
	route     Route
	keyPos    token.Position
	methodPos token.Position
	pathPos   token.Position
	permPos   token.Position
	viewPos   token.Position
	searchPos token.Position
	bodyPos   token.Position
	keyValid  bool
	methodOK  bool
	pathOK    bool
	permValid bool
	bodyOK    bool
	hasKey    bool
	hasMethod bool
	hasPath   bool
	hasDesc   bool
}

var reservedGenNames = map[string]bool{
	"Handler":  true,
	"Register": true,
	"Mount":    true,
	"Routes":   true,
	"ByKey":    true,
	"Prefix":   true,
	"L10n":     true,
}

var validMethods = map[string]bool{
	"GET":    true,
	"POST":   true,
	"PUT":    true,
	"PATCH":  true,
	"DELETE": true,
}

var validAuths = map[string]bool{
	"public":     true,
	"session":    true,
	"unverified": true,
}

var knownDirectives = map[string]bool{
	"key":    true,
	"method": true,
	"path":   true,
	"desc":   true,
	"auth":   true,
	"perm":   true,
	"view":   true,
	"search": true,
	"entity": true,
	"body":   true,
	"param":  true,
}

type syntaxConfig struct {
	routesPkg     string
	externalTakes map[string]int
}

// SyntaxOption configures directive syntax checking.
type SyntaxOption func(*syntaxConfig)

// WithRoutesPackage specifies the routes package name for the package being checked.
func WithRoutesPackage(pkg string) SyntaxOption {
	return func(c *syntaxConfig) {
		c.routesPkg = pkg
	}
}

// WithExternalPermCounts specifies counts of external taking routes for single-use warning.
func WithExternalPermCounts(counts map[string]int) SyntaxOption {
	return func(c *syntaxConfig) {
		c.externalTakes = counts
	}
}

// Syntax checks one package from its files' contents without type information.
// It tolerates files that do not fully parse and never panics.
func Syntax(files map[string][]byte, opts ...SyntaxOption) (*Package, []Diagnostic) {
	var cfg syntaxConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	pkg := &Package{}
	var diags []Diagnostic

	if len(files) == 0 {
		return pkg, nil
	}

	filenames := make([]string, 0, len(files))
	for name := range files {
		filenames = append(filenames, name)
	}
	sort.Strings(filenames)

	fset := token.NewFileSet()
	var recs []routeRec
	var firstRecvType string
	reportedRecvTypes := make(map[string]bool)

	for _, filename := range filenames {
		src := files[filename]
		f, _ := parser.ParseFile(fset, filename, src, parser.ParseComments)
		if f == nil {
			continue
		}
		if pkg.Name == "" && f.Name != nil {
			pkg.Name = f.Name.Name
		}

		httpAliases, dotHTTP := findNetHTTPImports(f)
		docComments := make(map[*ast.CommentGroup]bool)

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn == nil || fn.Doc == nil {
				continue
			}
			docComments[fn.Doc] = true

			lines := parseCommentGroup(fset, fn.Doc)
			hasAnyDir := false
			for _, l := range lines {
				if l.nearMiss != "" {
					diags = append(diags, Diagnostic{
						Pos:      l.pos,
						Severity: Error,
						Message:  fmt.Sprintf("not a directive: write %q", l.nearMiss),
					})
				}
				if l.isDir {
					hasAnyDir = true
				}
			}
			if !hasAnyDir {
				continue
			}

			funcPos := fset.Position(fn.Pos())
			if fn.Type != nil && fn.Type.Func.IsValid() {
				funcPos = fset.Position(fn.Type.Func)
			}

			recvType := receiverTypeName(fn.Recv)
			if recvType != "" {
				if firstRecvType == "" {
					firstRecvType = recvType
					pkg.Type = recvType
				} else if recvType != firstRecvType && !reportedRecvTypes[recvType] {
					reportedRecvTypes[recvType] = true
					diags = append(diags, Diagnostic{
						Pos:      funcPos,
						Severity: Error,
						Message:  fmt.Sprintf("directives on %s and %s: one handler type per package", firstRecvType, recvType),
					})
				}
			}

			params, sigErr := checkHandlerSignature(fset, fn, httpAliases, dotHTTP)
			if sigErr != "" {
				fnName := "<anonymous>"
				if fn.Name != nil {
					fnName = fn.Name.Name
				}
				diags = append(diags, Diagnostic{
					Pos:      funcPos,
					Severity: Error,
					Message:  fmt.Sprintf("%s isn't a handler: %s", fnName, sigErr),
				})
			}

			rec, rDiags := validateDirectives(lines, fn, funcPos, params, sigErr == "", cfg.routesPkg)
			diags = append(diags, rDiags...)
			if sigErr == "" {
				recs = append(recs, rec)
			}
		}

		// Check comments outside FuncDecl.Doc for stray directives.
		for _, cg := range f.Comments {
			if docComments[cg] {
				continue
			}
			lines := parseCommentGroup(fset, cg)
			reportedOutside := false
			for _, l := range lines {
				if l.nearMiss != "" {
					diags = append(diags, Diagnostic{
						Pos:      l.pos,
						Severity: Error,
						Message:  fmt.Sprintf("not a directive: write %q", l.nearMiss),
					})
				}
				if l.isDir && !reportedOutside {
					reportedOutside = true
					diags = append(diags, Diagnostic{
						Pos:      l.pos,
						Severity: Error,
						Message:  "directives must be the doc comment of a handler method",
					})
				}
			}
		}
	}

	// Package-level checks across valid handler routes.
	diags = append(diags, checkPackageRoutes(recs, cfg)...)

	for _, rec := range recs {
		pkg.Routes = append(pkg.Routes, rec.route)
	}

	sortDiagnostics(diags)
	return pkg, diags
}

func findNetHTTPImports(f *ast.File) (aliases map[string]bool, dot bool) {
	aliases = make(map[string]bool)
	for _, imp := range f.Imports {
		if imp == nil || imp.Path == nil || imp.Path.Value != `"net/http"` {
			continue
		}
		if imp.Name == nil {
			aliases["http"] = true
			continue
		}
		switch imp.Name.Name {
		case "_":
		case ".":
			dot = true
		default:
			aliases[imp.Name.Name] = true
		}
	}
	return aliases, dot
}

func parseCommentGroup(fset *token.FileSet, cg *ast.CommentGroup) []parsedLine {
	if cg == nil {
		return nil
	}
	var out []parsedLine
	for _, c := range cg.List {
		text := c.Text
		if !strings.HasPrefix(text, "//") {
			continue
		}
		pos := fset.Position(c.Slash)
		if strings.HasPrefix(text, "//h:") {
			rest := text[4:]
			name, val, hasSpace := cutWhitespace(rest)
			if !hasSpace {
				val = ""
			} else {
				val = strings.TrimSpace(val)
			}
			out = append(out, parsedLine{
				pos:   pos,
				isDir: true,
				name:  name,
				val:   val,
			})
			continue
		}
		if fixed, ok := matchNearMiss(text); ok {
			out = append(out, parsedLine{
				pos:      pos,
				nearMiss: fixed,
			})
		}
	}
	return out
}

func cutWhitespace(s string) (before, after string, found bool) {
	for i, r := range s {
		if r == ' ' || r == '\t' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

func matchNearMiss(line string) (string, bool) {
	if !strings.HasPrefix(line, "//") {
		return "", false
	}
	after := strings.TrimLeft(line[2:], " \t")
	if strings.HasPrefix(after, "h") || strings.HasPrefix(after, "H") {
		rest := strings.TrimLeft(after[1:], " \t")
		if strings.HasPrefix(rest, ":") {
			val := strings.TrimSpace(rest[1:])
			return "//h:" + val, true
		}
		return "", false
	}
	if strings.HasPrefix(after, "p[") {
		inner := after[2:]
		if idx := strings.IndexByte(inner, ']'); idx >= 0 {
			name := inner[:idx]
			tail := inner[idx+1:]
			tail = strings.TrimPrefix(tail, ":")
			tail = strings.TrimSpace(tail)
			if tail != "" {
				return "//h:param " + name + " " + tail, true
			}
			return "//h:param " + name, true
		}
		return "//h:param " + strings.TrimSpace(inner), true
	}
	return "", false
}

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func checkHandlerSignature(fset *token.FileSet, fn *ast.FuncDecl, httpAliases map[string]bool, dotHTTP bool) ([]Param, string) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return nil, "not a method"
	}
	if fn.Name == nil || !ast.IsExported(fn.Name.Name) {
		return nil, "not exported"
	}
	if fn.Type == nil || fn.Type.Params == nil {
		return nil, "the first parameters must be http.ResponseWriter and *http.Request"
	}

	type flatParam struct {
		name string
		pos  token.Position
		typ  ast.Expr
	}
	var flat []flatParam
	for _, field := range fn.Type.Params.List {
		if len(field.Names) == 0 {
			flat = append(flat, flatParam{
				pos: fset.Position(field.Pos()),
				typ: field.Type,
			})
			continue
		}
		for _, id := range field.Names {
			flat = append(flat, flatParam{
				name: id.Name,
				pos:  fset.Position(id.Pos()),
				typ:  field.Type,
			})
		}
	}

	if len(flat) < 2 || !isHTTPSelector(flat[0].typ, "ResponseWriter", httpAliases, dotHTTP) {
		return nil, "the first parameters must be http.ResponseWriter and *http.Request"
	}
	star, ok := flat[1].typ.(*ast.StarExpr)
	if !ok || !isHTTPSelector(star.X, "Request", httpAliases, dotHTTP) {
		return nil, "the first parameters must be http.ResponseWriter and *http.Request"
	}

	if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 || len(fn.Type.Results.List[0].Names) > 1 {
		return nil, "it must return error"
	}
	resID, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
	if !ok || resID.Name != "error" {
		return nil, "it must return error"
	}

	var params []Param
	for _, p := range flat[2:] {
		params = append(params, Param{
			Name: p.name,
			Pos:  p.pos,
		})
	}
	return params, ""
}

func isHTTPSelector(expr ast.Expr, selName string, aliases map[string]bool, dot bool) bool {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		if t.Sel == nil || t.Sel.Name != selName {
			return false
		}
		pkgID, ok := t.X.(*ast.Ident)
		return ok && aliases[pkgID.Name]
	case *ast.Ident:
		return dot && t.Name == selName
	default:
		return false
	}
}

func validateDirectives(lines []parsedLine, fn *ast.FuncDecl, funcPos token.Position, params []Param, isHandler bool, routesPkg string) (routeRec, []Diagnostic) {
	var diags []Diagnostic
	handlerName := ""
	if fn.Name != nil {
		handlerName = fn.Name.Name
	}

	paramIndex := make(map[string]int, len(params))
	for i, p := range params {
		if p.Name != "" {
			paramIndex[p.Name] = i
		}
	}

	rec := routeRec{
		route: Route{
			Handler: handlerName,
			Params:  params,
			Pos:     funcPos,
		},
	}

	seen := make(map[string]bool)
	seenParams := make(map[string]bool)

	for _, l := range lines {
		if !l.isDir {
			continue
		}
		if !knownDirectives[l.name] {
			diags = append(diags, Diagnostic{
				Pos:      l.pos,
				Severity: Error,
				Message:  fmt.Sprintf("unknown directive h:%s", l.name),
			})
			continue
		}

		if l.name != "param" {
			if seen[l.name] {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:%s given twice", l.name),
				})
				continue
			}
			seen[l.name] = true
		}

		switch l.name {
		case "key":
			rec.hasKey = true
			rec.keyPos = l.pos
			if !isValidKey(l.val) {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:key: %q is not a key", l.val),
				})
			} else {
				rec.keyValid = true
				rec.route.Key = l.val
			}

		case "method":
			rec.hasMethod = true
			rec.methodPos = l.pos
			upper := strings.ToUpper(l.val)
			if !validMethods[upper] {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:method: %q is not GET, POST, PUT, PATCH or DELETE", l.val),
				})
			} else {
				rec.methodOK = true
				rec.route.Method = upper
			}

		case "path":
			rec.hasPath = true
			rec.pathPos = l.pos
			rec.route.Path = l.val
			pathOK := true
			if !strings.HasPrefix(l.val, "/") {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf(`h:path: %q must start with "/"`, l.val),
				})
				pathOK = false
			} else if len(l.val) > 1 && strings.HasSuffix(l.val, "/") {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf(`h:path: %q must not end with "/"`, l.val),
				})
				pathOK = false
			} else if strings.Contains(l.val, "{$}") {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf(`h:path: %q: must not contain "{$}"`, l.val),
				})
				pathOK = false
			} else {
				method := rec.route.Method
				if method == "" {
					method = "GET"
				}
				if reason, ok := testServeMuxPattern(method + " " + l.val); !ok {
					diags = append(diags, Diagnostic{
						Pos:      l.pos,
						Severity: Error,
						Message:  fmt.Sprintf("h:path: %q: %s", l.val, reason),
					})
					pathOK = false
				}
			}
			rec.pathOK = pathOK

		case "desc":
			rec.hasDesc = true
			if l.val == "" {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  "h:desc is empty",
				})
			} else {
				rec.route.Desc = l.val
			}

		case "auth":
			if !validAuths[l.val] {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:auth: %q is not public, session or unverified", l.val),
				})
			} else {
				rec.route.Auth = l.val
			}

		case "perm":
			rec.permPos = l.pos
			pv, errMsg := ParsePerm(l.val, "h:perm")
			if errMsg != "" {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  errMsg,
				})
			} else if routesPkg != "" && pv.From != "" && pv.From == routesPkg {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:perm: %s is this routes package; declare the permission here", pv.From),
				})
			} else {
				rec.permValid = true
				rec.route.Perm = pv.Key
				rec.route.PermText = pv.Text
				rec.route.PermFrom = pv.From
			}

		case "view":
			if l.val != "" && l.val != "true" {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:view takes no value or true, not %q", l.val),
				})
			} else {
				rec.route.View = true
				rec.viewPos = l.pos
			}

		case "search":
			if l.val != "" && l.val != "true" {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:search takes no value or true, not %q", l.val),
				})
			} else {
				rec.route.Search = true
				rec.searchPos = l.pos
			}

		case "entity":
			// Retired by A9: a parameter names its source instead.
			diags = append(diags, Diagnostic{
				Pos:      l.pos,
				Severity: Error,
				Message:  `h:entity is retired: write "h:param <name> from <key>" on the parameter`,
			})

		case "body":
			rec.bodyPos = l.pos
			n, errMsg := ParseBodySize(l.val, "h:body")
			if errMsg != "" {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  errMsg,
				})
			} else {
				rec.bodyOK = true
				rec.route.MaxBody = n
			}

		case "param":
			pName, from, labelText, ok := parseParamDirective(l.val)
			if !ok {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  `h:param: write "h:param <name> [from <key>] [label <text>]"`,
				})
				continue
			}
			if from != "" && !isValidKey(from) {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:param %s: %q is not a key", pName, from),
				})
				continue
			}
			if seenParams[pName] {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:param %s given twice", pName),
				})
				continue
			}
			seenParams[pName] = true
			if isHandler {
				idx, exists := paramIndex[pName]
				if !exists {
					diags = append(diags, Diagnostic{
						Pos:      l.pos,
						Severity: Error,
						Message:  fmt.Sprintf("h:param: no parameter %s", pName),
					})
					continue
				}
				if labelText != "" {
					rec.route.Params[idx].Label = labelText
					rec.route.Params[idx].LabelSet = true
				}
				if from != "" {
					rec.route.Params[idx].From = from
					rec.route.Params[idx].FromPos = l.pos
				}
			}
		}
	}

	if rec.permValid && (rec.route.Auth == "public" || rec.route.Auth == "session") {
		diags = append(diags, Diagnostic{
			Pos:      rec.permPos,
			Severity: Error,
			Message:  fmt.Sprintf("h:perm needs a permission route, not auth %s", rec.route.Auth),
		})
	}

	if rec.methodOK && rec.route.Method != "GET" {
		if rec.route.View {
			diags = append(diags, Diagnostic{
				Pos:      rec.viewPos,
				Severity: Error,
				Message:  fmt.Sprintf("h:view on a %s route: only GET pages are searched", rec.route.Method),
			})
		}
		if rec.route.Search {
			diags = append(diags, Diagnostic{
				Pos:      rec.searchPos,
				Severity: Error,
				Message:  fmt.Sprintf("h:search on a %s route: only GET pages are searched", rec.route.Method),
			})
		}
	}

	if rec.keyValid {
		for _, p := range rec.route.Params {
			if p.From != "" && p.From == rec.route.Key {
				diags = append(diags, Diagnostic{
					Pos:      p.FromPos,
					Severity: Error,
					Message:  fmt.Sprintf("h:param %s: from names the route itself", p.Name),
				})
			}
		}
	}

	if rec.bodyOK && rec.methodOK && rec.route.Method == "GET" {
		diags = append(diags, Diagnostic{
			Pos:      rec.bodyPos,
			Severity: Error,
			Message:  "h:body on a GET route, which has no body",
		})
	}

	if !isHandler {
		return rec, diags
	}

	if !rec.hasKey {
		diags = append(diags, Diagnostic{Pos: funcPos, Severity: Error, Message: "missing h:key"})
	}
	if !rec.hasMethod {
		diags = append(diags, Diagnostic{Pos: funcPos, Severity: Error, Message: "missing h:method"})
	}
	if !rec.hasPath {
		diags = append(diags, Diagnostic{Pos: funcPos, Severity: Error, Message: "missing h:path"})
	}
	if !rec.hasDesc && rec.keyValid {
		diags = append(diags, Diagnostic{
			Pos:      funcPos,
			Severity: Warning,
			Message:  fmt.Sprintf("missing h:desc; the label is %q", Readable(rec.route.Key)),
		})
	}

	if rec.pathOK {
		wcs := extractWildcards(rec.route.Path)
		wcSet := make(map[string]bool, len(wcs))
		for _, wc := range wcs {
			wcSet[wc.name] = true
			if _, ok := paramIndex[wc.name]; !ok {
				diags = append(diags, Diagnostic{
					Pos:      rec.pathPos,
					Severity: Error,
					Message:  fmt.Sprintf("wildcard {%s} has no parameter", wc.name),
				})
			}
		}
		for _, p := range rec.route.Params {
			if p.Name != "" && !wcSet[p.Name] {
				diags = append(diags, Diagnostic{
					Pos:      p.Pos,
					Severity: Error,
					Message:  fmt.Sprintf("parameter %s has no wildcard in %s", p.Name, rec.route.Path),
				})
			}
		}
	}

	return rec, diags
}

// parseParamDirective reads "<name> [from <key>] [label <text>]", with at
// least one of from and label (A9). from's key is returned unchecked.
func parseParamDirective(val string) (name, from, label string, ok bool) {
	name, rest, found := cutWhitespace(val)
	if !found || name == "" {
		return "", "", "", false
	}
	rest = strings.TrimLeft(rest, " \t")
	kw, after, _ := cutWhitespace(rest)
	after = strings.TrimLeft(after, " \t")
	if kw == "from" {
		from, after, _ = cutWhitespace(after)
		if from == "" {
			return "", "", "", false
		}
		after = strings.TrimLeft(after, " \t")
		if after == "" {
			return name, from, "", true
		}
		kw, after, _ = cutWhitespace(after)
		after = strings.TrimLeft(after, " \t")
	}
	if kw != "label" {
		return "", "", "", false
	}
	label = strings.TrimSpace(after)
	if label == "" {
		return "", "", "", false
	}
	return name, from, label, true
}

func testServeMuxPattern(pattern string) (reason string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			reason = fmt.Sprint(r)
			ok = false
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	return "", true
}

func checkPackageRoutes(recs []routeRec, cfg syntaxConfig) []Diagnostic {
	var diags []Diagnostic

	// Known route keys, for h:param's from.
	routeKeys := make(map[string]token.Position)
	byKey := make(map[string]*routeRec)
	for i := range recs {
		r := recs[i]
		if r.keyValid {
			if _, seen := byKey[r.route.Key]; !seen {
				byKey[r.route.Key] = &recs[i]
			}
		}
		if r.keyValid {
			if first, dup := routeKeys[r.route.Key]; dup {
				diags = append(diags, Diagnostic{
					Pos:      r.keyPos,
					Severity: Error,
					Message:  fmt.Sprintf("key %q also at %s:%d", r.route.Key, filepath.Base(first.Filename), first.Line),
				})
			} else {
				routeKeys[r.route.Key] = r.keyPos
			}
		}
	}

	// h:param's from: a GET route of this package without parameters.
	for _, r := range recs {
		for _, p := range r.route.Params {
			if p.From == "" || (r.keyValid && p.From == r.route.Key) {
				continue
			}
			src, ok := byKey[p.From]
			switch {
			case !ok:
				diags = append(diags, Diagnostic{
					Pos:      p.FromPos,
					Severity: Error,
					Message:  fmt.Sprintf("h:param %s: no route with key %q", p.Name, p.From),
				})
			case src.methodOK && src.route.Method != "GET":
				diags = append(diags, Diagnostic{
					Pos:      p.FromPos,
					Severity: Error,
					Message:  fmt.Sprintf("h:param %s: %s is %s, and a source must be GET", p.Name, p.From, src.route.Method),
				})
			case len(src.route.Params) > 0:
				diags = append(diags, Diagnostic{
					Pos:      p.FromPos,
					Severity: Error,
					Message:  fmt.Sprintf("h:param %s: %s has parameters, and a source can't", p.Name, p.From),
				})
			}
		}
	}

	// Duplicate method+path and ServeMux conflicts.
	seenPatterns := make(map[string]token.Position)
	mux := http.NewServeMux()
	var registered []routeRec

	for _, r := range recs {
		if !r.methodOK || !r.pathOK {
			continue
		}
		pat := r.route.Method + " " + r.route.Path
		if first, dup := seenPatterns[pat]; dup {
			diags = append(diags, Diagnostic{
				Pos:      r.pathPos,
				Severity: Error,
				Message:  fmt.Sprintf("%s also at %s:%d", pat, filepath.Base(first.Filename), first.Line),
			})
			continue
		}
		seenPatterns[pat] = r.pathPos

		if _, ok := registerServeMux(mux, pat); !ok {
			for _, prev := range registered {
				prevPat := prev.route.Method + " " + prev.route.Path
				if conflictsWith(prevPat, pat) {
					diags = append(diags, Diagnostic{
						Pos:      r.pathPos,
						Severity: Error,
						Message:  fmt.Sprintf("%s conflicts with %s at %s:%d", pat, prevPat, filepath.Base(prev.pathPos.Filename), prev.pathPos.Line),
					})
					break
				}
			}
			continue
		}
		registered = append(registered, r)
	}

	// Permissions: declared vs taken, texts, unverified agreement, single-use warning, and Perm Go-name collisions.
	type permTrack struct {
		key string

		hasDecl     bool
		declPos     token.Position
		declText    string
		declTextPos token.Position

		hasTaken bool
		takenPkg string
		takenPos token.Position

		unverifiedKey      string
		verifiedKey        string
		unverifiedReported bool

		declCount int
	}
	perms := make(map[string]*permTrack)
	type goNameEntry struct {
		key string
		pos token.Position
	}
	permVars := make(map[string]goNameEntry)

	type takenOccur struct {
		permKey string
		pkg     string
		pos     token.Position
	}
	var takenOccurs []takenOccur

	for _, r := range recs {
		if r.route.Auth == "public" || r.route.Auth == "session" {
			continue
		}
		var permKey string
		var permPos token.Position
		var isTaken bool
		var fromPkg string
		var text string

		if r.permValid {
			permKey = r.route.Perm
			permPos = r.permPos
			isTaken = r.route.PermFrom != ""
			fromPkg = r.route.PermFrom
			text = r.route.PermText
		} else if r.keyValid {
			permKey = r.route.Key
			permPos = r.keyPos
		} else {
			continue
		}

		pt, exists := perms[permKey]
		if !exists {
			pt = &permTrack{key: permKey}
			perms[permKey] = pt
		}

		if isTaken {
			takenOccurs = append(takenOccurs, takenOccur{
				permKey: permKey,
				pkg:     fromPkg,
				pos:     permPos,
			})
			if pt.hasDecl {
				diags = append(diags, Diagnostic{
					Pos:      permPos,
					Severity: Error,
					Message:  fmt.Sprintf("permission %q is declared in this package at %s:%d, so it can't be taken from %s", permKey, filepath.Base(pt.declPos.Filename), pt.declPos.Line, fromPkg),
				})
			} else if pt.hasTaken && pt.takenPkg != fromPkg {
				diags = append(diags, Diagnostic{
					Pos:      permPos,
					Severity: Error,
					Message:  fmt.Sprintf("permission %q is taken from %s at %s:%d, not %s", permKey, pt.takenPkg, filepath.Base(pt.takenPos.Filename), pt.takenPos.Line, fromPkg),
				})
			} else if !pt.hasTaken {
				pt.hasTaken = true
				pt.takenPkg = fromPkg
				pt.takenPos = permPos
			}
		} else {
			pt.declCount++
			if !pt.hasDecl {
				pt.hasDecl = true
				pt.declPos = permPos
				for _, prevTaken := range takenOccurs {
					if prevTaken.permKey == permKey {
						diags = append(diags, Diagnostic{
							Pos:      prevTaken.pos,
							Severity: Error,
							Message:  fmt.Sprintf("permission %q is declared in this package at %s:%d, so it can't be taken from %s", permKey, filepath.Base(permPos.Filename), permPos.Line, prevTaken.pkg),
						})
					}
				}
			}
			if text != "" {
				if pt.declText != "" && pt.declText != text {
					diags = append(diags, Diagnostic{
						Pos:      permPos,
						Severity: Error,
						Message:  fmt.Sprintf("permission %q: text %q differs from %q at %s:%d", permKey, text, pt.declText, filepath.Base(pt.declTextPos.Filename), pt.declTextPos.Line),
					})
				} else if pt.declText == "" {
					pt.declText = text
					pt.declTextPos = permPos
				}
			}

			// Perm Go-name collisions (only for declared permissions)
			gn := permGoName(permKey)
			if prev, taken := permVars[gn]; taken && prev.key != permKey {
				diags = append(diags, Diagnostic{
					Pos:      permPos,
					Severity: Error,
					Message:  fmt.Sprintf("keys %q and %q both give the Go name %s", prev.key, permKey, gn),
				})
			} else if !taken {
				permVars[gn] = goNameEntry{key: permKey, pos: permPos}
			}
		}

		// Unverified agreement among routes in this package (both declared and taken)
		if r.route.Auth == "unverified" {
			if pt.unverifiedKey == "" {
				pt.unverifiedKey = r.route.Key
			}
		} else {
			if pt.verifiedKey == "" {
				pt.verifiedKey = r.route.Key
			}
		}
		if pt.unverifiedKey != "" && pt.verifiedKey != "" && !pt.unverifiedReported {
			pt.unverifiedReported = true
			diags = append(diags, Diagnostic{
				Pos:      permPos,
				Severity: Error,
				Message:  fmt.Sprintf("permission %q: %s is unverified and %s isn't", permKey, pt.unverifiedKey, pt.verifiedKey),
			})
		}
	}

	// Single-use warning: only for declared permissions (not taken)
	for _, r := range recs {
		if !r.permValid || r.route.Auth == "public" || r.route.Auth == "session" {
			continue
		}
		if r.route.PermFrom != "" {
			continue
		}
		ext := 0
		if cfg.externalTakes != nil {
			ext = cfg.externalTakes[r.route.Perm]
		}
		if pt := perms[r.route.Perm]; pt != nil && (pt.declCount+ext) == 1 {
			diags = append(diags, Diagnostic{
				Pos:      r.permPos,
				Severity: Warning,
				Message:  fmt.Sprintf("h:perm %q is used by this route only", r.route.Perm),
			})
		}
	}

	// Label Go-name collisions and reserved/taken handler names.
	labelVars := make(map[string]goNameEntry)
	for _, r := range recs {
		if !r.keyValid {
			continue
		}
		routeVar := "L10n" + r.route.Handler
		if prev, taken := labelVars[routeVar]; taken {
			diags = append(diags, Diagnostic{
				Pos:      r.route.Pos,
				Severity: Error,
				Message:  fmt.Sprintf("keys %q and %q both give the Go name %s", prev.key, r.route.Key, routeVar),
			})
		} else {
			labelVars[routeVar] = goNameEntry{key: r.route.Key, pos: r.route.Pos}
		}

		for _, p := range r.route.Params {
			if p.Name == "" {
				continue
			}
			paramVar := "L10n" + r.route.Handler + upperFirst(p.Name)
			if prev, taken := labelVars[paramVar]; taken {
				diags = append(diags, Diagnostic{
					Pos:      p.Pos,
					Severity: Error,
					Message:  fmt.Sprintf("keys %q and %q both give the Go name %s", prev.key, r.route.Key, paramVar),
				})
			} else {
				labelVars[paramVar] = goNameEntry{key: r.route.Key, pos: p.Pos}
			}
		}
	}

	for _, r := range recs {
		name := r.route.Handler
		if reservedGenNames[name] {
			diags = append(diags, Diagnostic{
				Pos:      r.route.Pos,
				Severity: Error,
				Message:  fmt.Sprintf("%s is a name the generated package declares", name),
			})
		}
		if lv, ok := labelVars[name]; ok {
			diags = append(diags, Diagnostic{
				Pos:      r.route.Pos,
				Severity: Error,
				Message:  fmt.Sprintf("%s is also the label variable of %s", name, lv.key),
			})
		}
		if pv, ok := permVars[name]; ok {
			diags = append(diags, Diagnostic{
				Pos:      r.route.Pos,
				Severity: Error,
				Message:  fmt.Sprintf("%s is also the variable of permission %s", name, pv.key),
			})
		}
	}

	return diags
}

func registerServeMux(mux *http.ServeMux, pattern string) (reason string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			reason = fmt.Sprint(r)
			ok = false
		}
	}()
	mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	return "", true
}

func conflictsWith(p1, p2 string) bool {
	m := http.NewServeMux()
	if _, ok := registerServeMux(m, p1); !ok {
		return false
	}
	_, ok := registerServeMux(m, p2)
	return !ok
}
