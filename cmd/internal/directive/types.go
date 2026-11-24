package directive

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

var (
	textMarshalerIface   = buildTextMarshaler()
	textUnmarshalerIface = buildTextUnmarshaler()
)

func buildTextMarshaler() *types.Interface {
	byteSlice := types.NewSlice(types.Typ[types.Byte])
	errType := types.Universe.Lookup("error").Type()
	sig := types.NewSignatureType(nil, nil, nil, nil,
		types.NewTuple(
			types.NewVar(token.NoPos, nil, "", byteSlice),
			types.NewVar(token.NoPos, nil, "", errType),
		), false)
	return types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "MarshalText", sig),
	}, nil).Complete()
}

func buildTextUnmarshaler() *types.Interface {
	byteSlice := types.NewSlice(types.Typ[types.Byte])
	errType := types.Universe.Lookup("error").Type()
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "", byteSlice)),
		types.NewTuple(types.NewVar(token.NoPos, nil, "", errType)),
		false)
	return types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "UnmarshalText", sig),
	}, nil).Complete()
}

// Types type-checks the package in dir with go/packages and checks parameter types (§4.2).
// It returns only the diagnostics that need type information.
func Types(dir string, pkg *Package, out string) ([]Diagnostic, error) {
	if pkg == nil || len(pkg.Routes) == 0 {
		return nil, nil
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedSyntax | packages.NeedModule,
		Dir: dir,
	}
	lpkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}
	if len(lpkgs) == 0 {
		return nil, fmt.Errorf("no package found in %s", dir)
	}
	lpkg := lpkgs[0]

	if len(lpkg.Errors) > 0 {
		if diag, ok := findRouteDiagnostic(lpkg, dir, out); ok {
			return []Diagnostic{diag}, nil
		}
		pos, msg := formatPackageError(lpkg.Errors[0], dir)
		return []Diagnostic{{
			Pos:      pos,
			Severity: Error,
			Message:  "the package doesn't type-check: " + msg,
		}}, nil
	}

	if lpkg.Types == nil || pkg.Type == "" {
		return nil, nil
	}
	recvObj := lpkg.Types.Scope().Lookup(pkg.Type)
	if recvObj == nil {
		return nil, nil
	}
	recvPtr := types.NewPointer(recvObj.Type())

	var diags []Diagnostic
	for ri := range pkg.Routes {
		r := &pkg.Routes[ri]
		if len(r.Params) == 0 {
			continue
		}
		fnObj, _, _ := types.LookupFieldOrMethod(recvPtr, true, lpkg.Types, r.Handler)
		fn, ok := fnObj.(*types.Func)
		if !ok {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Params().Len() < 2+len(r.Params) {
			continue
		}

		restName := ""
		for _, wc := range extractWildcards(r.Path) {
			if wc.isRest {
				restName = wc.name
			}
		}

		sigParams := sig.Params()
		for i := range r.Params {
			p := &r.Params[i]
			pt := sigParams.At(2 + i).Type()
			typeName := types.TypeString(pt, types.RelativeTo(lpkg.Types))

			if isDeclaredInPackage(pt, lpkg.Types) {
				diags = append(diags, Diagnostic{
					Pos:      p.Pos,
					Severity: Error,
					Message:  fmt.Sprintf("parameter %s: type %s is declared in this package; declare it in another so the generated package can import it", p.Name, typeName),
				})
				continue
			}

			if restName != "" && p.Name == restName {
				if !isStringType(pt) {
					diags = append(diags, Diagnostic{
						Pos:      p.Pos,
						Severity: Error,
						Message:  fmt.Sprintf("parameter %s: a {%s...} wildcard needs a string type", p.Name, restName),
					})
					continue
				}
				recordParamType(p, pt, lpkg.Types)
				continue
			}

			if !isSupportedBasic(pt) && !implementsTextCodec(pt) {
				diags = append(diags, Diagnostic{
					Pos:      p.Pos,
					Severity: Error,
					Message:  fmt.Sprintf("parameter %s: type %s can't be read from a path", p.Name, typeName),
				})
				continue
			}

			recordParamType(p, pt, lpkg.Types)
		}
	}

	sortDiagnostics(diags)
	return diags, nil
}

func recordParamType(p *Param, pt types.Type, pkg *types.Package) {
	u := types.Unalias(pt)
	switch t := u.(type) {
	case *types.Named:
		if obj := t.Obj(); obj != nil && obj.Pkg() != nil {
			p.ImportPath = obj.Pkg().Path()
			p.PkgName = obj.Pkg().Name()
			p.TypeName = p.PkgName + "." + obj.Name()
		} else {
			p.TypeName = types.TypeString(pt, types.RelativeTo(pkg))
		}
	case *types.Basic:
		p.ImportPath = ""
		p.PkgName = ""
		p.TypeName = t.Name()
	default:
		p.TypeName = types.TypeString(pt, types.RelativeTo(pkg))
	}

	if implementsTextCodec(pt) {
		p.Parse = "text"
		return
	}
	if b, ok := pt.Underlying().(*types.Basic); ok {
		p.Parse = basicParseName(b.Kind())
	}
}

func basicParseName(k types.BasicKind) string {
	switch k {
	case types.String:
		return "string"
	case types.Bool:
		return "bool"
	case types.Int:
		return "int"
	case types.Int8:
		return "int8"
	case types.Int16:
		return "int16"
	case types.Int32:
		return "int32"
	case types.Int64:
		return "int64"
	case types.Uint:
		return "uint"
	case types.Uint8:
		return "uint8"
	case types.Uint16:
		return "uint16"
	case types.Uint32:
		return "uint32"
	case types.Uint64:
		return "uint64"
	default:
		return ""
	}
}

func isDeclaredInPackage(t types.Type, pkg *types.Package) bool {
	if pkg == nil {
		return false
	}
	for {
		t = types.Unalias(t)
		switch u := t.(type) {
		case *types.Pointer:
			t = u.Elem()
		case *types.Slice:
			t = u.Elem()
		case *types.Array:
			t = u.Elem()
		case *types.Named:
			return u.Obj() != nil && u.Obj().Pkg() == pkg
		default:
			return false
		}
	}
}

func isStringType(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.String
}

func isSupportedBasic(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	switch b.Kind() {
	case types.String, types.Bool,
		types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
		return true
	default:
		return false
	}
}

func implementsTextCodec(t types.Type) bool {
	if !types.Implements(t, textMarshalerIface) {
		return false
	}
	return types.Implements(t, textUnmarshalerIface) || types.Implements(types.NewPointer(t), textUnmarshalerIface)
}

func formatPackageError(e packages.Error, dir string) (token.Position, string) {
	if e.Pos != "" {
		return parsePackageErrorPos(e.Pos, dir), e.Msg
	}
	for line := range strings.SplitSeq(e.Msg, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, ": "); idx > 0 {
			posPart := line[:idx]
			msgPart := line[idx+2:]
			pos := parsePackageErrorPos(posPart, dir)
			if pos.Filename != posPart {
				if !filepath.IsAbs(pos.Filename) {
					pos.Filename = filepath.Join(dir, filepath.Clean(pos.Filename))
				}
				return pos, msgPart
			}
		}
		return token.Position{Filename: dir, Line: 1, Column: 1}, line
	}
	return token.Position{Filename: dir, Line: 1, Column: 1}, e.Msg
}

func parsePackageErrorPos(posStr, fallbackDir string) token.Position {
	pos := token.Position{Filename: fallbackDir, Line: 1, Column: 1}
	if posStr == "" {
		return pos
	}
	// Format is typically "file:line:col" or "file:line" (and may have Windows drive letter like C:\...).
	parts := strings.Split(posStr, ":")
	if len(parts) >= 3 {
		if col, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			if line, err := strconv.Atoi(parts[len(parts)-2]); err == nil {
				pos.Filename = strings.Join(parts[:len(parts)-2], ":")
				pos.Line = line
				pos.Column = col
				return pos
			}
		}
	}
	if len(parts) >= 2 {
		if line, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			pos.Filename = strings.Join(parts[:len(parts)-1], ":")
			pos.Line = line
			return pos
		}
	}
	pos.Filename = posStr
	return pos
}

func findRouteDiagnostic(lpkg *packages.Package, dir, out string) (Diagnostic, bool) {
	if out != "" && !filepath.IsAbs(out) {
		if abs, err := filepath.Abs(out); err == nil {
			out = abs
		}
	}
	if out != "" {
		out = filepath.Clean(out)
	}

	modDir, modPath := findModuleInfo(dir)
	if lpkg.Module != nil {
		if lpkg.Module.Dir != "" {
			modDir = lpkg.Module.Dir
		}
		if lpkg.Module.Path != "" {
			modPath = lpkg.Module.Path
		}
	}
	outImportPath := computeOutImportPath(out, modDir, modPath)

	var qualifying []Diagnostic
	for _, e := range lpkg.Errors {
		pos, msg := formatPackageError(e, dir)

		if ident, name, ok := parseUndefinedPkgMember(msg); ok {
			if isGeneratedRoutesImport(lpkg, dir, pos.Filename, ident, out, modDir, modPath, outImportPath) {
				qualifying = append(qualifying, Diagnostic{
					Pos:      pos,
					Severity: Error,
					Message:  fmt.Sprintf("%s.%s doesn't exist yet: generate the route before linking to it", ident, name),
				})
				continue
			}
		}

		if outImportPath != "" && !hasGoFiles(out) && namesImport(e.Msg, outImportPath) {
			qualifying = append(qualifying, Diagnostic{
				Pos:      pos,
				Severity: Error,
				Message:  fmt.Sprintf("%s doesn't exist yet: generate the routes before linking to them", outImportPath),
			})
			continue
		}
	}

	if len(qualifying) > 0 {
		sortDiagnostics(qualifying)
		return qualifying[0], true
	}
	return Diagnostic{}, false
}

func parseUndefinedPkgMember(msg string) (ident, name string, ok bool) {
	msg = strings.TrimSpace(msg)
	const prefix = "undefined: "
	if !strings.HasPrefix(msg, prefix) {
		return "", "", false
	}
	target := strings.TrimPrefix(msg, prefix)
	parts := strings.Split(target, ".")
	if len(parts) != 2 {
		return "", "", false
	}
	ident, name = parts[0], parts[1]
	if !isGoIdent(ident) || !isGoIdent(name) || !token.IsExported(name) {
		return "", "", false
	}
	return ident, name, true
}

func isGoIdent(s string) bool {
	if s == "" || token.Lookup(s) != token.IDENT {
		return false
	}
	return token.IsIdentifier(s)
}

func isGeneratedRoutesImport(lpkg *packages.Package, dir, filename, ident, out, modDir, modPath, outImportPath string) bool {
	absFile := filename
	if !filepath.IsAbs(absFile) {
		absFile = filepath.Join(dir, absFile)
	}
	absFile = filepath.Clean(absFile)

	fileImports := findFileImports(lpkg, dir, absFile)
	for _, spec := range fileImports {
		impPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		var localName string
		if spec.Name != nil {
			localName = spec.Name.Name
		} else {
			if impPkg, ok := lpkg.Imports[impPath]; ok && impPkg.Name != "" {
				localName = impPkg.Name
			} else {
				localName = resolvePkgName(impPath, dir, modDir, modPath, out, outImportPath)
			}
		}
		if localName != ident {
			continue
		}

		pkgDir := resolvePkgDir(lpkg, impPath, dir, modDir, modPath, out, outImportPath)
		if pkgDir != "" {
			cleanPkgDir := filepath.Clean(pkgDir)
			if out != "" && cleanPkgDir == filepath.Clean(out) {
				return true
			}
			if hasRoutesJSON(cleanPkgDir) {
				return true
			}
		}
		if outImportPath != "" && impPath == outImportPath {
			return true
		}
	}
	return false
}

func findFileImports(lpkg *packages.Package, dir, absFile string) []*ast.ImportSpec {
	for _, f := range lpkg.Syntax {
		fPos := lpkg.Fset.Position(f.Pos())
		fName := fPos.Filename
		if !filepath.IsAbs(fName) {
			fName = filepath.Join(dir, fName)
		}
		if sameFilePath(fName, absFile) {
			return f.Imports
		}
	}
	fset := token.NewFileSet()
	if f, err := parser.ParseFile(fset, absFile, nil, parser.ImportsOnly); err == nil {
		return f.Imports
	}
	return nil
}

func sameFilePath(a, b string) bool {
	if a == b {
		return true
	}
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	if aReal, err := filepath.EvalSymlinks(a); err == nil {
		if bReal, err := filepath.EvalSymlinks(b); err == nil {
			return filepath.Clean(aReal) == filepath.Clean(bReal)
		}
	}
	return false
}

func resolvePkgDir(lpkg *packages.Package, impPath, dir, modDir, modPath, out, outImportPath string) string {
	if lpkg != nil {
		if impPkg, ok := lpkg.Imports[impPath]; ok && len(impPkg.GoFiles) > 0 {
			return filepath.Dir(impPkg.GoFiles[0])
		}
	}
	if outImportPath != "" && impPath == outImportPath && out != "" {
		return out
	}
	if modDir != "" && modPath != "" {
		if impPath == modPath {
			return modDir
		}
		if strings.HasPrefix(impPath, modPath+"/") {
			sub := strings.TrimPrefix(impPath, modPath+"/")
			return filepath.Join(modDir, filepath.FromSlash(sub))
		}
	}
	return ""
}

func resolvePkgName(impPath, dir, modDir, modPath, out, outImportPath string) string {
	pkgDir := resolvePkgDir(nil, impPath, dir, modDir, modPath, out, outImportPath)
	if pkgDir != "" {
		entries, err := os.ReadDir(pkgDir)
		if err == nil {
			var names []string
			for _, e := range entries {
				n := e.Name()
				if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
					continue
				}
				f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(pkgDir, n), nil, parser.PackageClauseOnly)
				if err == nil && !strings.HasSuffix(f.Name.Name, "_test") {
					names = append(names, f.Name.Name)
				}
			}
			if len(names) > 0 {
				slices.Sort(names)
				return names[0]
			}
		}
	}
	return path.Base(impPath)
}

func hasRoutesJSON(dir string) bool {
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "zz_") && strings.HasSuffix(e.Name(), ".routes.json") {
			return true
		}
	}
	return false
}

func findModuleInfo(startDir string) (modDir, modPath string) {
	cur := filepath.Clean(startDir)
	for {
		data, err := os.ReadFile(filepath.Join(cur, "go.mod"))
		if err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					return cur, strings.TrimSpace(strings.TrimPrefix(line, "module "))
				}
			}
			return cur, ""
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", ""
}

func computeOutImportPath(out, modDir, modPath string) string {
	if out == "" || modDir == "" || modPath == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(modDir); err == nil {
		modDir = r
	}
	if r, err := filepath.EvalSymlinks(out); err == nil {
		out = r
	} else if r, err := filepath.EvalSymlinks(filepath.Dir(out)); err == nil {
		out = filepath.Join(r, filepath.Base(out))
	}
	rel, err := filepath.Rel(modDir, out)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	if rel == "." {
		return modPath
	}
	return path.Join(modPath, filepath.ToSlash(rel))
}

// hasGoFiles reports whether dir holds a non-test Go file.
func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			return true
		}
	}
	return false
}

// namesImport reports whether msg is an import failure naming exactly path
// (quoted, or followed by a space, a parenthesis, a colon or the end).
func namesImport(msg, path string) bool {
	if !strings.Contains(msg, "import") && !strings.Contains(msg, "package") {
		return false
	}
	for i := 0; ; {
		j := strings.Index(msg[i:], path)
		if j < 0 {
			return false
		}
		end := i + j + len(path)
		if end == len(msg) || strings.ContainsRune("\" ( ):", rune(msg[end])) {
			return true
		}
		i = end
	}
}
