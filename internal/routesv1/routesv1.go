// Package routesv1 validates routes/v1 documents according to rules.md
// and schema/routes.v1.json.
package routesv1

import (
	"errors"
	"fmt"
	"go/token"
	"math"
	"regexp"
	"strings"
)

var (
	keyPattern = regexp.MustCompile(`^[a-z0-9][A-Za-z0-9_]*(\.[a-z0-9][A-Za-z0-9_]*)*$`)
)

type wildcard struct {
	name   string
	isRest bool
}

// Check validates one decoded routes/v1 document against the shape
// in schema/routes.v1.json and the semantic rules in rules.md.
// It returns all problems joined via errors.Join.
func Check(v any) error {
	obj, ok := v.(map[string]any)
	if !ok {
		return errors.New("document: must be an object")
	}

	var errs []error
	add := func(loc, msg string) {
		errs = append(errs, fmt.Errorf("%s: %s", loc, msg))
	}

	// Preliminary scan of route keys, permission keys, and used permissions.
	allRouteKeys := make(map[string]bool)
	if routesRaw, ok := obj["routes"].([]any); ok {
		for _, item := range routesRaw {
			if r, ok := item.(map[string]any); ok {
				if k, ok := r["key"].(string); ok && k != "" {
					allRouteKeys[k] = true
				}
			}
		}
	}

	allPermKeys := make(map[string]bool)
	if permsRaw, ok := obj["permissions"].([]any); ok {
		for _, item := range permsRaw {
			if p, ok := item.(map[string]any); ok {
				if k, ok := p["key"].(string); ok && k != "" {
					allPermKeys[k] = true
				}
			}
		}
	}

	usedPerms := make(map[string]bool)
	if routesRaw, ok := obj["routes"].([]any); ok {
		for _, item := range routesRaw {
			if r, ok := item.(map[string]any); ok {
				if auth, ok := r["auth"].(string); ok && auth == "permission" {
					if perm, ok := r["permission"].(string); ok && perm != "" {
						usedPerms[perm] = true
					}
				}
			}
		}
	}

	// domain: required, exactly "routes/v1"
	if raw, ok := obj["domain"]; !ok {
		add("domain", "required")
	} else if s, ok := raw.(string); !ok {
		add("domain", "must be a string")
	} else if s != "routes/v1" {
		add("domain", fmt.Sprintf("want %q, got %q", "routes/v1", s))
	}

	// producer: required non-empty string
	if raw, ok := obj["producer"]; !ok {
		add("producer", "required")
	} else if s, ok := raw.(string); !ok {
		add("producer", "must be a string")
	} else if s == "" {
		add("producer", "must not be empty")
	}

	// source: required non-empty string, the handler package or the spec
	if raw, ok := obj["source"]; !ok {
		add("source", "required")
	} else if s, ok := raw.(string); !ok {
		add("source", "must be a string")
	} else if s == "" {
		add("source", "must not be empty")
	}

	// package: required object
	docPkgName := ""
	if raw, ok := obj["package"]; !ok {
		add("package", "required")
	} else if pkg, ok := raw.(map[string]any); !ok {
		add("package", "must be an object")
	} else {
		if nameRaw, ok := pkg["name"].(string); ok {
			docPkgName = nameRaw
		}
		checkPackage(pkg, add)
	}

	// surface: required, "html" or "json"
	surface := ""
	if raw, ok := obj["surface"]; !ok {
		add("surface", "required")
	} else if s, ok := raw.(string); !ok {
		add("surface", "must be a string")
	} else if s != "html" && s != "json" {
		add("surface", fmt.Sprintf("unknown surface %q", s))
	} else {
		surface = s
	}

	// prefix: required, "" or starts with "/" and doesn't end with "/", no "//", no "{"
	if raw, ok := obj["prefix"]; !ok {
		add("prefix", "required")
	} else if s, ok := raw.(string); !ok {
		add("prefix", "must be a string")
	} else if s != "" {
		checkPrefix(s, add)
	}

	// State across routes
	seenRouteKey := make(map[string]int)
	seenMethodPattern := make(map[string]int)
	seenRouteName := make(map[string]int)
	seenVar := make(map[string]string)

	// routes: required array of route objects
	if raw, ok := obj["routes"]; !ok {
		add("routes", "required")
	} else if items, ok := raw.([]any); !ok {
		add("routes", "must be an array")
	} else {
		for i, item := range items {
			loc := fmt.Sprintf("routes[%d]", i)
			r, ok := item.(map[string]any)
			if !ok {
				add(loc, "must be an object")
				continue
			}
			checkRoute(i, loc, r, surface, allRouteKeys, allPermKeys, seenRouteKey, seenMethodPattern, seenRouteName, seenVar, add)
		}
		checkSources(items, add)
	}

	// permissions: required array of permission objects
	if raw, ok := obj["permissions"]; !ok {
		add("permissions", "required")
	} else if items, ok := raw.([]any); !ok {
		add("permissions", "must be an array")
	} else {
		seenPermKey := make(map[string]int)
		seenPermVar := make(map[string]int)
		for k, item := range items {
			loc := fmt.Sprintf("permissions[%d]", k)
			p, ok := item.(map[string]any)
			if !ok {
				add(loc, "must be an object")
				continue
			}
			checkPermission(k, loc, p, docPkgName, usedPerms, seenPermKey, seenPermVar, add)
		}
	}

	// Rule 12: an alias names one import path, and an import path has one
	// alias.
	if routesRaw, ok := obj["routes"].([]any); ok {
		pathOf := map[string]string{}  // alias -> import
		aliasOf := map[string]string{} // import -> alias
		for i, item := range routesRaw {
			r, ok := item.(map[string]any)
			if !ok {
				continue
			}
			params, _ := r["params"].([]any)
			for j, pi := range params {
				p, ok := pi.(map[string]any)
				if !ok {
					continue
				}
				imp, _ := p["import"].(string)
				alias, _ := p["alias"].(string)
				if imp == "" || alias == "" {
					continue
				}
				loc := fmt.Sprintf("routes[%d].params[%d].alias", i, j)
				if prev, ok := pathOf[alias]; ok && prev != imp {
					add(loc, fmt.Sprintf("%q also names %q", alias, prev))
				} else if prev, ok := aliasOf[imp]; ok && prev != alias {
					add(loc, fmt.Sprintf("%q is already imported as %q", imp, prev))
				} else {
					pathOf[alias], aliasOf[imp] = imp, alias
				}
			}
		}
	}

	// Rule 11: route names are package-level identifiers of the generated
	// package, beside its fixed names and the variables of this document.
	if routesRaw, ok := obj["routes"].([]any); ok {
		taken := map[string]string{}
		for _, n := range reservedNames {
			taken[n] = "a name the generated package declares"
		}
		if permsRaw, ok := obj["permissions"].([]any); ok {
			for k, item := range permsRaw {
				if p, ok := item.(map[string]any); ok {
					if v, ok := p["var"].(string); ok && v != "" {
						taken[v] = fmt.Sprintf("permissions[%d].var", k)
					}
				}
			}
		}
		for name, loc := range seenVar {
			taken[name] = loc
		}
		for i, item := range routesRaw {
			if r, ok := item.(map[string]any); ok {
				if n, ok := r["name"].(string); ok {
					if where, clash := taken[n]; clash {
						add(fmt.Sprintf("routes[%d].name", i), fmt.Sprintf("%q is also %s", n, where))
					}
				}
			}
		}
	}

	return errors.Join(errs...)
}

// reservedNames are the identifiers every generated routes package
// declares, which a route's name must not take.
var reservedNames = []string{"Handler", "Register", "Mount", "Routes", "ByKey", "Prefix", "L10n"}

func checkPackage(pkg map[string]any, add func(loc, msg string)) {
	if raw, ok := pkg["name"]; !ok {
		add("package.name", "required")
	} else if s, ok := raw.(string); !ok {
		add("package.name", "must be a string")
	} else if s == "" {
		add("package.name", "must not be empty")
	} else if token.IsKeyword(s) {
		add("package.name", fmt.Sprintf("%q is a Go keyword", s))
	} else if !token.IsIdentifier(s) {
		add("package.name", fmt.Sprintf("%q is not a Go identifier", s))
	} else if s == "_" {
		add("package.name", `"_"`+" is the blank identifier")
	}

	if raw, ok := pkg["dir"]; !ok {
		add("package.dir", "required")
	} else if s, ok := raw.(string); !ok {
		add("package.dir", "must be a string")
	} else if s == "" {
		add("package.dir", "must not be empty")
	} else if s != "." {
		checkPackageDir(s, add)
	}
}

func checkPackageDir(s string, add func(loc, msg string)) {
	if strings.Contains(s, "\\") {
		add("package.dir", "must be slash-separated")
		return
	}
	if strings.HasPrefix(s, "/") {
		add("package.dir", "must be relative")
		return
	}
	if strings.HasSuffix(s, "/") {
		add("package.dir", `must not end with "/"`)
		return
	}
	if strings.Contains(s, "//") {
		add("package.dir", `must not contain "//"`)
		return
	}
	for _, elem := range strings.Split(s, "/") {
		if elem == ".." {
			add("package.dir", `must not contain ".."`)
			return
		}
		if elem == "." {
			add("package.dir", `must not contain "." element`)
			return
		}
	}
}

func checkPrefix(s string, add func(loc, msg string)) {
	if !strings.HasPrefix(s, "/") {
		add("prefix", `must start with "/"`)
		return
	}
	if strings.HasSuffix(s, "/") {
		add("prefix", `must not end with "/"`)
		return
	}
	if strings.Contains(s, "//") {
		add("prefix", `must not contain "//"`)
		return
	}
	if strings.Contains(s, "{") {
		add("prefix", `must not contain "{"`)
		return
	}
}

func checkRoute(
	i int,
	loc string,
	r map[string]any,
	surface string,
	allRouteKeys, allPermKeys map[string]bool,
	seenRouteKey, seenMethodPattern, seenRouteName map[string]int,
	seenVar map[string]string,
	add func(loc, msg string),
) {
	// key: dot-separated segments, each [a-z0-9][A-Za-z0-9_]*
	if raw, ok := r["key"]; !ok {
		add(loc+".key", "required")
	} else if s, ok := raw.(string); !ok {
		add(loc+".key", "must be a string")
	} else if s == "" {
		add(loc+".key", "must not be empty")
	} else if !keyPattern.MatchString(s) {
		add(loc+".key", fmt.Sprintf("%q is not a valid key", s))
	} else {
		if first, ok := seenRouteKey[s]; ok {
			add(loc+".key", fmt.Sprintf("%q also at routes[%d]", s, first))
		} else {
			seenRouteKey[s] = i
		}
	}

	// method: GET POST PUT PATCH DELETE (upper case only)
	var method string
	if raw, ok := r["method"]; !ok {
		add(loc+".method", "required")
	} else if s, ok := raw.(string); !ok {
		add(loc+".method", "must be a string")
	} else {
		switch s {
		case "GET", "POST", "PUT", "PATCH", "DELETE":
			method = s
		default:
			add(loc+".method", fmt.Sprintf("unknown method %q", s))
		}
	}

	// pattern
	var pattern string
	var wildcards []wildcard
	var wildcardMap map[string]wildcard
	if raw, ok := r["pattern"]; !ok {
		add(loc+".pattern", "required")
	} else if s, ok := raw.(string); !ok {
		add(loc+".pattern", "must be a string")
	} else {
		pattern = s
		wildcards, wildcardMap = checkPattern(loc, s, method, seenMethodPattern, i, add)
	}

	// name: exported Go identifier
	if raw, ok := r["name"]; !ok {
		add(loc+".name", "required")
	} else if s, ok := raw.(string); !ok {
		add(loc+".name", "must be a string")
	} else if s == "" {
		add(loc+".name", "must not be empty")
	} else if token.IsKeyword(s) {
		add(loc+".name", fmt.Sprintf("%q is a Go keyword", s))
	} else if !token.IsIdentifier(s) {
		add(loc+".name", fmt.Sprintf("%q is not a Go identifier", s))
	} else if !token.IsExported(s) {
		add(loc+".name", fmt.Sprintf("%q is not exported", s))
	} else {
		if first, ok := seenRouteName[s]; ok {
			add(loc+".name", fmt.Sprintf("%q also at routes[%d]", s, first))
		} else {
			seenRouteName[s] = i
		}
	}

	// var: Go identifier starting with L10n
	if raw, ok := r["var"]; !ok {
		add(loc+".var", "required")
	} else {
		checkVar(loc+".var", raw, seenVar, add)
	}

	// text: non-empty string
	if raw, ok := r["text"]; !ok {
		add(loc+".text", "required")
	} else if s, ok := raw.(string); !ok {
		add(loc+".text", "must be a string")
	} else if s == "" {
		add(loc+".text", "must not be empty")
	}

	// auth: public, session or permission
	var auth string
	if raw, ok := r["auth"]; !ok {
		add(loc+".auth", "required")
	} else if s, ok := raw.(string); !ok {
		add(loc+".auth", "must be a string")
	} else {
		switch s {
		case "public", "session", "permission":
			auth = s
		default:
			add(loc+".auth", fmt.Sprintf("unknown auth %q", s))
		}
	}

	// permission: required and non-empty when auth is permission; must be absent when auth is public or session
	if raw, ok := r["permission"]; ok {
		if auth == "permission" {
			if s, ok := raw.(string); !ok {
				add(loc+".permission", "must be a string")
			} else if s == "" {
				add(loc+".permission", "must not be empty")
			} else if !allPermKeys[s] {
				add(loc+".permission", fmt.Sprintf("unknown permission %q", s))
			}
		} else if auth == "public" {
			add(loc+".permission", "must be absent when auth is public")
		} else if auth == "session" {
			add(loc+".permission", "must be absent when auth is session")
		}
	} else {
		if auth == "permission" {
			add(loc+".permission", "required")
		}
	}

	// view: boolean
	if raw, ok := r["view"]; !ok {
		add(loc+".view", "required")
	} else if _, ok := raw.(bool); !ok {
		add(loc+".view", "must be a boolean")
	}

	// search: boolean
	if raw, ok := r["search"]; !ok {
		add(loc+".search", "required")
	} else if _, ok := raw.(bool); !ok {
		add(loc+".search", "must be a boolean")
	}

	// entity: optional non-empty string naming a route key in the document
	if raw, ok := r["entity"]; ok {
		if s, ok := raw.(string); !ok {
			add(loc+".entity", "must be a string")
		} else if s == "" {
			add(loc+".entity", "must not be empty")
		} else if !allRouteKeys[s] {
			add(loc+".entity", fmt.Sprintf("unknown route key %q", s))
		}
	}

	// maxBody: optional integer >= 1, absent when method is GET
	if raw, ok := r["maxBody"]; ok {
		if method == "GET" {
			add(loc+".maxBody", "must be absent when method is GET")
		} else if f, ok := raw.(float64); !ok || math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
			add(loc+".maxBody", "must be an integer")
		} else if f < 1 {
			add(loc+".maxBody", "must be at least 1")
		}
	}

	// operation: optional non-empty string; required when surface is json, absent when html
	if raw, ok := r["operation"]; ok {
		if surface == "html" {
			add(loc+".operation", "must be absent when surface is html")
		} else if s, ok := raw.(string); !ok {
			add(loc+".operation", "must be a string")
		} else if s == "" {
			add(loc+".operation", "must not be empty")
		}
	} else {
		if surface == "json" {
			add(loc+".operation", "required")
		}
	}

	// params: required array
	if raw, ok := r["params"]; !ok {
		add(loc+".params", "required")
	} else if items, ok := raw.([]any); !ok {
		add(loc+".params", "must be an array")
	} else {
		seenParamName := make(map[string]int)
		matchedWildcards := make(map[string]bool)

		for j, item := range items {
			pLoc := fmt.Sprintf("%s.params[%d]", loc, j)
			p, ok := item.(map[string]any)
			if !ok {
				add(pLoc, "must be an object")
				continue
			}
			checkParam(pLoc, p, pattern, wildcardMap, matchedWildcards, seenParamName, seenVar, j, add)
		}

		for _, wc := range wildcards {
			if !matchedWildcards[wc.name] {
				add(loc+".pattern", fmt.Sprintf("wildcard {%s} has no param", wc.name))
			}
		}
	}
}

func checkPattern(loc, s, method string, seenMethodPattern map[string]int, i int, add func(loc, msg string)) ([]wildcard, map[string]wildcard) {
	valid := true
	if !strings.HasPrefix(s, "/") {
		add(loc+".pattern", `must start with "/"`)
		valid = false
	}
	if len(s) > 1 && strings.HasSuffix(s, "/") {
		add(loc+".pattern", `must not end with "/"`)
		valid = false
	}
	if strings.Contains(s, "//") {
		add(loc+".pattern", `must not contain "//"`)
		valid = false
	}
	if strings.Contains(s, "{$}") {
		add(loc+".pattern", `must not contain "{$}"`)
		valid = false
	}

	if method != "" && valid {
		pair := method + " " + s
		if first, ok := seenMethodPattern[pair]; ok {
			add(loc+".pattern", fmt.Sprintf("%s also at routes[%d]", pair, first))
		} else {
			seenMethodPattern[pair] = i
		}
	}

	if !valid {
		return nil, nil
	}

	var wildcards []wildcard
	wildcardMap := make(map[string]wildcard)
	seenWildcard := make(map[string]bool)

	if s == "/" {
		return wildcards, wildcardMap
	}

	segs := strings.Split(s[1:], "/")
	for segIdx, seg := range segs {
		hasBrace := strings.Contains(seg, "{") || strings.Contains(seg, "}")
		if !hasBrace {
			continue
		}
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") ||
			strings.Count(seg, "{") != 1 || strings.Count(seg, "}") != 1 {
			add(loc+".pattern", "wildcard must fill a whole segment")
			continue
		}
		inner := seg[1 : len(seg)-1]
		isRest := false
		name := inner
		if strings.HasSuffix(inner, "...") {
			isRest = true
			name = strings.TrimSuffix(inner, "...")
			if segIdx != len(segs)-1 {
				add(loc+".pattern", fmt.Sprintf("rest wildcard {%s...} must be the last segment", name))
			}
		} else if strings.Contains(inner, "...") {
			add(loc+".pattern", fmt.Sprintf("invalid wildcard %q", seg))
			continue
		}

		identValid := true
		if name == "" {
			add(loc+".pattern", "wildcard name must not be empty")
			identValid = false
		} else if token.IsKeyword(name) {
			add(loc+".pattern", fmt.Sprintf("%q in wildcard is a Go keyword", name))
			identValid = false
		} else if !token.IsIdentifier(name) {
			add(loc+".pattern", fmt.Sprintf("%q in wildcard is not a Go identifier", name))
			identValid = false
		} else if name == "_" {
			add(loc+".pattern", `"_"`+" in wildcard is the blank identifier")
			identValid = false
		}

		if seenWildcard[name] {
			add(loc+".pattern", fmt.Sprintf("duplicate wildcard {%s}", name))
		} else if identValid {
			seenWildcard[name] = true
			wc := wildcard{name: name, isRest: isRest}
			wildcards = append(wildcards, wc)
			wildcardMap[name] = wc
		}
	}

	return wildcards, wildcardMap
}

func checkVar(loc string, raw any, seenVar map[string]string, add func(loc, msg string)) {
	s, ok := raw.(string)
	if !ok {
		add(loc, "must be a string")
		return
	}
	if s == "" {
		add(loc, "must not be empty")
		return
	}
	if s == "L10n" || s == "l10n" {
		add(loc, fmt.Sprintf("%q is reserved", s))
		return
	}
	if !strings.HasPrefix(s, "L10n") {
		add(loc, `must start with "L10n"`)
		return
	}
	if token.IsKeyword(s) {
		add(loc, fmt.Sprintf("%q is a Go keyword", s))
		return
	}
	if !token.IsIdentifier(s) {
		add(loc, fmt.Sprintf("%q is not a Go identifier", s))
		return
	}
	if firstLoc, ok := seenVar[s]; ok {
		add(loc, fmt.Sprintf("%q also at %s", s, firstLoc))
	} else {
		seenVar[s] = loc
	}
}

func checkParam(
	pLoc string,
	p map[string]any,
	pattern string,
	wildcardMap map[string]wildcard,
	matchedWildcards map[string]bool,
	seenParamName map[string]int,
	seenVar map[string]string,
	j int,
	add func(loc, msg string),
) {
	var paramName string

	// name: Go identifier
	if raw, ok := p["name"]; !ok {
		add(pLoc+".name", "required")
	} else if s, ok := raw.(string); !ok {
		add(pLoc+".name", "must be a string")
	} else if s == "" {
		add(pLoc+".name", "must not be empty")
	} else if token.IsKeyword(s) {
		add(pLoc+".name", fmt.Sprintf("%q is a Go keyword", s))
	} else if !token.IsIdentifier(s) {
		add(pLoc+".name", fmt.Sprintf("%q is not a Go identifier", s))
	} else if s == "_" {
		add(pLoc+".name", `"_"`+" is the blank identifier")
	} else {
		paramName = s
		if first, ok := seenParamName[s]; ok {
			parentRoute := pLoc[:strings.LastIndex(pLoc, ".")]
			add(pLoc+".name", fmt.Sprintf("%q also at %s.params[%d]", s, parentRoute, first))
		} else {
			seenParamName[s] = j
		}
	}

	// go: Go identifier
	if raw, ok := p["go"]; !ok {
		add(pLoc+".go", "required")
	} else if s, ok := raw.(string); !ok {
		add(pLoc+".go", "must be a string")
	} else if s == "" {
		add(pLoc+".go", "must not be empty")
	} else if token.IsKeyword(s) {
		add(pLoc+".go", fmt.Sprintf("%q is a Go keyword", s))
	} else if !token.IsIdentifier(s) {
		add(pLoc+".go", fmt.Sprintf("%q is not a Go identifier", s))
	} else if s == "_" {
		add(pLoc+".go", `"_"`+" is the blank identifier")
	}

	// var: Go identifier starting with L10n
	if raw, ok := p["var"]; !ok {
		add(pLoc+".var", "required")
	} else {
		checkVar(pLoc+".var", raw, seenVar, add)
	}

	// type: non-empty string
	if raw, ok := p["type"]; !ok {
		add(pLoc+".type", "required")
	} else if s, ok := raw.(string); !ok {
		add(pLoc+".type", "must be a string")
	} else if s == "" {
		add(pLoc+".type", "must not be empty")
	}

	// import: string ("" for none); alias: the qualifier the type uses,
	// present exactly when import isn't ""
	importPath := ""
	if raw, ok := p["import"]; !ok {
		add(pLoc+".import", "required")
	} else if s, ok := raw.(string); !ok {
		add(pLoc+".import", "must be a string")
	} else {
		importPath = s
	}
	if raw, ok := p["alias"]; ok {
		if s, ok := raw.(string); !ok {
			add(pLoc+".alias", "must be a string")
		} else if importPath == "" {
			add(pLoc+".alias", "must be absent without an import")
		} else if !token.IsIdentifier(s) || s == "_" {
			add(pLoc+".alias", fmt.Sprintf("%q is not a Go identifier", s))
		} else if t, ok := p["type"].(string); ok && !strings.HasPrefix(t, s+".") {
			add(pLoc+".type", fmt.Sprintf("%q must be qualified by its alias %q", t, s))
		}
	} else if importPath != "" {
		add(pLoc+".alias", "required with an import")
	}

	// parse: one of string int uint bool text
	if raw, ok := p["parse"]; !ok {
		add(pLoc+".parse", "required")
	} else if s, ok := raw.(string); !ok {
		add(pLoc+".parse", "must be a string")
	} else {
		switch s {
		case "string", "bool", "text", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
			// valid
		default:
			add(pLoc+".parse", fmt.Sprintf("unknown parse %q", s))
		}
	}

	// kind: segment or rest
	var kind string
	if raw, ok := p["kind"]; !ok {
		add(pLoc+".kind", "required")
	} else if s, ok := raw.(string); !ok {
		add(pLoc+".kind", "must be a string")
	} else {
		switch s {
		case "segment", "rest":
			kind = s
		default:
			add(pLoc+".kind", fmt.Sprintf("unknown kind %q", s))
		}
	}

	// label: non-empty string
	if raw, ok := p["label"]; !ok {
		add(pLoc+".label", "required")
	} else if s, ok := raw.(string); !ok {
		add(pLoc+".label", "must be a string")
	} else if s == "" {
		add(pLoc+".label", "must not be empty")
	}

	// Rule 5: param matches a wildcard in the pattern
	if paramName != "" && pattern != "" {
		if wc, ok := wildcardMap[paramName]; ok {
			matchedWildcards[paramName] = true
			// Rule 6: param's kind is rest exactly when its wildcard is {name...}
			if kind != "" {
				if wc.isRest && kind != "rest" {
					add(pLoc+".kind", fmt.Sprintf("want %q for wildcard {%s...}", "rest", paramName))
				} else if !wc.isRest && kind != "segment" {
					add(pLoc+".kind", fmt.Sprintf("want %q for wildcard {%s}", "segment", paramName))
				}
			}
		} else {
			add(pLoc, fmt.Sprintf("no wildcard {%s} in %s", paramName, pattern))
		}
	}
}

func checkPermission(
	k int,
	loc string,
	p map[string]any,
	docPkgName string,
	usedPerms map[string]bool,
	seenPermKey, seenPermVar map[string]int,
	add func(loc, msg string),
) {
	var permKey string

	// key: dot-separated segments, each [a-z0-9][A-Za-z0-9_]*
	if raw, ok := p["key"]; !ok {
		add(loc+".key", "required")
	} else if s, ok := raw.(string); !ok {
		add(loc+".key", "must be a string")
	} else if s == "" {
		add(loc+".key", "must not be empty")
	} else if !keyPattern.MatchString(s) {
		add(loc+".key", fmt.Sprintf("%q is not a valid key", s))
	} else {
		permKey = s
		if first, ok := seenPermKey[s]; ok {
			add(loc+".key", fmt.Sprintf("%q also at permissions[%d]", s, first))
		} else {
			seenPermKey[s] = k
		}
	}

	rawFrom, hasFrom := p["from"]
	if hasFrom {
		if s, ok := rawFrom.(string); !ok {
			add(loc+".from", "must be a string")
		} else if s == "" {
			add(loc+".from", "must not be empty")
		} else if token.IsKeyword(s) {
			add(loc+".from", fmt.Sprintf("%q is a Go keyword", s))
		} else if !token.IsIdentifier(s) {
			add(loc+".from", fmt.Sprintf("%q is not a Go identifier", s))
		} else if s == "_" {
			add(loc+".from", `"_"`+" is the blank identifier")
		} else if docPkgName != "" && s == docPkgName {
			add(loc+".from", fmt.Sprintf("%q is the document's own package name", s))
		}

		if _, ok := p["var"]; ok {
			add(loc+".var", "must be absent with from")
		}
		if _, ok := p["text"]; ok {
			add(loc+".text", "must be absent with from")
		}
	} else {
		// var: Go identifier starting with Perm
		if raw, ok := p["var"]; !ok {
			add(loc+".var", "required")
		} else if s, ok := raw.(string); !ok {
			add(loc+".var", "must be a string")
		} else if s == "" {
			add(loc+".var", "must not be empty")
		} else if !strings.HasPrefix(s, "Perm") {
			add(loc+".var", `must start with "Perm"`)
		} else if s == "Perm" {
			add(loc+".var", `must not be empty after "Perm"`)
		} else if token.IsKeyword(s) {
			add(loc+".var", fmt.Sprintf("%q is a Go keyword", s))
		} else if !token.IsIdentifier(s) {
			add(loc+".var", fmt.Sprintf("%q is not a Go identifier", s))
		} else {
			if first, ok := seenPermVar[s]; ok {
				add(loc+".var", fmt.Sprintf("%q also at permissions[%d]", s, first))
			} else {
				seenPermVar[s] = k
			}
		}

		// text: non-empty string
		if raw, ok := p["text"]; !ok {
			add(loc+".text", "required")
		} else if s, ok := raw.(string); !ok {
			add(loc+".text", "must be a string")
		} else if s == "" {
			add(loc+".text", "must not be empty")
		}
	}

	// level: verified or unverified
	if raw, ok := p["level"]; !ok {
		add(loc+".level", "required")
	} else if s, ok := raw.(string); !ok {
		add(loc+".level", "must be a string")
	} else {
		switch s {
		case "verified", "unverified":
			// valid
		default:
			add(loc+".level", fmt.Sprintf("unknown level %q", s))
		}
	}

	// Rule 9: every entry is named by at least one route (permissions[2]: no route uses "x")
	if permKey != "" && !usedPerms[permKey] {
		add(loc, fmt.Sprintf("no route uses %q", permKey))
	}
}

// checkSources checks every param's optional from (A9): a string naming a
// GET route of the document without params, other than the param's own.
func checkSources(routes []any, add func(loc, msg string)) {
	type source struct {
		method  string
		nparams int
	}
	byKey := make(map[string]source)
	for _, item := range routes {
		r, ok := item.(map[string]any)
		if !ok {
			continue
		}
		k, _ := r["key"].(string)
		if _, seen := byKey[k]; k == "" || seen {
			continue
		}
		m, _ := r["method"].(string)
		ps, _ := r["params"].([]any)
		byKey[k] = source{method: m, nparams: len(ps)}
	}
	for i, item := range routes {
		r, ok := item.(map[string]any)
		if !ok {
			continue
		}
		own, _ := r["key"].(string)
		ps, _ := r["params"].([]any)
		for j, pi := range ps {
			p, ok := pi.(map[string]any)
			if !ok {
				continue
			}
			raw, ok := p["from"]
			if !ok {
				continue
			}
			loc := fmt.Sprintf("routes[%d].params[%d].from", i, j)
			k, ok := raw.(string)
			if !ok {
				add(loc, "must be a string")
				continue
			}
			src, found := byKey[k]
			switch {
			case k == "":
				add(loc, "must not be empty")
			case k == own:
				add(loc, fmt.Sprintf("%q is the param's own route", k))
			case !found:
				add(loc, fmt.Sprintf("unknown route key %q", k))
			case src.method != "GET":
				add(loc, fmt.Sprintf("%q is %s, not GET", k, src.method))
			case src.nparams > 0:
				add(loc, fmt.Sprintf("%q has params", k))
			}
		}
	}
}
