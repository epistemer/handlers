package main

import (
	"flag"
	"fmt"
	"go/token"
	"io"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/epistemer/handlers/cmd/internal/directive"
	"github.com/getkin/kin-openapi/openapi3"
)

type openAPIDiag struct {
	location string
	message  string
	severity directive.Severity
}

func (d openAPIDiag) format(specPath string) string {
	s := fmt.Sprintf("%s: %s: %s", specPath, d.location, d.message)
	if d.severity == directive.Warning {
		s += " (warning)"
	}
	return s
}

var canonicalMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

func runOpenAPI(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("openapi", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outDir := fs.String("out", "", "output directory (required)")
	pkgNameFlag := fs.String("package", "", "generated package name (default: last element of -out)")
	keyPrefix := fs.String("key-prefix", "", "prefix prepended to derived route keys")
	check := fs.Bool("check", false, "check whether the output file is up to date")
	strict := fs.Bool("strict", false, "treat warnings as errors")
	baseline := fs.String("baseline", "", "git ref baseline for removal warnings")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *outDir == "" || fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: handlers openapi -out <dir> [-package <name>] [-key-prefix <key>] [-check] [-strict] [-baseline <ref>] <spec>")
		return 2
	}
	if *keyPrefix != "" && !directive.IsValidKey(*keyPrefix) {
		fmt.Fprintf(stderr, "handlers openapi: -key-prefix %q is not a key\n", *keyPrefix)
		return 2
	}

	pkgName, ok := resolveRoutesPackageName(*pkgNameFlag, *outDir)
	if !ok {
		fmt.Fprintf(stderr, "handlers openapi: invalid package name %q\n", pkgName)
		return 2
	}

	specPath := fs.Arg(0)
	absSpecPath, err := filepath.Abs(specPath)
	if err != nil {
		fmt.Fprintf(stderr, "handlers openapi: %v\n", err)
		return 1
	}
	cfg, err := resolveRoutesPaths(*outDir, pkgName, absSpecPath, filepath.Dir(absSpecPath))
	if err != nil {
		fmt.Fprintf(stderr, "handlers openapi: %v\n", err)
		return 1
	}

	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromFile(specPath)
	if err != nil {
		fmt.Fprintf(stderr, "handlers openapi: %v\n", err)
		return 1
	}

	extCounts := countCommittedTakingRoutes(cfg.modRoot, cfg.sourceRel, cfg.outRel)
	doc, diags := analyzeOpenAPI(spec, cfg.sourceRel, cfg.pkgName, cfg.outRel, *keyPrefix, extCounts)

	hasError := false
	for _, d := range diags {
		if d.severity == directive.Error {
			hasError = true
			break
		}
	}
	if hasError {
		for _, d := range diags {
			fmt.Fprintln(stdout, d.format(specPath))
		}
		return 1
	}

	var warnLines []string
	for _, d := range diags {
		warnLines = append(warnLines, d.format(specPath))
	}

	return writeOrCheckRoutesDoc("openapi", cfg, doc, warnLines, *baseline, *strict, *check, stdout, stderr)
}

type openAPIOpInfo struct {
	loc      string
	method   string
	path     string
	op       *openapi3.Operation
	pathItem *openapi3.PathItem
	opID     string
	goName   string
	routeKey string
	auth     string
	permKey  string
	permText string
	permFrom string
	hasXPerm bool
	label    string
	maxBody  int64
	params   []paramDoc
}

func analyzeOpenAPI(spec *openapi3.T, sourceRel, pkgName, outRel, keyPrefix string, extCounts ...map[string]int) (routesDoc, []openAPIDiag) {
	var externalTakes map[string]int
	if len(extCounts) > 0 {
		externalTakes = extCounts[0]
	}
	var diags []openAPIDiag
	addErr := func(loc, msg string) {
		diags = append(diags, openAPIDiag{location: loc, message: msg, severity: directive.Error})
	}
	addWarn := func(loc, msg string) {
		diags = append(diags, openAPIDiag{location: loc, message: msg, severity: directive.Warning})
	}

	if spec.OpenAPI != "3.0" && !strings.HasPrefix(spec.OpenAPI, "3.0.") {
		addErr("openapi", fmt.Sprintf("%q is not 3.0.x", spec.OpenAPI))
	}

	prefix := parseOpenAPIPrefix(spec, addErr)

	var errSchemaRef *openapi3.SchemaRef
	if spec.Components != nil && spec.Components.Schemas != nil {
		errSchemaRef = spec.Components.Schemas["Error"]
	}
	if !isValidErrorSchema(errSchemaRef) {
		addErr("components/schemas/Error", "needs string properties code and message")
	}

	type opIDRef struct {
		opID string
		loc  string
	}
	seenOpID := make(map[string]string)
	seenGoName := make(map[string]opIDRef)
	seenRouteKey := make(map[string]string)

	var ops []*openAPIOpInfo

	if spec.Paths != nil {
		for _, pathStr := range spec.Paths.Keys() {
			pathItem := spec.Paths.Value(pathStr)
			if pathItem == nil {
				continue
			}
			for _, entry := range orderedOperations(pathItem) {
				method := entry.method
				op := entry.op
				loc := method + " " + pathStr

				if !isSupportedMethod(method) {
					addErr(loc, fmt.Sprintf("method %s isn't supported", method))
					continue
				}

				checkErrorResponses(loc, op, errSchemaRef, addErr)

				wildcards, pathOK := checkOpenAPIPathSegments(loc, pathStr, addErr)
				params := checkOpenAPIPathParams(loc, pathStr, pathItem, op, wildcards, pathOK, addErr)

				opID := op.OperationID
				goName := ""
				if opID == "" {
					addErr(loc, "no operationId")
				} else {
					if opID == "_" || !isGoIdentifier(opID) || !token.IsExported(upperFirst(opID)) {
						addErr(loc, fmt.Sprintf("operationId %q is not a Go identifier", opID))
					} else {
						goName = upperFirst(opID)
						if directive.IsReservedGenName(goName) {
							addErr(loc, fmt.Sprintf("%s is a name the generated package declares", goName))
						}
					}
					if firstLoc, dup := seenOpID[opID]; dup {
						addErr(loc, fmt.Sprintf("operationId %q also at %s", opID, firstLoc))
					} else {
						seenOpID[opID] = loc
					}
					if goName != "" {
						if prev, dup := seenGoName[goName]; dup && prev.opID != opID {
							addErr(loc, fmt.Sprintf("operationId %q gives the Go name %s, also from %s", opID, goName, prev.loc))
						} else if !dup {
							seenGoName[goName] = opIDRef{opID: opID, loc: loc}
						}
					}
				}

				auth, permKey, permText, permFrom, hasXPerm := checkOpenAPISecurity(loc, spec, op, pkgName, addErr)

				routeKey := ""
				if rawKey, hasXKey := op.Extensions["x-route-key"]; hasXKey {
					xk, ok := rawKey.(string)
					if !ok {
						xk = fmt.Sprint(rawKey)
					}
					if !directive.IsValidKey(xk) {
						addErr(loc, fmt.Sprintf("x-route-key %q is not a key", xk))
					} else {
						routeKey = xk
					}
				} else if goName != "" {
					derived := deriveRouteKey(opID)
					if keyPrefix != "" {
						routeKey = keyPrefix + "." + derived
					} else {
						routeKey = derived
					}
				}

				if routeKey != "" {
					if firstOpID, dup := seenRouteKey[routeKey]; dup {
						addErr(loc, fmt.Sprintf("key %q also from operationId %q", routeKey, firstOpID))
					} else {
						seenRouteKey[routeKey] = opID
					}
				}

				if auth == "permission" && !hasXPerm {
					permKey = routeKey
				}

				label := resolveOpenAPIOpLabel(loc, op, opID, addErr)
				maxBody := checkOpenAPIMaxBody(loc, method, op, addErr)

				// Finalize parameter Var now that goName is known.
				for i := range params {
					params[i].Var = "L10n" + goName + upperFirst(params[i].Name)
				}

				ops = append(ops, &openAPIOpInfo{
					loc:      loc,
					method:   method,
					path:     pathStr,
					op:       op,
					pathItem: pathItem,
					opID:     opID,
					goName:   goName,
					routeKey: routeKey,
					auth:     auth,
					permKey:  permKey,
					permText: permText,
					permFrom: permFrom,
					hasXPerm: hasXPerm,
					label:    label,
					maxBody:  maxBody,
					params:   params,
				})
			}
		}
	}

	// Count permissions and check label/permission variable collisions.
	pb := newPermBuilder()
	permVars := make(map[string]string) // Go name -> perm key
	type labelOwner struct {
		key string
		loc string
	}
	labelVars := make(map[string]labelOwner)

	type openAPIPermTrack struct {
		hasDecl     bool
		declLoc     string
		declCount   int
		declText    string
		declTextLoc string
		hasTaken    bool
		takenPkg    string
		takenLoc    string
	}
	type openAPITakenOccur struct {
		permKey string
		pkg     string
		loc     string
	}
	permTracks := make(map[string]*openAPIPermTrack)
	var takenOccurs []openAPITakenOccur

	for _, info := range ops {
		if info.auth == "permission" && info.permKey != "" {
			pt := permTracks[info.permKey]
			if pt == nil {
				pt = &openAPIPermTrack{}
				permTracks[info.permKey] = pt
			}
			if info.permFrom != "" {
				takenOccurs = append(takenOccurs, openAPITakenOccur{
					permKey: info.permKey,
					pkg:     info.permFrom,
					loc:     info.loc,
				})
				if pt.hasDecl {
					addErr(info.loc, fmt.Sprintf("permission %q is declared in this package at %s, so it can't be taken from %s", info.permKey, pt.declLoc, info.permFrom))
				} else if pt.hasTaken && pt.takenPkg != info.permFrom {
					addErr(info.loc, fmt.Sprintf("permission %q is taken from %s at %s, not %s", info.permKey, pt.takenPkg, pt.takenLoc, info.permFrom))
				} else if !pt.hasTaken {
					pt.hasTaken = true
					pt.takenPkg = info.permFrom
					pt.takenLoc = info.loc
				}
				pb.addTaken(info.permKey, info.permFrom, false)
			} else {
				pt.declCount++
				if !pt.hasDecl {
					pt.hasDecl = true
					pt.declLoc = info.loc
					for _, prevTaken := range takenOccurs {
						if prevTaken.permKey == info.permKey {
							addErr(prevTaken.loc, fmt.Sprintf("permission %q is declared in this package at %s, so it can't be taken from %s", info.permKey, info.loc, prevTaken.pkg))
						}
					}
				}
				if info.permText != "" {
					if pt.declText != "" && pt.declText != info.permText {
						addErr(info.loc, fmt.Sprintf("permission %q: text %q differs from %q at %s", info.permKey, info.permText, pt.declText, pt.declTextLoc))
					} else if pt.declText == "" {
						pt.declText = info.permText
						pt.declTextLoc = info.loc
					}
				}
				pb.addDeclared(info.permKey, info.label, info.permText, false)

				gn := permGoName(info.permKey)
				if prev, clash := permVars[gn]; clash && prev != info.permKey {
					addErr(info.loc, fmt.Sprintf("keys %q and %q both give the Go name %s", prev, info.permKey, gn))
				} else {
					permVars[gn] = info.permKey
				}
			}
		}
		if info.goName != "" && info.routeKey != "" {
			rVar := "L10n" + info.goName
			if prev, clash := labelVars[rVar]; clash {
				addErr(info.loc, fmt.Sprintf("keys %q and %q both give the Go name %s", prev.key, info.routeKey, rVar))
			} else {
				labelVars[rVar] = labelOwner{key: info.routeKey, loc: info.loc}
			}
			for _, p := range info.params {
				if prev, clash := labelVars[p.Var]; clash {
					addErr(info.loc, fmt.Sprintf("keys %q and %q both give the Go name %s", prev.key, info.routeKey, p.Var))
				} else {
					labelVars[p.Var] = labelOwner{key: info.routeKey, loc: info.loc}
				}
			}
		}
	}

	for _, info := range ops {
		if info.goName == "" {
			continue
		}
		if lv, ok := labelVars[info.goName]; ok {
			addErr(info.loc, fmt.Sprintf("%s is also the label variable of %s", info.goName, lv.key))
		}
		if pk, ok := permVars[info.goName]; ok {
			addErr(info.loc, fmt.Sprintf("%s is also the variable of permission %s", info.goName, pk))
		}
	}

	for _, info := range ops {
		if info.auth == "permission" && info.hasXPerm && info.permKey != "" && info.permFrom == "" {
			pt := permTracks[info.permKey]
			ext := 0
			if externalTakes != nil {
				ext = externalTakes[info.permKey]
			}
			if pt != nil && pt.declCount+ext == 1 {
				addWarn(info.loc, fmt.Sprintf("x-permission %q is used by this operation only", info.permKey))
			}
		}
	}

	sortOpenAPIDiags(diags)

	for _, d := range diags {
		if d.severity == directive.Error {
			return routesDoc{}, diags
		}
	}

	routes := make([]routeDoc, 0, len(ops))
	for _, info := range ops {
		routes = append(routes, routeDoc{
			Key:        info.routeKey,
			Method:     info.method,
			Pattern:    info.path,
			Name:       info.goName,
			Var:        "L10n" + info.goName,
			Text:       info.label,
			Auth:       info.auth,
			View:       false,
			Search:     false,
			Params:     info.params,
			Permission: info.permKey,
			MaxBody:    info.maxBody,
			Operation:  info.opID,
		})
	}

	doc := routesDoc{
		Domain:      "routes/v1",
		Producer:    "handlers",
		Source:      sourceRel,
		Package:     packageDoc{Name: pkgName, Dir: outRel},
		Surface:     "json",
		Prefix:      prefix,
		Routes:      routes,
		Permissions: pb.docs(),
	}

	return doc, diags
}

func parseOpenAPIPrefix(spec *openapi3.T, addErr func(loc, msg string)) string {
	if len(spec.Servers) == 0 || spec.Servers[0] == nil {
		return ""
	}
	raw := spec.Servers[0].URL
	if raw == "" {
		addErr("servers[0].url", fmt.Sprintf("%q is not a URL", raw))
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		addErr("servers[0].url", fmt.Sprintf("%q is not a URL", raw))
		return ""
	}
	if strings.ContainsAny(u.Path, "{}") || (len(spec.Servers[0].Variables) > 0 && strings.ContainsAny(raw, "{}")) {
		addErr("servers[0].url", fmt.Sprintf("%q: the path must not hold a variable", raw))
		return ""
	}
	if strings.HasSuffix(u.Path, "/") {
		addErr("servers[0].url", fmt.Sprintf(`%q: the path must not end with "/"`, raw))
		return ""
	}
	if u.Path != "" && (!strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, "//")) {
		addErr("servers[0].url", fmt.Sprintf("%q is not a URL", raw))
		return ""
	}
	return u.Path
}

func isValidErrorSchema(ref *openapi3.SchemaRef) bool {
	if ref == nil || ref.Value == nil {
		return false
	}
	s := ref.Value
	if s.Type != nil && !s.Type.Is("object") {
		return false
	}
	return hasStringProp(s.Properties, "code") && hasStringProp(s.Properties, "message")
}

func hasStringProp(props openapi3.Schemas, name string) bool {
	p := props[name]
	if p == nil || p.Value == nil || p.Value.Type == nil {
		return false
	}
	return p.Value.Type.Is("string")
}

type methodOp struct {
	method string
	op     *openapi3.Operation
}

func orderedOperations(pi *openapi3.PathItem) []methodOp {
	ops := pi.Operations()
	out := make([]methodOp, 0, len(ops))
	seen := make(map[string]bool, len(canonicalMethods))
	for _, m := range canonicalMethods {
		if op := ops[m]; op != nil {
			out = append(out, methodOp{method: m, op: op})
			seen[m] = true
		}
	}
	var extras []string
	for m := range ops {
		if !seen[m] {
			extras = append(extras, m)
		}
	}
	slices.Sort(extras)
	for _, m := range extras {
		if op := ops[m]; op != nil {
			out = append(out, methodOp{method: m, op: op})
		}
	}
	return out
}

func isSupportedMethod(m string) bool {
	return slices.Contains(canonicalMethods, m)
}

func checkErrorResponses(loc string, op *openapi3.Operation, errSchemaRef *openapi3.SchemaRef, addErr func(loc, msg string)) {
	if op.Responses == nil {
		return
	}
	respMap := op.Responses.Map()
	codes := slices.Sorted(maps.Keys(respMap))
	for _, code := range codes {
		if !isErrorStatusCode(code) {
			continue
		}
		ref := respMap[code]
		if ref == nil || ref.Value == nil || len(ref.Value.Content) == 0 {
			continue
		}
		mimes := slices.Sorted(maps.Keys(ref.Value.Content))
		for _, mime := range mimes {
			if !isJSONMediaType(mime) {
				continue
			}
			mt := ref.Value.Content[mime]
			if mt == nil || mt.Schema == nil || !usesErrorSchema(mt.Schema, errSchemaRef) {
				addErr(loc, fmt.Sprintf("response %s doesn't use #/components/schemas/Error", code))
				break
			}
		}
	}
}

func isErrorStatusCode(code string) bool {
	if code == "default" || strings.EqualFold(code, "4XX") || strings.EqualFold(code, "5XX") {
		return true
	}
	if len(code) != 3 {
		return false
	}
	if code[0] != '4' && code[0] != '5' {
		return false
	}
	return code[1] >= '0' && code[1] <= '9' && code[2] >= '0' && code[2] <= '9'
}

func isJSONMediaType(mime string) bool {
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(mime)), ";")
	base = strings.TrimSpace(base)
	return base == "application/json" || strings.HasSuffix(base, "+json")
}

func usesErrorSchema(sref *openapi3.SchemaRef, errSchemaRef *openapi3.SchemaRef) bool {
	if sref.Ref == "#/components/schemas/Error" {
		return true
	}
	return errSchemaRef != nil && errSchemaRef.Value != nil && sref.Value == errSchemaRef.Value
}

func checkOpenAPIPathSegments(loc, pathStr string, addErr func(loc, msg string)) ([]string, bool) {
	if pathStr == "/" {
		return nil, true
	}
	ok := true
	if !strings.HasPrefix(pathStr, "/") {
		ok = false
		addErr(loc, fmt.Sprintf(`path %q must start with "/"`, pathStr))
	}
	if len(pathStr) > 1 && strings.HasSuffix(pathStr, "/") {
		ok = false
		addErr(loc, fmt.Sprintf(`path %q must not end with "/"`, pathStr))
	}
	if strings.Contains(pathStr, "//") {
		ok = false
		addErr(loc, fmt.Sprintf(`path %q must not contain "//"`, pathStr))
	}
	if strings.Contains(pathStr, "{$}") {
		ok = false
		addErr(loc, fmt.Sprintf(`path %q must not contain "{$}"`, pathStr))
	}
	var wildcards []string
	for seg := range strings.SplitSeq(strings.TrimPrefix(pathStr, "/"), "/") {
		if !strings.ContainsAny(seg, "{}") {
			continue
		}
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") ||
			strings.Count(seg, "{") != 1 || strings.Count(seg, "}") != 1 {
			ok = false
			for _, name := range extractSegmentBraceNames(seg) {
				addErr(loc, fmt.Sprintf("parameter {%s} doesn't fill its segment", name))
			}
			continue
		}
		name := seg[1 : len(seg)-1]
		wildcards = append(wildcards, name)
		if name == "_" || !isGoIdentifier(name) {
			ok = false
			addErr(loc, fmt.Sprintf("path parameter %q is not a Go identifier", name))
		}
	}
	return wildcards, ok
}

func extractSegmentBraceNames(seg string) []string {
	var names []string
	rest := seg
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			break
		}
		after := rest[open+1:]
		closeIdx := strings.IndexByte(after, '}')
		if closeIdx < 0 {
			names = append(names, after)
			break
		}
		names = append(names, after[:closeIdx])
		rest = after[closeIdx+1:]
	}
	if len(names) == 0 {
		names = append(names, seg)
	}
	return names
}

func checkOpenAPIPathParams(
	loc, pathStr string,
	pi *openapi3.PathItem,
	op *openapi3.Operation,
	wildcards []string,
	pathOK bool,
	addErr func(loc, msg string),
) []paramDoc {
	// PathItem parameters overridden by Operation parameters on (in, name).
	byName := make(map[string]*openapi3.Parameter)
	for _, ref := range pi.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == openapi3.ParameterInPath {
			byName[ref.Value.Name] = ref.Value
		}
	}
	for _, ref := range op.Parameters {
		if ref != nil && ref.Value != nil && ref.Value.In == openapi3.ParameterInPath {
			byName[ref.Value.Name] = ref.Value
		}
	}

	reportedBadIdent := make(map[string]bool)
	for _, wc := range wildcards {
		if wc == "_" || !isGoIdentifier(wc) {
			reportedBadIdent[wc] = true
		}
	}

	// Validate all declared path parameters in deterministic name order.
	names := slices.Sorted(maps.Keys(byName))
	wildcardSet := make(map[string]bool, len(wildcards))
	for _, wc := range wildcards {
		wildcardSet[wc] = true
	}

	mapped := make(map[string]paramDoc, len(byName))
	for _, name := range names {
		p := byName[name]
		if p.AllowReserved {
			addErr(loc, fmt.Sprintf("parameter %q sets allowReserved", name))
		}
		if (name == "_" || !isGoIdentifier(name)) && !reportedBadIdent[name] {
			addErr(loc, fmt.Sprintf("path parameter %q is not a Go identifier", name))
		}
		label := directive.Readable(name)
		if rawLbl, hasLbl := p.Extensions["x-label"]; hasLbl {
			s, ok := rawLbl.(string)
			if !ok || strings.TrimSpace(s) == "" {
				addErr(loc, "x-label is empty")
			} else {
				label = s
			}
		}

		goType, parse, errDesc := mapOpenAPIPathParamSchema(p)
		if errDesc != "" {
			addErr(loc, fmt.Sprintf("path parameter %q: %s isn't supported", name, errDesc))
			continue
		}
		if pathOK && !wildcardSet[name] {
			addErr(loc, fmt.Sprintf("path parameter %q: not in %s isn't supported", name, pathStr))
			continue
		}

		mapped[name] = paramDoc{
			Name:   name,
			Go:     name,
			Type:   goType,
			Import: "",
			Parse:  parse,
			Kind:   "segment",
			Label:  label,
		}
	}

	params := make([]paramDoc, 0)
	if pathOK {
		for _, wc := range wildcards {
			if pd, ok := mapped[wc]; ok {
				params = append(params, pd)
			} else if !reportedBadIdent[wc] && byName[wc] == nil {
				addErr(loc, fmt.Sprintf("path parameter %q: missing parameter definition isn't supported", wc))
			}
		}
	}
	return params
}

func mapOpenAPIPathParamSchema(p *openapi3.Parameter) (goType, parse, errDesc string) {
	if p.Extensions["x-go-type"] != nil {
		return "", "", "x-go-type"
	}
	if p.Schema == nil || p.Schema.Value == nil {
		return "", "", "missing schema"
	}
	s := p.Schema.Value
	if s.Extensions["x-go-type"] != nil {
		return "", "", "x-go-type"
	}
	if s.Type == nil || len(s.Type.Slice()) != 1 {
		return "", "", schemaTypeDesc(s)
	}
	typ := s.Type.Slice()[0]
	switch typ {
	case "string":
		return "string", "string", ""
	case "integer":
		if s.Format == "int32" {
			return "int32", "int32", ""
		}
		return "int64", "int64", ""
	case "boolean":
		return "bool", "bool", ""
	default:
		return "", "", typ
	}
}

func schemaTypeDesc(s *openapi3.Schema) string {
	if s.Type != nil && len(s.Type.Slice()) > 0 {
		return strings.Join(s.Type.Slice(), "/")
	}
	return "schema"
}

func checkOpenAPISecurity(loc string, spec *openapi3.T, op *openapi3.Operation, pkgName string, addErr func(loc, msg string)) (auth, permKey, permText, permFrom string, hasXPerm bool) {
	isPublic := op.Security != nil && len(*op.Security) == 0
	if !isPublic {
		var reqs openapi3.SecurityRequirements
		if op.Security != nil {
			reqs = *op.Security
		} else {
			reqs = spec.Security
		}
		if len(reqs) == 0 {
			addErr(loc, "no security; write security: [] for a public operation")
		} else {
			hasEmpty := false
			for _, req := range reqs {
				if len(req) == 0 {
					hasEmpty = true
				}
			}
			if hasEmpty {
				addErr(loc, "optional security isn't supported")
			}
			for _, req := range reqs {
				for _, schemeName := range slices.Sorted(maps.Keys(req)) {
					var ss *openapi3.SecuritySchemeRef
					if spec.Components != nil && spec.Components.SecuritySchemes != nil {
						ss = spec.Components.SecuritySchemes[schemeName]
					}
					if ss == nil || ss.Value == nil {
						addErr(loc, fmt.Sprintf("security names undeclared scheme %q", schemeName))
						continue
					}
					if ss.Value.Type != "http" || !strings.EqualFold(ss.Value.Scheme, "bearer") {
						addErr(loc, fmt.Sprintf("scheme %q isn't http bearer", schemeName))
					}
				}
			}
		}
	}

	rawAuth, hasXAuth := op.Extensions["x-auth"]
	rawPerm, hasXPermExt := op.Extensions["x-permission"]

	if isPublic {
		if hasXPermExt {
			addErr(loc, "x-permission on a public operation")
		}
		if hasXAuth {
			addErr(loc, "x-auth on a public operation")
		}
		return "public", "", "", "", false
	}

	if hasXAuth {
		xAuth, ok := rawAuth.(string)
		if !ok || xAuth != "session" {
			addErr(loc, "x-auth must be session")
		} else if hasXPermExt {
			addErr(loc, "x-auth: session takes no x-permission")
		}
		if hasXPermExt {
			checkXPermValue(loc, rawPerm, pkgName, addErr)
		}
		return "session", "", "", "", false
	}

	if hasXPermExt {
		pv, valid := checkXPermValue(loc, rawPerm, pkgName, addErr)
		if valid {
			return "permission", pv.Key, pv.Text, pv.From, true
		}
		return "permission", "", "", "", true
	}

	return "permission", "", "", "", false
}

func checkXPermValue(loc string, raw any, pkgName string, addErr func(loc, msg string)) (directive.PermValue, bool) {
	xp, ok := raw.(string)
	if !ok {
		xp = fmt.Sprint(raw)
	}
	pv, errStr := directive.ParsePerm(xp, "x-permission")
	if errStr != "" {
		addErr(loc, errStr)
		return directive.PermValue{}, false
	}
	if pkgName != "" && pv.From != "" && pv.From == pkgName {
		addErr(loc, fmt.Sprintf("x-permission: %s is this routes package; declare the permission here", pv.From))
		return directive.PermValue{}, false
	}
	return pv, true
}

func resolveOpenAPIOpLabel(loc string, op *openapi3.Operation, opID string, addErr func(loc, msg string)) string {
	if raw, ok := op.Extensions["x-label"]; ok {
		s, isStr := raw.(string)
		if !isStr || strings.TrimSpace(s) == "" {
			addErr(loc, "x-label is empty")
		} else {
			return s
		}
	}
	if op.Summary != "" {
		return op.Summary
	}
	if opID != "" {
		return directive.Readable(opID)
	}
	return ""
}

func checkOpenAPIMaxBody(loc, method string, op *openapi3.Operation, addErr func(loc, msg string)) int64 {
	raw, ok := op.Extensions["x-max-body"]
	if !ok {
		return 0
	}
	s, isStr := raw.(string)
	if !isStr {
		addErr(loc, `x-max-body must be a string, as "20MiB"`)
		return 0
	}
	n, errMsg := directive.ParseBodySize(s, "x-max-body")
	if errMsg != "" {
		addErr(loc, errMsg)
		return 0
	}
	if method == "GET" {
		addErr(loc, "x-max-body on a GET route, which has no body")
		return 0
	}
	return n
}

func deriveRouteKey(operationID string) string {
	words := directive.SplitWords(operationID)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "_")
}

func sortOpenAPIDiags(diags []openAPIDiag) {
	sort.Slice(diags, func(i, j int) bool {
		if diags[i].location != diags[j].location {
			return diags[i].location < diags[j].location
		}
		return diags[i].message < diags[j].message
	})
}
