package lsp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/epistemer/handlers/cmd/internal/directive"
)

var directiveNames = []string{
	"key",
	"method",
	"path",
	"desc",
	"auth",
	"perm",
	"view",
	"search",
	"entity",
	"param",
}

var directiveDocs = map[string]string{
	"key":    "h:key <key> — Required route key: dot-separated segments ([a-z0-9][A-Za-z0-9_]*).",
	"method": "h:method <METHOD> — Required HTTP method: GET, POST, PUT, PATCH, or DELETE (any case).",
	"path":   "h:path <path> — Required URL path starting with \"/\" (no trailing \"/\" unless \"/\", no \"{$}\"; supports {name} and trailing {name...}).",
	"desc":   "h:desc <text> — Route label text (defaults to the key made readable, with a warning).",
	"auth":   "h:auth <level> — Access level: public, session, or unverified (default: route permission at Verified).",
	"perm":   "h:perm <key> — Permission key (only when auth is default or unverified; defaults to the route key).",
	"view":   "h:view [true] — Mark route as a view page (takes no value or true).",
	"search": "h:search [true] — Include route in search (takes no value or true).",
	"entity": "h:entity <key> — Key of another route in the same package representing this route's entity.",
	"param":  "h:param <name> label <text> — Label for a path parameter of the handler (defaults to parameter name made readable).",
}

var methodValues = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
var authValues = []string{"public", "session", "unverified"}

func completeDocument(doc *openDoc, routes []directive.Route, pos Position, enc string) CompletionList {
	list := CompletionList{Items: []CompletionItem{}}
	if doc == nil || pos.Line < 0 || pos.Line >= len(doc.lines) {
		return list
	}
	line := doc.lines[pos.Line]
	cursorByte := charToByteOffset(line, pos.Character, enc)
	before := line[:cursorByte]

	trimmed := strings.TrimLeft(before, " \t")
	leadingBytes := len(before) - len(trimmed)
	if !strings.HasPrefix(trimmed, "//h:") {
		return list
	}

	afterH := trimmed[4:]
	afterHStartByte := leadingBytes + 4

	sepIdx := strings.IndexAny(afterH, " \t")
	if sepIdx < 0 {
		// Completing directive name after //h:
		editRange := Range{
			Start: Position{Line: pos.Line, Character: byteOffsetToChar(line, afterHStartByte, enc)},
			End:   pos,
		}
		for _, name := range directiveNames {
			if strings.HasPrefix(name, afterH) {
				list.Items = append(list.Items, CompletionItem{
					Label:  name,
					Kind:   itemKindKeyword,
					Detail: directiveDocs[name],
					TextEdit: &TextEdit{
						Range:   editRange,
						NewText: name,
					},
				})
			}
		}
		return list
	}

	dirName := afterH[:sepIdx]
	rest := afterH[sepIdx+1:]
	valPrefix := strings.TrimLeft(rest, " \t")

	if dirName == "param" {
		if paramSep := strings.IndexAny(valPrefix, " \t"); paramSep >= 0 {
			afterParam := strings.TrimLeft(valPrefix[paramSep+1:], " \t")
			if !strings.ContainsAny(afterParam, " \t") && strings.HasPrefix("label", afterParam) {
				kwStartByte := cursorByte - len(afterParam)
				editRange := Range{
					Start: Position{Line: pos.Line, Character: byteOffsetToChar(line, kwStartByte, enc)},
					End:   pos,
				}
				list.Items = append(list.Items, CompletionItem{
					Label: "label",
					Kind:  itemKindKeyword,
					TextEdit: &TextEdit{
						Range:   editRange,
						NewText: "label",
					},
				})
			}
			return list
		}

		valStartByte := cursorByte - len(valPrefix)
		editRange := Range{
			Start: Position{Line: pos.Line, Character: byteOffsetToChar(line, valStartByte, enc)},
			End:   pos,
		}
		for _, pName := range extractHandlerParams(doc.content, pos.Line) {
			if strings.HasPrefix(pName, valPrefix) {
				list.Items = append(list.Items, CompletionItem{
					Label:  pName,
					Kind:   itemKindVariable,
					Detail: "handler path parameter",
					TextEdit: &TextEdit{
						Range:   editRange,
						NewText: pName,
					},
				})
			}
		}
		return list
	}

	if strings.ContainsAny(valPrefix, " \t") {
		return list
	}

	valStartByte := cursorByte - len(valPrefix)
	editRange := Range{
		Start: Position{Line: pos.Line, Character: byteOffsetToChar(line, valStartByte, enc)},
		End:   pos,
	}

	switch dirName {
	case "method":
		upperPrefix := strings.ToUpper(valPrefix)
		for _, m := range methodValues {
			if strings.HasPrefix(m, upperPrefix) {
				list.Items = append(list.Items, CompletionItem{
					Label: m,
					Kind:  itemKindEnumMember,
					TextEdit: &TextEdit{
						Range:   editRange,
						NewText: m,
					},
				})
			}
		}
	case "auth":
		for _, a := range authValues {
			if strings.HasPrefix(a, valPrefix) {
				list.Items = append(list.Items, CompletionItem{
					Label: a,
					Kind:  itemKindEnumMember,
					TextEdit: &TextEdit{
						Range:   editRange,
						NewText: a,
					},
				})
			}
		}
	case "view", "search":
		if strings.HasPrefix("true", valPrefix) {
			list.Items = append(list.Items, CompletionItem{
				Label: "true",
				Kind:  itemKindKeyword,
				TextEdit: &TextEdit{
					Range:   editRange,
					NewText: "true",
				},
			})
		}
	case "entity":
		selfKey := ownKeyInCommentBlock(doc.lines, pos.Line)
		seen := make(map[string]bool)
		var keys []string
		for _, r := range routes {
			if r.Key == "" || r.Key == selfKey || seen[r.Key] {
				continue
			}
			seen[r.Key] = true
			if strings.HasPrefix(r.Key, valPrefix) {
				keys = append(keys, r.Key)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			list.Items = append(list.Items, CompletionItem{
				Label: k,
				Kind:  itemKindReference,
				TextEdit: &TextEdit{
					Range:   editRange,
					NewText: k,
				},
			})
		}
	}

	return list
}

func hoverDocument(doc *openDoc, pos Position, enc string) *Hover {
	if doc == nil || pos.Line < 0 || pos.Line >= len(doc.lines) {
		return nil
	}
	line := doc.lines[pos.Line]
	trimmed := strings.TrimLeft(line, " \t")
	leadingBytes := len(line) - len(trimmed)
	if !strings.HasPrefix(trimmed, "//h:") {
		return nil
	}

	cursorByte := charToByteOffset(line, pos.Character, enc)
	if cursorByte < leadingBytes {
		return nil
	}

	afterH := trimmed[4:]
	dirName := afterH
	if sep := strings.IndexAny(afterH, " \t"); sep >= 0 {
		dirName = afterH[:sep]
	}

	docLine, ok := directiveDocs[dirName]
	if !ok {
		return nil
	}

	startByte := leadingBytes
	endByte := leadingBytes + 4 + len(dirName)
	rng := Range{
		Start: Position{Line: pos.Line, Character: byteOffsetToChar(line, startByte, enc)},
		End:   Position{Line: pos.Line, Character: byteOffsetToChar(line, endByte, enc)},
	}
	return &Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: docLine,
		},
		Range: &rng,
	}
}

func definitionForEntity(
	doc *openDoc,
	routes []directive.Route,
	pos Position,
	enc string,
	linesForFile func(path string) []string,
) *Location {
	if doc == nil || pos.Line < 0 || pos.Line >= len(doc.lines) {
		return nil
	}
	line := doc.lines[pos.Line]
	trimmed := strings.TrimLeft(line, " \t")
	leadingBytes := len(line) - len(trimmed)
	if !strings.HasPrefix(trimmed, "//h:entity") {
		return nil
	}
	rest := trimmed[len("//h:entity"):]
	if len(rest) == 0 || (rest[0] != ' ' && rest[0] != '\t') {
		return nil
	}

	trimmedValLeft := strings.TrimLeft(rest, " \t")
	valStartByte := leadingBytes + len("//h:entity") + (len(rest) - len(trimmedValLeft))
	val := strings.TrimRight(trimmedValLeft, " \t")
	valEndByte := valStartByte + len(val)
	if val == "" {
		return nil
	}

	cursorByte := charToByteOffset(line, pos.Character, enc)
	if cursorByte < valStartByte || cursorByte > valEndByte {
		return nil
	}

	for _, r := range routes {
		if r.Key != val {
			continue
		}
		targetLines := linesForFile(r.Pos.Filename)
		lineIdx := max(0, r.Pos.Line-1)
		if len(targetLines) > 0 && lineIdx >= len(targetLines) {
			lineIdx = len(targetLines) - 1
		}
		startChar := 0
		endChar := 0
		if len(targetLines) > 0 {
			targetLine := targetLines[lineIdx]
			startByte := max(0, min(r.Pos.Column-1, len(targetLine)))
			endByte := min(startByte+len("func"), len(targetLine))
			startChar = byteOffsetToChar(targetLine, startByte, enc)
			endChar = byteOffsetToChar(targetLine, endByte, enc)
		}
		return &Location{
			URI: pathToURI(r.Pos.Filename),
			Range: Range{
				Start: Position{Line: lineIdx, Character: startChar},
				End:   Position{Line: lineIdx, Character: endChar},
			},
		}
	}
	return nil
}

func extractHandlerParams(src []byte, line0 int) []string {
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "", src, parser.ParseComments)
	if f == nil {
		return nil
	}

	targetLine := line0 + 1
	var matched *ast.FuncDecl
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn == nil || fn.Type == nil || fn.Type.Params == nil {
			continue
		}
		if fn.Doc != nil {
			docStart := fset.Position(fn.Doc.Pos()).Line
			docEnd := fset.Position(fn.Doc.End()).Line
			if targetLine >= docStart && targetLine <= docEnd {
				matched = fn
				break
			}
		}
		fnLine := fset.Position(fn.Pos()).Line
		if fnLine >= targetLine && fnLine-targetLine <= 20 {
			matched = fn
			break
		}
	}
	if matched == nil {
		return nil
	}

	var names []string
	for _, field := range matched.Type.Params.List {
		if len(field.Names) == 0 {
			names = append(names, "")
			continue
		}
		for _, id := range field.Names {
			names = append(names, id.Name)
		}
	}
	if len(names) <= 2 {
		return nil
	}
	var out []string
	for _, n := range names[2:] {
		if n != "" && n != "_" {
			out = append(out, n)
		}
	}
	return out
}

func ownKeyInCommentBlock(lines []string, line0 int) string {
	for i := line0; i >= 0; i-- {
		t := strings.TrimLeft(lines[i], " \t")
		if !strings.HasPrefix(t, "//") {
			break
		}
		if k, ok := parseKeyLine(t); ok {
			return k
		}
	}
	for i := line0 + 1; i < len(lines); i++ {
		t := strings.TrimLeft(lines[i], " \t")
		if !strings.HasPrefix(t, "//") {
			break
		}
		if k, ok := parseKeyLine(t); ok {
			return k
		}
	}
	return ""
}

func parseKeyLine(trimmedComment string) (string, bool) {
	if !strings.HasPrefix(trimmedComment, "//h:key") {
		return "", false
	}
	rest := trimmedComment[len("//h:key"):]
	if len(rest) == 0 || (rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	return strings.TrimSpace(rest), true
}
