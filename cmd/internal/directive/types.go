package directive

import (
	"fmt"
	"go/token"
	"go/types"
	"path/filepath"
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
func Types(dir string, pkg *Package) ([]Diagnostic, error) {
	if pkg == nil || len(pkg.Routes) == 0 {
		return nil, nil
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedTypes | packages.NeedTypesInfo,
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
