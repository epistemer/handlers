package main

import (
	"encoding/json"
	"fmt"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/epistemer/handlers/cmd/internal/directive"
)

type vettedPkg struct {
	dir           string
	relDir        string
	pkg           *directive.Package
	prefix        string
	pkgDir        string
	pkgName       string
	routesFile    string
	routesFileAbs string
}

type routesFileEntry struct {
	absPath string
	relPath string
	doc     routesDoc
}

type crossRoute struct {
	sourceID       int
	key            string
	displayPattern string
	muxPattern     string
	pos            token.Position
	refFile        string
	refLine        int
}

type crossPerm struct {
	sourceID int
	key      string
	pkgDir   string
	pos      token.Position
}

func crossVet(modRoot string, vetted []vettedPkg, baselineRef string) ([]directive.Diagnostic, error) {
	var files []routesFileEntry
	err := filepath.WalkDir(modRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != modRoot && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, "zz_") && strings.HasSuffix(name, ".routes.json") {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var doc routesDoc
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			rel, err := relSlash(modRoot, path)
			if err != nil {
				return err
			}
			files = append(files, routesFileEntry{
				absPath: path,
				relPath: rel,
				doc:     doc,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].relPath < files[j].relPath
	})

	replaced := make([]bool, len(files))
	for vi := range vetted {
		vp := &vetted[vi]
		vp.pkgDir = vp.relDir
		for fi, f := range files {
			if f.doc.Source == vp.relDir {
				replaced[fi] = true
				vp.prefix = f.doc.Prefix
				if f.doc.Package.Dir != "" {
					vp.pkgDir = f.doc.Package.Dir
				}
				if f.doc.Package.Name != "" {
					vp.pkgName = f.doc.Package.Name
				}
				vp.routesFile = f.relPath
				vp.routesFileAbs = f.absPath
				break
			}
		}
		if vp.pkgName == "" {
			if vp.pkg != nil && vp.pkg.Name != "" {
				vp.pkgName = vp.pkg.Name + "routes"
			} else {
				vp.pkgName = filepath.Base(vp.pkgDir)
			}
		}
	}

	var diags []directive.Diagnostic

	if baselineRef != "" {
		for _, vp := range vetted {
			if vp.routesFileAbs == "" {
				continue
			}
			if baseBytes, ok := loadGitFile(baselineRef, vp.routesFileAbs); ok {
				newDoc := buildRoutesDoc(vp.pkg, vp.relDir, vp.pkgName, vp.pkgDir, vp.prefix)
				diags = append(diags, compareBaseline(baseBytes, baselineRef, vp.routesFile, newDoc)...)
			}
		}
	}

	type crossPackage struct {
		name     string
		dir      string
		sourceID int
		decls    map[string]string // permKey -> level ("verified" or "unverified")
	}
	type crossTakenPerm struct {
		sourceID  int
		permKey   string
		fromPkg   string
		takingDir string
		routeKey  string
		level     string // "verified" or "unverified"
		pos       token.Position
	}

	var allRoutes []crossRoute
	var allPerms []crossPerm
	var allPackages []crossPackage
	var allTakenPerms []crossTakenPerm
	sourceID := 0

	for fi, f := range files {
		if replaced[fi] {
			continue
		}
		sourceID++
		pos := token.Position{Filename: f.relPath, Line: 1, Column: 1}
		pkgDir := f.doc.Package.Dir
		if pkgDir == "" {
			pkgDir = filepath.ToSlash(filepath.Dir(f.relPath))
		}
		pkgName := f.doc.Package.Name
		if pkgName == "" {
			pkgName = filepath.Base(pkgDir)
		}
		decls := make(map[string]string)
		for _, r := range f.doc.Routes {
			allRoutes = append(allRoutes, crossRoute{
				sourceID:       sourceID,
				key:            r.Key,
				displayPattern: r.Method + " " + f.doc.Prefix + r.Pattern,
				muxPattern:     r.Method + " " + f.doc.Prefix + muxPath(r.Pattern),
				pos:            pos,
				refFile:        f.relPath,
				refLine:        1,
			})
		}
		seenTakenInFile := make(map[string]bool)
		for _, p := range f.doc.Permissions {
			lvl := p.Level
			if lvl == "" {
				lvl = "verified"
			}
			if p.From == "" {
				decls[p.Key] = lvl
				allPerms = append(allPerms, crossPerm{
					sourceID: sourceID,
					key:      p.Key,
					pkgDir:   pkgDir,
					pos:      pos,
				})
			} else if !seenTakenInFile[p.Key] {
				seenTakenInFile[p.Key] = true
				rk := p.Key
				for _, r := range f.doc.Routes {
					if r.Permission == p.Key && r.Key != "" {
						rk = r.Key
						break
					}
				}
				allTakenPerms = append(allTakenPerms, crossTakenPerm{
					sourceID:  sourceID,
					permKey:   p.Key,
					fromPkg:   p.From,
					takingDir: pkgDir,
					routeKey:  rk,
					level:     lvl,
					pos:       pos,
				})
			}
		}
		allPackages = append(allPackages, crossPackage{
			name:     pkgName,
			dir:      pkgDir,
			sourceID: sourceID,
			decls:    decls,
		})
	}

	for _, vp := range vetted {
		sourceID++
		if vp.pkg == nil || (vp.routesFile == "" && len(vp.pkg.Routes) == 0) {
			continue
		}
		decls := make(map[string]string)
		seenPermInPkg := make(map[string]bool)
		seenTakenInPkg := make(map[string]bool)
		for _, r := range vp.pkg.Routes {
			if r.Key != "" {
				refFile := r.Pos.Filename
				if rel, err := relSlash(modRoot, r.Pos.Filename); err == nil {
					refFile = rel
				}
				allRoutes = append(allRoutes, crossRoute{
					sourceID:       sourceID,
					key:            r.Key,
					displayPattern: strings.ToUpper(r.Method) + " " + vp.prefix + r.Path,
					muxPattern:     strings.ToUpper(r.Method) + " " + vp.prefix + muxPath(r.Path),
					pos:            r.Pos,
					refFile:        refFile,
					refLine:        r.Pos.Line,
				})
			}
			if r.Auth == "" || r.Auth == "unverified" {
				pk := r.Perm
				if pk == "" {
					pk = r.Key
				}
				lvl := "verified"
				if r.Auth == "unverified" {
					lvl = "unverified"
				}
				if r.PermFrom == "" {
					if pk != "" {
						decls[pk] = lvl
					}
					if pk != "" && !seenPermInPkg[pk] {
						seenPermInPkg[pk] = true
						allPerms = append(allPerms, crossPerm{
							sourceID: sourceID,
							key:      pk,
							pkgDir:   vp.pkgDir,
							pos:      r.Pos,
						})
					}
				} else {
					if pk != "" && !seenTakenInPkg[pk] {
						seenTakenInPkg[pk] = true
						allTakenPerms = append(allTakenPerms, crossTakenPerm{
							sourceID:  sourceID,
							permKey:   pk,
							fromPkg:   r.PermFrom,
							takingDir: vp.pkgDir,
							routeKey:  r.Key,
							level:     lvl,
							pos:       r.Pos,
						})
					}
				}
			}
		}
		allPackages = append(allPackages, crossPackage{
			name:     vp.pkgName,
			dir:      vp.pkgDir,
			sourceID: sourceID,
			decls:    decls,
		})
	}

	seenKeys := make(map[string]crossRoute)
	for _, r := range allRoutes {
		if first, ok := seenKeys[r.key]; ok {
			if first.sourceID != r.sourceID {
				diags = append(diags, directive.Diagnostic{
					Pos:      r.pos,
					Severity: directive.Error,
					Message:  fmt.Sprintf("key %q also at %s:%d", r.key, first.refFile, first.refLine),
				})
			}
		} else {
			seenKeys[r.key] = r
		}
	}

	seenPerms := make(map[string]crossPerm)
	for _, p := range allPerms {
		if first, ok := seenPerms[p.key]; ok {
			if first.sourceID != p.sourceID {
				diags = append(diags, directive.Diagnostic{
					Pos:      p.pos,
					Severity: directive.Error,
					Message:  fmt.Sprintf("permission %q is in %s and %s; declare it in one and take it from there with \"from <package>\"", p.key, first.pkgDir, p.pkgDir),
				})
			}
		} else {
			seenPerms[p.key] = p
		}
	}

	for _, tp := range allTakenPerms {
		var targets []crossPackage
		for _, cp := range allPackages {
			if cp.name == tp.fromPkg {
				targets = append(targets, cp)
			}
		}
		if len(targets) > 1 {
			sort.Slice(targets, func(i, j int) bool {
				return targets[i].dir < targets[j].dir
			})
			diags = append(diags, directive.Diagnostic{
				Pos:      tp.pos,
				Severity: directive.Error,
				Message:  fmt.Sprintf("permission %q in %s: two routes packages are named %s, in %s and %s", tp.permKey, tp.takingDir, tp.fromPkg, targets[0].dir, targets[1].dir),
			})
		} else if len(targets) == 0 {
			diags = append(diags, directive.Diagnostic{
				Pos:      tp.pos,
				Severity: directive.Error,
				Message:  fmt.Sprintf("permission %q in %s: no routes package %s", tp.permKey, tp.takingDir, tp.fromPkg),
			})
		} else {
			target := targets[0]
			declLevel, ok := target.decls[tp.permKey]
			if !ok {
				diags = append(diags, directive.Diagnostic{
					Pos:      tp.pos,
					Severity: directive.Error,
					Message:  fmt.Sprintf("permission %q in %s: %s doesn't declare it", tp.permKey, tp.takingDir, tp.fromPkg),
				})
			} else {
				declUnv := (declLevel == "unverified")
				takenUnv := (tp.level == "unverified")
				if takenUnv && !declUnv {
					diags = append(diags, directive.Diagnostic{
						Pos:      tp.pos,
						Severity: directive.Error,
						Message:  fmt.Sprintf("permission %q in %s: %s is unverified and the declaration in %s isn't", tp.permKey, tp.takingDir, tp.routeKey, tp.fromPkg),
					})
				} else if !takenUnv && declUnv {
					diags = append(diags, directive.Diagnostic{
						Pos:      tp.pos,
						Severity: directive.Error,
						Message:  fmt.Sprintf("permission %q in %s: %s isn't unverified and the declaration in %s is", tp.permKey, tp.takingDir, tp.routeKey, tp.fromPkg),
					})
				}
			}
		}
	}

	seenPatterns := make(map[string]crossRoute)
	mux := http.NewServeMux()
	var registered []crossRoute
	for _, r := range allRoutes {
		if first, ok := seenPatterns[r.displayPattern]; ok {
			if first.sourceID != r.sourceID {
				diags = append(diags, directive.Diagnostic{
					Pos:      r.pos,
					Severity: directive.Error,
					Message:  fmt.Sprintf("%s also at %s:%d", r.displayPattern, first.refFile, first.refLine),
				})
			}
			continue
		}
		seenPatterns[r.displayPattern] = r

		if !tryServeMux(mux, r.muxPattern) {
			for _, prev := range registered {
				if prev.sourceID == r.sourceID {
					continue
				}
				if muxConflicts(prev.muxPattern, r.muxPattern) {
					diags = append(diags, directive.Diagnostic{
						Pos:      r.pos,
						Severity: directive.Error,
						Message:  fmt.Sprintf("%s conflicts with %s at %s:%d", r.displayPattern, prev.displayPattern, prev.refFile, prev.refLine),
					})
					break
				}
			}
			continue
		}
		registered = append(registered, r)
	}

	return diags, nil
}

func muxPath(p string) string {
	if p == "/" {
		return "/{$}"
	}
	return p
}

func tryServeMux(mux *http.ServeMux, pattern string) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	return true
}

func muxConflicts(a, b string) bool {
	m := http.NewServeMux()
	if !tryServeMux(m, a) {
		return false
	}
	return !tryServeMux(m, b)
}

func countCommittedTakingRoutes(modRoot, skipSource, skipOutRel string) map[string]int {
	counts := make(map[string]int)
	if modRoot == "" {
		return counts
	}
	_ = filepath.WalkDir(modRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != modRoot && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, "zz_") && strings.HasSuffix(name, ".routes.json") {
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			var doc routesDoc
			if err := json.Unmarshal(b, &doc); err != nil {
				return nil
			}
			if doc.Source == skipSource || (skipOutRel != "" && doc.Package.Dir == skipOutRel) {
				return nil
			}
			takenPerms := make(map[string]bool)
			for _, perm := range doc.Permissions {
				if perm.From != "" {
					takenPerms[perm.Key] = true
				}
			}
			for _, r := range doc.Routes {
				if r.Auth == "permission" && takenPerms[r.Permission] {
					counts[r.Permission]++
				}
			}
		}
		return nil
	})
	return counts
}

// committedRoutesFor returns the package name and directory of the committed
// routes file whose source is sourceRel, or "" when there is none.
func committedRoutesFor(modRoot, sourceRel string) (pkgName, outRel string) {
	_ = filepath.WalkDir(modRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || pkgName != "" {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != modRoot && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasPrefix(name, "zz_") || !strings.HasSuffix(name, ".routes.json") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var doc routesDoc
		if json.Unmarshal(b, &doc) == nil && doc.Source == sourceRel {
			pkgName, outRel = doc.Package.Name, doc.Package.Dir
		}
		return nil
	})
	return pkgName, outRel
}
