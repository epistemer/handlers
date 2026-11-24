package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/epistemer/handlers/cmd/internal/directive"
	"github.com/epistemer/handlers/internal/routesv1"
)

type routesDoc struct {
	Domain      string     `json:"domain"`
	Producer    string     `json:"producer"`
	Source      string     `json:"source"`
	Package     packageDoc `json:"package"`
	Surface     string     `json:"surface"`
	Prefix      string     `json:"prefix"`
	Routes      []routeDoc `json:"routes"`
	Permissions []permDoc  `json:"permissions"`
}

type packageDoc struct {
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

type routeDoc struct {
	Key        string     `json:"key"`
	Method     string     `json:"method"`
	Pattern    string     `json:"pattern"`
	Name       string     `json:"name"`
	Var        string     `json:"var"`
	Text       string     `json:"text"`
	Auth       string     `json:"auth"`
	View       bool       `json:"view"`
	Search     bool       `json:"search"`
	Params     []paramDoc `json:"params"`
	Permission string     `json:"permission,omitempty"`
	Entity     string     `json:"entity,omitempty"`
	MaxBody    int64      `json:"maxBody,omitempty"`
	Operation  string     `json:"operation,omitempty"`
}

type paramDoc struct {
	Name   string `json:"name"`
	Go     string `json:"go"`
	Var    string `json:"var"`
	Type   string `json:"type"`
	Import string `json:"import"`
	Parse  string `json:"parse"`
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Alias  string `json:"alias,omitempty"`
}

type permDoc struct {
	Key   string `json:"key"`
	Var   string `json:"var"`
	Text  string `json:"text"`
	Level string `json:"level"`
}

var reservedAliases = map[string]bool{
	"handlers": true,
	"http":     true,
	"strconv":  true,
	"url":      true,
	"access":   true,
	"auth":     true,
	"openapi":  true,
	"openapi3": true,
}

type routesOutputConfig struct {
	pkgName    string
	outDir     string
	absOutDir  string
	outFile    string
	absOutFile string
	outRel     string
	sourceRel  string
	modRoot    string
}

func resolveRoutesPackageName(pkgNameFlag, outDir string) (string, bool) {
	pkgName := pkgNameFlag
	if pkgName == "" {
		pkgName = filepath.Base(filepath.Clean(outDir))
	}
	if !isGoIdentifier(pkgName) || pkgName == "_" {
		return pkgName, false
	}
	return pkgName, true
}

func resolveRoutesPaths(outDir, pkgName, targetAbs, modRootStartDir string) (routesOutputConfig, error) {
	absOutDir, err := filepath.Abs(outDir)
	if err != nil {
		return routesOutputConfig{}, err
	}
	modRoot, err := findModuleRootFromCwdOr(modRootStartDir)
	if err != nil {
		return routesOutputConfig{}, err
	}
	sourceRel, err := relSlash(modRoot, targetAbs)
	if err != nil {
		return routesOutputConfig{}, err
	}
	outRel, err := relSlash(modRoot, absOutDir)
	if err != nil {
		return routesOutputConfig{}, err
	}
	return routesOutputConfig{
		pkgName:    pkgName,
		outDir:     outDir,
		absOutDir:  absOutDir,
		outFile:    filepath.Join(outDir, "zz_"+pkgName+".routes.json"),
		absOutFile: filepath.Join(absOutDir, "zz_"+pkgName+".routes.json"),
		outRel:     outRel,
		sourceRel:  sourceRel,
		modRoot:    modRoot,
	}, nil
}

func writeOrCheckRoutesDoc(
	cmdName string,
	cfg routesOutputConfig,
	doc routesDoc,
	warnLines []string,
	baseline string,
	strict, check bool,
	stdout, stderr io.Writer,
) int {
	encoded, err := encodeRoutesDoc(doc)
	if err != nil {
		fmt.Fprintf(stderr, "handlers %s: internal error: %v\n", cmdName, err)
		return 1
	}

	var rawAny any
	if err := json.Unmarshal(encoded, &rawAny); err != nil {
		fmt.Fprintf(stderr, "handlers %s: internal error: %v\n", cmdName, err)
		return 1
	}
	if err := routesv1.Check(rawAny); err != nil {
		fmt.Fprintf(stderr, "handlers %s: internal error: %v\n", cmdName, err)
		return 1
	}

	var baseBytes []byte
	var baseLabel string
	if baseline != "" {
		baseLabel = baseline
		if b, ok := loadGitFile(baseline, cfg.absOutFile); ok {
			baseBytes = b
		}
	} else {
		baseLabel = "the committed file"
		if b, err := os.ReadFile(cfg.absOutFile); err == nil {
			baseBytes = b
		}
	}
	if len(baseBytes) > 0 {
		for _, d := range compareBaseline(baseBytes, baseLabel, cfg.outFile, doc) {
			warnLines = append(warnLines, d.String())
		}
	}

	for _, line := range warnLines {
		fmt.Fprintln(stdout, line)
	}

	hasWarning := len(warnLines) > 0

	if check {
		existing, err := os.ReadFile(cfg.absOutFile)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(stderr, "%s is missing\n", cfg.outFile)
				return 1
			}
			fmt.Fprintf(stderr, "handlers %s: %v\n", cmdName, err)
			return 1
		}
		if !bytes.Equal(existing, encoded) {
			fmt.Fprintf(stderr, "%s is stale\n", cfg.outFile)
			return 1
		}
		if strict && hasWarning {
			return 1
		}
		return 0
	}

	if strict && hasWarning {
		return 1
	}

	existing, err := os.ReadFile(cfg.absOutFile)
	if err == nil && bytes.Equal(existing, encoded) {
		return 0
	}

	if err := os.MkdirAll(cfg.absOutDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "handlers %s: %v\n", cmdName, err)
		return 1
	}
	if err := os.WriteFile(cfg.absOutFile, encoded, 0o644); err != nil {
		fmt.Fprintf(stderr, "handlers %s: %v\n", cmdName, err)
		return 1
	}
	return 0
}

func runGen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outDir := fs.String("out", "", "output directory (required)")
	pkgNameFlag := fs.String("package", "", "generated package name (default: last element of -out)")
	prefix := fs.String("prefix", "", "mount prefix path")
	check := fs.Bool("check", false, "check whether the output file is up to date")
	strict := fs.Bool("strict", false, "treat warnings as errors")
	baseline := fs.String("baseline", "", "git ref baseline for removal warnings")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *outDir == "" || fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: handlers gen -out <dir> [-package <name>] [-prefix <path>] [-check] [-strict] [-baseline <ref>] <package dir>")
		return 2
	}
	if err := validatePrefix(*prefix); err != nil {
		fmt.Fprintf(stderr, "handlers gen: -prefix: %v\n", err)
		return 2
	}

	pkgName, ok := resolveRoutesPackageName(*pkgNameFlag, *outDir)
	if !ok {
		fmt.Fprintf(stderr, "handlers gen: invalid package name %q\n", pkgName)
		return 2
	}

	pkgDir := fs.Arg(0)
	absPkgDir, err := filepath.Abs(pkgDir)
	if err != nil {
		fmt.Fprintf(stderr, "handlers gen: %v\n", err)
		return 1
	}
	cfg, err := resolveRoutesPaths(*outDir, pkgName, absPkgDir, absPkgDir)
	if err != nil {
		fmt.Fprintf(stderr, "handlers gen: %v\n", err)
		return 1
	}

	pkg, diags, err := analyzePackageDir(pkgDir, cfg.absOutDir)
	if err != nil {
		fmt.Fprintf(stderr, "handlers gen: %v\n", err)
		return 1
	}

	hasError := false
	for _, d := range diags {
		if d.Severity == directive.Error {
			hasError = true
			break
		}
	}
	if hasError {
		for _, d := range diags {
			fmt.Fprintln(stdout, d.String())
		}
		return 1
	}

	var warnLines []string
	for _, d := range diags {
		warnLines = append(warnLines, d.String())
	}

	doc := buildRoutesDoc(pkg, cfg.sourceRel, cfg.pkgName, cfg.outRel, *prefix)
	return writeOrCheckRoutesDoc("gen", cfg, doc, warnLines, *baseline, *strict, *check, stdout, stderr)
}

func buildRoutesDoc(pkg *directive.Package, sourceRel, pkgName, outRel, prefix string) routesDoc {
	doc := routesDoc{
		Domain:      "routes/v1",
		Producer:    "handlers",
		Source:      sourceRel,
		Package:     packageDoc{Name: pkgName, Dir: outRel},
		Surface:     "html",
		Prefix:      prefix,
		Routes:      make([]routeDoc, 0),
		Permissions: make([]permDoc, 0),
	}
	if pkg == nil {
		return doc
	}

	usedAliases := make(map[string]bool, len(reservedAliases))
	for k, v := range reservedAliases {
		usedAliases[k] = v
	}
	importAliases := make(map[string]string)

	assignAlias := func(importPath, pkgName string) string {
		if a, ok := importAliases[importPath]; ok {
			return a
		}
		candidate := pkgName
		if usedAliases[candidate] {
			for n := 2; ; n++ {
				try := fmt.Sprintf("%s%d", pkgName, n)
				if !usedAliases[try] {
					candidate = try
					break
				}
			}
		}
		usedAliases[candidate] = true
		importAliases[importPath] = candidate
		return candidate
	}

	pb := newPermBuilder()

	for _, r := range pkg.Routes {
		text := r.Desc
		if text == "" {
			text = directive.Readable(r.Key)
		}

		auth := r.Auth
		if auth == "" || auth == "unverified" {
			auth = "permission"
		}

		var permKey string
		if auth == "permission" {
			permKey = r.Perm
			if permKey == "" {
				permKey = r.Key
			}
			pb.add(permKey, text, r.Auth == "unverified")
		}

		restParam := restParamName(r.Path)
		params := make([]paramDoc, 0, len(r.Params))
		for _, p := range r.Params {
			kind := "segment"
			if restParam != "" && p.Name == restParam {
				kind = "rest"
			}
			label := p.Label
			if !p.LabelSet {
				label = directive.Readable(p.Name)
			}

			var alias, paramType string
			if p.ImportPath != "" {
				alias = assignAlias(p.ImportPath, p.PkgName)
				shortName := strings.TrimPrefix(p.TypeName, p.PkgName+".")
				paramType = alias + "." + shortName
			} else {
				paramType = p.TypeName
			}

			params = append(params, paramDoc{
				Name:   p.Name,
				Go:     p.Name,
				Var:    "L10n" + r.Handler + upperFirst(p.Name),
				Type:   paramType,
				Import: p.ImportPath,
				Parse:  p.Parse,
				Kind:   kind,
				Label:  label,
				Alias:  alias,
			})
		}

		doc.Routes = append(doc.Routes, routeDoc{
			Key:        r.Key,
			Method:     strings.ToUpper(r.Method),
			Pattern:    r.Path,
			Name:       r.Handler,
			Var:        "L10n" + r.Handler,
			Text:       text,
			Auth:       auth,
			View:       r.View,
			Search:     r.Search,
			Params:     params,
			Permission: permKey,
			Entity:     r.Entity,
			MaxBody:    r.MaxBody,
		})
	}

	doc.Permissions = pb.docs()

	return doc
}

type permAccum struct {
	key        string
	count      int
	firstText  string
	unverified bool
}

type permBuilder struct {
	order []string
	byKey map[string]*permAccum
}

func newPermBuilder() *permBuilder {
	return &permBuilder{byKey: make(map[string]*permAccum)}
}

func (b *permBuilder) add(key, routeText string, unverified bool) {
	pa, exists := b.byKey[key]
	if !exists {
		pa = &permAccum{
			key:        key,
			firstText:  routeText,
			unverified: unverified,
		}
		b.byKey[key] = pa
		b.order = append(b.order, key)
	}
	pa.count++
}

func (b *permBuilder) docs() []permDoc {
	out := make([]permDoc, 0, len(b.order))
	for _, key := range b.order {
		pa := b.byKey[key]
		text := directive.Readable(key)
		if pa.count == 1 {
			text = pa.firstText
		}
		level := "verified"
		if pa.unverified {
			level = "unverified"
		}
		out = append(out, permDoc{
			Key:   key,
			Var:   permGoName(key),
			Text:  text,
			Level: level,
		})
	}
	return out
}

func encodeRoutesDoc(doc routesDoc) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func validatePrefix(s string) error {
	if s == "" {
		return nil
	}
	if !strings.HasPrefix(s, "/") {
		return fmt.Errorf(`%q must start with "/"`, s)
	}
	if strings.HasSuffix(s, "/") {
		return fmt.Errorf(`%q must not end with "/"`, s)
	}
	if strings.Contains(s, "//") {
		return fmt.Errorf(`%q must not contain "//"`, s)
	}
	if strings.Contains(s, "{") {
		return fmt.Errorf(`%q must not contain "{"`, s)
	}
	return nil
}

func isGoIdentifier(s string) bool {
	if s == "" || token.Lookup(s) != token.IDENT {
		return false
	}
	return token.IsIdentifier(s)
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func permGoName(key string) string {
	parts := strings.FieldsFunc(key, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	var b strings.Builder
	b.WriteString("Perm")
	for _, p := range parts {
		b.WriteString(upperFirst(p))
	}
	return b.String()
}

func restParamName(path string) string {
	if path == "" || path == "/" {
		return ""
	}
	for seg := range strings.SplitSeq(strings.TrimPrefix(path, "/"), "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "...}") {
			return strings.TrimSuffix(strings.TrimPrefix(seg, "{"), "...}")
		}
	}
	return ""
}

func findModuleRootFromCwdOr(fallbackDir string) (string, error) {
	if cwd, err := os.Getwd(); err == nil {
		if root, ok := findGoModUp(cwd); ok {
			return root, nil
		}
	}
	if fallbackDir != "" {
		if root, ok := findGoModUp(fallbackDir); ok {
			return root, nil
		}
	}
	return "", fmt.Errorf("go.mod not found")
}

func findGoModUp(dir string) (string, bool) {
	cur := filepath.Clean(dir)
	for {
		info, err := os.Stat(filepath.Join(cur, "go.mod"))
		if err == nil && !info.IsDir() {
			return cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

func relSlash(base, target string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Clean(rel)), nil
}

func loadGitFile(ref, absFile string) ([]byte, bool) {
	dir := filepath.Dir(absFile)
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, false
		}
		dir = parent
	}

	// "<ref>:./<path>" is resolved by git relative to the -C directory, so
	// symlinks (macOS's /var) and drive letters (Windows) don't matter.
	rel, err := filepath.Rel(dir, absFile)
	if err != nil {
		return nil, false
	}
	gitRel := "./" + filepath.ToSlash(rel)

	showCmd := exec.Command("git", "-C", dir, "show", ref+":"+gitRel)
	out, err := showCmd.Output()
	if err != nil {
		return nil, false
	}
	return out, true
}

func compareBaseline(baseBytes []byte, baseLabel, filePosPath string, newDoc routesDoc) []directive.Diagnostic {
	var oldDoc routesDoc
	if err := json.Unmarshal(baseBytes, &oldDoc); err != nil {
		return nil
	}

	newRoutes := make(map[string]bool, len(newDoc.Routes))
	newParamLabels := make(map[string]bool)
	for _, r := range newDoc.Routes {
		newRoutes[r.Key] = true
		for _, p := range r.Params {
			newParamLabels["route_param."+r.Key+"."+p.Name] = true
		}
	}

	newPerms := make(map[string]bool, len(newDoc.Permissions))
	for _, p := range newDoc.Permissions {
		newPerms[p.Key] = true
	}

	pos := token.Position{Filename: filePosPath, Line: 1, Column: 1}
	var diags []directive.Diagnostic

	for _, r := range oldDoc.Routes {
		if !newRoutes[r.Key] {
			diags = append(diags, directive.Diagnostic{
				Pos:      pos,
				Severity: directive.Warning,
				Message:  fmt.Sprintf("key %q is gone since %s: its translations and permission grants are orphaned", r.Key, baseLabel),
			})
		}
	}

	for _, p := range oldDoc.Permissions {
		if !newPerms[p.Key] {
			diags = append(diags, directive.Diagnostic{
				Pos:      pos,
				Severity: directive.Warning,
				Message:  fmt.Sprintf("permission %q is gone since %s: its grants are orphaned", p.Key, baseLabel),
			})
		}
	}

	for _, r := range oldDoc.Routes {
		for _, p := range r.Params {
			lbl := "route_param." + r.Key + "." + p.Name
			if !newParamLabels[lbl] {
				diags = append(diags, directive.Diagnostic{
					Pos:      pos,
					Severity: directive.Warning,
					Message:  fmt.Sprintf("parameter label %q is gone since %s: its translations are orphaned", lbl, baseLabel),
				})
			}
		}
	}

	return diags
}
