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
				vp.routesFile = f.relPath
				vp.routesFileAbs = f.absPath
				break
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
				newDoc := buildRoutesDoc(vp.pkg, vp.relDir, "", vp.pkgDir, vp.prefix)
				diags = append(diags, compareBaseline(baseBytes, baselineRef, vp.routesFile, newDoc)...)
			}
		}
	}

	var allRoutes []crossRoute
	var allPerms []crossPerm
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
		for _, p := range f.doc.Permissions {
			allPerms = append(allPerms, crossPerm{
				sourceID: sourceID,
				key:      p.Key,
				pkgDir:   pkgDir,
				pos:      pos,
			})
		}
	}

	for _, vp := range vetted {
		sourceID++
		if vp.pkg == nil {
			continue
		}
		seenPermInPkg := make(map[string]bool)
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
				if pk != "" && !seenPermInPkg[pk] {
					seenPermInPkg[pk] = true
					allPerms = append(allPerms, crossPerm{
						sourceID: sourceID,
						key:      pk,
						pkgDir:   vp.pkgDir,
						pos:      r.Pos,
					})
				}
			}
		}
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
					Message:  fmt.Sprintf("permission %q is in %s and %s; a permission belongs to one routes package", p.key, first.pkgDir, p.pkgDir),
				})
			}
		} else {
			seenPerms[p.key] = p
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
