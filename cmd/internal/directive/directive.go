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
	Name       string
	Label      string
	LabelSet   bool
	ImportPath string // "" for predeclared types
	PkgName    string // "" for predeclared types
	TypeName   string // "<pkgname>.<Name>" or predeclared name
	Parse      string // "text" or underlying basic type name
	Pos        token.Position
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
	PermText     string // "" when not given
	PermFrom     string // "" when not given
	View, Search bool
	Entity       string
	MaxBody      int64          // bytes; 0 when not given
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

var (
	keyPattern      = regexp.MustCompile(`^[a-z0-9][A-Za-z0-9_]*(\.[a-z0-9][A-Za-z0-9_]*)*$`)
	bodySizePattern = regexp.MustCompile(`^([1-9][0-9]*)(B|KiB|MiB|GiB)$`)
)

// ParseBodySize parses s as a body size matching ^[1-9][0-9]*(B|KiB|MiB|GiB)$
// whose byte count fits in int64. If valid, it returns the byte count and "".
// Otherwise it returns 0 and the diagnostic message prefixed with prefix
// (e.g. "h:body" or "x-max-body").
func ParseBodySize(s, prefix string) (int64, string) {
	m := bodySizePattern.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Sprintf(`%s: %q is not a size: write a positive whole number and B, KiB, MiB or GiB, as 20MiB`, prefix, s)
	}
	var shift uint
	switch m[2] {
	case "B":
		shift = 0
	case "KiB":
		shift = 10
	case "MiB":
		shift = 20
	case "GiB":
		shift = 30
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	maxN := maxInt64 >> shift
	var n int64
	for i := 0; i < len(m[1]); i++ {
		d := int64(m[1][i] - '0')
		if n > (maxN-d)/10 {
			return 0, fmt.Sprintf(`%s: %q is larger than a body can be`, prefix, s)
		}
		n = n*10 + d
	}
	return n << shift, ""
}

// PermValue holds the parsed parts of a perm or x-permission directive.
type PermValue struct {
	Key  string
	Text string
	From string
}

// ParsePerm parses s according to the grammar:
//
//	<key>
//	<key> <text>
//	<key> from <package>
//
// prefix is "h:perm" or "x-permission".
// If invalid, it returns a non-empty diagnostic message.
func ParsePerm(s, prefix string) (PermValue, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		if prefix == "h:perm" {
			return PermValue{}, fmt.Sprintf("%s: %q is not a key", prefix, "")
		}
		return PermValue{}, fmt.Sprintf("%s %q is not a key", prefix, "")
	}
	fields := strings.Fields(s)
	key := fields[0]
	if !IsValidKey(key) {
		if prefix == "h:perm" {
			return PermValue{}, fmt.Sprintf("%s: %q is not a key", prefix, key)
		}
		return PermValue{}, fmt.Sprintf("%s %q is not a key", prefix, key)
	}
	if len(fields) == 1 {
		return PermValue{Key: key}, ""
	}
	if fields[1] == "from" {
		if len(fields) != 3 || !token.IsIdentifier(fields[2]) || token.IsKeyword(fields[2]) || fields[2] == "_" {
			if prefix == "h:perm" {
				return PermValue{}, fmt.Sprintf(`%s: write "h:perm %s from <package>", with nothing after`, prefix, key)
			}
			return PermValue{}, fmt.Sprintf(`%s: write "%s from <package>", with nothing after`, prefix, key)
		}
		return PermValue{Key: key, From: fields[2]}, ""
	}
	text := strings.TrimSpace(s[len(key):])
	return PermValue{Key: key, Text: text}, ""
}

// IsValidKey reports whether s matches the routes/v1 key grammar.
func IsValidKey(s string) bool {
	return keyPattern.MatchString(s)
}

func isValidKey(s string) bool {
	return IsValidKey(s)
}

// IsReservedGenName reports whether name is a package-level identifier
// declared by every generated routes package.
func IsReservedGenName(name string) bool {
	return reservedGenNames[name]
}

// SplitWords splits name at '.', '_', '-' and lower-to-upper case transitions
// (keeping a run of capitals together as one word).
func SplitWords(name string) []string {
	var words []string
	var cur []rune

	flush := func() {
		if len(cur) > 0 {
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
	return words
}

// Readable converts a key or identifier name into human-readable title text
// (§13.3): split at '.', '_', '-' and lower-to-upper transitions (a run of
// capitals is one word), each word's first letter upper-cased, joined with spaces.
func Readable(name string) string {
	words := SplitWords(name)
	for i, w := range words {
		words[i] = upperFirst(w)
	}
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
