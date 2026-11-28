package lsp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
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
	"body",
	"param",
}

var directiveDocs = map[string]string{
	"key":    "h:key <key> — Required route key: dot-separated segments ([a-z0-9][A-Za-z0-9_]*).",
	"method": "h:method <METHOD> — Required HTTP method: GET, POST, PUT, PATCH, or DELETE (any case).",
	"path":   "h:path <path> — Required URL path starting with \"/\" (no trailing \"/\" unless \"/\", no \"{$}\"; supports {name} and trailing {name...}).",
	"desc":   "h:desc <text> — Route label text (defaults to the key made readable, with a warning).",
	"auth":   "h:auth <level> — Access level: public, session, or unverified (default: route permission at Verified).",
	"perm":   "h:perm <key> [text] | <key> from <package> — Permission key, optional text, or taken from another package (only when auth is default or unverified; defaults to the route key).",
	"view":   "h:view [true] — Mark route as a view page (takes no value or true).",
	"search": "h:search [true] — Include route in search (takes no value or true).",
	"body":   "h:body <size> — Maximum request body size for this route: positive whole number and B, KiB, MiB, or GiB (e.g. 20MiB).",
	"param":  "h:param <name> [from <key>] [label <text>] — A path parameter's source, a GET route of this package without parameters that suggests its values, and its label (defaults to the parameter name made readable).",
}

var methodValues = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
var authValues = []string{"public", "session", "unverified"}
var bodyUnits = []string{"B", "KiB", "MiB", "GiB"}
var bodyExamples = []string{"512B", "64KiB", "20MiB", "1GiB"}

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
			// The words after the parameter's name: [from <key>] [label <text>].
			words := strings.FieldsFunc(valPrefix[paramSep+1:], func(r rune) bool { return r == ' ' || r == '\t' })
			partial := ""
			if !strings.HasSuffix(valPrefix, " ") && !strings.HasSuffix(valPrefix, "\t") && len(words) > 0 {
				partial = words[len(words)-1]
				words = words[:len(words)-1]
			}
			editRange := Range{
				Start: Position{Line: pos.Line, Character: byteOffsetToChar(line, cursorByte-len(partial), enc)},
				End:   pos,
			}
			var offer []string
			kind := itemKindKeyword
			switch {
			case len(words) == 0:
				offer = []string{"from", "label"}
			case len(words) == 1 && words[0] == "from":
				selfKey := ownKeyInCommentBlock(doc.lines, pos.Line)
				seen := make(map[string]bool)
				for _, r := range routes {
					// A source is a GET route without parameters.
					if r.Key == "" || r.Key == selfKey || seen[r.Key] || !strings.EqualFold(r.Method, "GET") || len(r.Params) > 0 {
						continue
					}
					seen[r.Key] = true
					offer = append(offer, r.Key)
				}
				sort.Strings(offer)
				kind = itemKindReference
			case len(words) == 2 && words[0] == "from":
				offer = []string{"label"}
			}
			for _, o := range offer {
				if strings.HasPrefix(o, partial) {
					list.Items = append(list.Items, CompletionItem{
						Label: o,
						Kind:  kind,
						TextEdit: &TextEdit{
							Range:   editRange,
							NewText: o,
						},
					})
				}
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
	case "body":
		candidates := bodyExamples
		if num, unitPrefix, ok := splitLeadingPositiveNumber(valPrefix); ok {
			candidates = nil
			for _, u := range bodyUnits {
				if strings.HasPrefix(u, unitPrefix) {
					candidates = append(candidates, num+u)
				}
			}
		}
		for _, c := range candidates {
			if strings.HasPrefix(c, valPrefix) {
				list.Items = append(list.Items, CompletionItem{
					Label: c,
					Kind:  itemKindEnumMember,
					TextEdit: &TextEdit{
						Range:   editRange,
						NewText: c,
					},
				})
			}
		}
	}

	return list
}

func splitLeadingPositiveNumber(s string) (num, rest string, ok bool) {
	if s == "" || s[0] < '1' || s[0] > '9' {
		return "", "", false
	}
	i := 1
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:], true
}

func formatGroupedBytes(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	rem := len(s) % 3
	var b strings.Builder
	if rem > 0 {
		b.WriteString(s[:rem])
	}
	for i := rem; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
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
	val := ""
	if sep := strings.IndexAny(afterH, " \t"); sep >= 0 {
		dirName = afterH[:sep]
		val = strings.TrimSpace(afterH[sep+1:])
	}

	docLine, ok := directiveDocs[dirName]
	if !ok {
		return nil
	}
	if dirName == "body" && val != "" {
		if n, errMsg := directive.ParseBodySize(val, "h:body"); errMsg == "" {
			docLine = val + " = " + formatGroupedBytes(n) + " bytes — " + docLine
		}
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

// definitionForSource answers go-to-definition on the key of an
// h:param <name> from <key> line: the source route's handler (A9).
func definitionForSource(
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
	if !strings.HasPrefix(trimmed, "//h:param ") && !strings.HasPrefix(trimmed, "//h:param\t") {
		return nil
	}
	// The key is the word after the word "from", which follows the name.
	rest := trimmed[len("//h:param"):]
	base := leadingBytes + len("//h:param")
	var spans [][2]int // byte offsets of the first three words, in line
	for i := 0; i < len(rest) && len(spans) < 3; {
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t') {
			i++
		}
		start := i
		for i < len(rest) && rest[i] != ' ' && rest[i] != '\t' {
			i++
		}
		if i > start {
			spans = append(spans, [2]int{base + start, base + i})
		}
	}
	if len(spans) < 3 || line[spans[1][0]:spans[1][1]] != "from" {
		return nil
	}
	valStartByte, valEndByte := spans[2][0], spans[2][1]
	val := line[valStartByte:valEndByte]

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
