// Package directive parses, validates, and type-checks //h: route directives
// on Go handler methods according to docs/interface.md (§4, §5, §13.3).
package directive

import (
	"fmt"
	"go/token"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Severity indicates whether a Diagnostic is an error or a warning.
type Severity uint8

const (
	Error Severity = iota + 1
	Warning
)

// Diagnostic is a single rule-engine finding at a source position.
type Diagnostic struct {
	Pos      token.Position // file, line, column (1-based)
	Severity Severity
	Message  string
}

// String formats the diagnostic as "<file>:<line>:<col>: <message>",
// with " (warning)" appended for warnings.
func (d Diagnostic) String() string {
	s := fmt.Sprintf("%s:%d:%d: %s", d.Pos.Filename, d.Pos.Line, d.Pos.Column, d.Message)
	if d.Severity == Warning {
		s += " (warning)"
	}
	return s
}

// Param describes a path parameter of a handler method.
type Param struct {
	Name     string
	Label    string
	LabelSet bool
	Pos      token.Position
}

// Route describes a validated handler method and its directives.
type Route struct {
	Handler      string // method name
	Key          string
	Method       string
	Path         string
	Desc         string
	Auth         string // "", "public", "session", "unverified"
	Perm         string // "" when not given
	View, Search bool
	Entity       string
	Params       []Param        // signature order, after w and r
	Pos          token.Position // the func keyword
}

// Package holds the parsed routes of one Go handler package.
type Package struct {
	Dir    string
	Name   string
	Type   string // receiver type all handlers belong to
	Routes []Route
}

var keyPattern = regexp.MustCompile(`^[a-z0-9][A-Za-z0-9_]*(\.[a-z0-9][A-Za-z0-9_]*)*$`)

func isValidKey(s string) bool {
	return keyPattern.MatchString(s)
}

// Readable converts a key or identifier name into human-readable title text
// (§13.3): split at '.', '_', '-' and lower-to-upper transitions (a run of
// capitals is one word), each word's first letter upper-cased, joined with spaces.
func Readable(name string) string {
	var words []string
	var cur []rune

	flush := func() {
		if len(cur) > 0 {
			cur[0] = unicode.ToUpper(cur[0])
			words = append(words, string(cur))
			cur = cur[:0]
		}
	}

	runes := []rune(name)
	for i, r := range runes {
		if r == '.' || r == '_' || r == '-' {
			flush()
			continue
		}
		if len(cur) > 0 && unicode.IsUpper(r) {
			prev := cur[len(cur)-1]
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && nextIsLower) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return strings.Join(words, " ")
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

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func sortDiagnostics(diags []Diagnostic) {
	sort.Slice(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		if a.Pos.Filename != b.Pos.Filename {
			return a.Pos.Filename < b.Pos.Filename
		}
		if a.Pos.Line != b.Pos.Line {
			return a.Pos.Line < b.Pos.Line
		}
		if a.Pos.Column != b.Pos.Column {
			return a.Pos.Column < b.Pos.Column
		}
		return a.Message < b.Message
	})
}

type wildcard struct {
	name   string
	isRest bool
}

func extractWildcards(path string) []wildcard {
	if path == "" || path == "/" {
		return nil
	}
	var out []wildcard
	for seg := range strings.SplitSeq(strings.TrimPrefix(path, "/"), "/") {
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") {
			continue
		}
		inner := seg[1 : len(seg)-1]
		isRest := false
		name := inner
		if strings.HasSuffix(inner, "...") {
			isRest = true
			name = strings.TrimSuffix(inner, "...")
		}
		if name != "" {
			out = append(out, wildcard{name: name, isRest: isRest})
		}
	}
	return out
}
