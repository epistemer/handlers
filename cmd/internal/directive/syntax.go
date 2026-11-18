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
	entityPos token.Position
	keyValid  bool
	methodOK  bool
	pathOK    bool
	permValid bool
	entityOK  bool
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
	"param":  true,
}

// Syntax checks one package from its files' contents without type information.
// It tolerates files that do not fully parse and never panics.
func Syntax(files map[string][]byte) (*Package, []Diagnostic) {
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

			rec, rDiags := validateDirectives(lines, fn, funcPos, params, sigErr == "")
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
	diags = append(diags, checkPackageRoutes(recs)...)

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

func validateDirectives(lines []parsedLine, fn *ast.FuncDecl, funcPos token.Position, params []Param, isHandler bool) (routeRec, []Diagnostic) {
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
			if !isValidKey(l.val) {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:perm: %q is not a key", l.val),
				})
			} else {
				rec.permValid = true
				rec.route.Perm = l.val
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
			}

		case "entity":
			rec.entityPos = l.pos
			if !isValidKey(l.val) {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  fmt.Sprintf("h:entity: %q is not a key", l.val),
				})
			} else {
				rec.entityOK = true
				rec.route.Entity = l.val
			}

		case "param":
			pName, kw, labelText, ok := parseParamDirective(l.val)
			if !ok || kw != "label" || labelText == "" {
				diags = append(diags, Diagnostic{
					Pos:      l.pos,
					Severity: Error,
					Message:  `h:param: write "h:param <name> label <text>"`,
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
				rec.route.Params[idx].Label = labelText
				rec.route.Params[idx].LabelSet = true
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

	if rec.entityOK && rec.keyValid && rec.route.Entity == rec.route.Key {
		diags = append(diags, Diagnostic{
			Pos:      rec.entityPos,
			Severity: Error,
			Message:  "h:entity names the route itself",
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

func parseParamDirective(val string) (name, kw, text string, ok bool) {
	p1, rest, f1 := cutWhitespace(val)
	if !f1 || p1 == "" {
		return "", "", "", false
	}
	rest = strings.TrimLeft(rest, " \t")
	p2, rest2, f2 := cutWhitespace(rest)
	if !f2 || p2 == "" {
		return "", "", "", false
	}
	rest2 = strings.TrimSpace(rest2)
	if rest2 == "" {
		return "", "", "", false
	}
	return p1, p2, rest2, true
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

func checkPackageRoutes(recs []routeRec) []Diagnostic {
	var diags []Diagnostic

	// Known route keys for h:entity lookup.
	routeKeys := make(map[string]token.Position)
	for _, r := range recs {
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

	// h:entity unknown route check.
	for _, r := range recs {
		if r.entityOK && r.route.Entity != r.route.Key {
			if _, ok := routeKeys[r.route.Entity]; !ok {
				diags = append(diags, Diagnostic{
					Pos:      r.entityPos,
					Severity: Error,
					Message:  fmt.Sprintf("h:entity: no route with key %q", r.route.Entity),
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

	// Permissions: unverified agreement, single-use warning, and Perm Go-name collisions.
	type permEntry struct {
		key        string
		pos        token.Position
		unverified string
		verified   string
		count      int
		reported   bool
	}
	perms := make(map[string]*permEntry)
	type goNameEntry struct {
		key string
		pos token.Position
	}
	permVars := make(map[string]goNameEntry)

	for _, r := range recs {
		if r.route.Auth == "public" || r.route.Auth == "session" {
			continue
		}
		var permKey string
		var permPos token.Position
		if r.permValid {
			permKey = r.route.Perm
			permPos = r.permPos
		} else if r.keyValid {
			permKey = r.route.Key
			permPos = r.keyPos
		} else {
			continue
		}

		pe, exists := perms[permKey]
		if !exists {
			pe = &permEntry{key: permKey, pos: permPos}
			perms[permKey] = pe

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
		pe.count++
		if r.route.Auth == "unverified" {
			if pe.unverified == "" {
				pe.unverified = r.route.Key
			}
		} else {
			if pe.verified == "" {
				pe.verified = r.route.Key
			}
		}
		if pe.unverified != "" && pe.verified != "" && !pe.reported {
			pe.reported = true
			diags = append(diags, Diagnostic{
				Pos:      permPos,
				Severity: Error,
				Message:  fmt.Sprintf("permission %q: %s is unverified and %s isn't", permKey, pe.unverified, pe.verified),
			})
		}
	}

	for _, r := range recs {
		if !r.permValid || r.route.Auth == "public" || r.route.Auth == "session" {
			continue
		}
		if pe := perms[r.route.Perm]; pe != nil && pe.count == 1 {
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
