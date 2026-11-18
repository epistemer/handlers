package lsp

import (
	"go/token"
	"net/url"
	slashpath "path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	encUTF8  = "utf-8"
	encUTF16 = "utf-16"
)

const (
	severityError   = 1
	severityWarning = 2
)

const (
	syncFull = 1
)

const (
	itemKindKeyword    = 14
	itemKindEnumMember = 20
	itemKindVariable   = 6
	itemKindReference  = 18
)

type InitializeParams struct {
	Capabilities ClientCapabilities `json:"capabilities"`
}

type ClientCapabilities struct {
	General *GeneralClientCapabilities `json:"general,omitempty"`
}

type GeneralClientCapabilities struct {
	PositionEncodings []string `json:"positionEncodings,omitempty"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   *ServerInfo        `json:"serverInfo,omitempty"`
}

type ServerInfo struct {
	Name string `json:"name"`
}

type ServerCapabilities struct {
	PositionEncoding   string             `json:"positionEncoding,omitempty"`
	TextDocumentSync   int                `json:"textDocumentSync"`
	CompletionProvider *CompletionOptions `json:"completionProvider,omitempty"`
	HoverProvider      bool               `json:"hoverProvider,omitempty"`
	DefinitionProvider bool               `json:"definitionProvider,omitempty"`
}

type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type CompletionItem struct {
	Label    string    `json:"label"`
	Kind     int       `json:"kind,omitempty"`
	Detail   string    `json:"detail,omitempty"`
	TextEdit *TextEdit `json:"textEdit,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type HoverParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type DefinitionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

func negotiateEncoding(params InitializeParams) string {
	if params.Capabilities.General != nil &&
		slices.Contains(params.Capabilities.General.PositionEncodings, encUTF8) {
		return encUTF8
	}
	return encUTF16
}

func splitLines(src []byte) []string {
	raw := strings.Split(string(src), "\n")
	for i, l := range raw {
		raw[i] = strings.TrimSuffix(l, "\r")
	}
	return raw
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func byteOffsetToChar(line string, byteOff int, enc string) int {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff > len(line) {
		byteOff = len(line)
	}
	if enc == encUTF8 {
		return byteOff
	}
	return utf16Len(line[:byteOff])
}

func charToByteOffset(line string, charOff int, enc string) int {
	if charOff <= 0 {
		return 0
	}
	if enc == encUTF8 {
		if charOff >= len(line) {
			return len(line)
		}
		return charOff
	}
	units := 0
	for idx, r := range line {
		if units >= charOff {
			return idx
		}
		w := utf16.RuneLen(r)
		if w < 1 {
			w = 1
		}
		if units+w > charOff {
			return idx
		}
		units += w
	}
	return len(line)
}

func tokenPosToLineRange(pos token.Position, lines []string, enc string) Range {
	if len(lines) == 0 {
		return Range{}
	}
	lineIdx := pos.Line - 1
	if lineIdx < 0 {
		lineIdx = 0
	}
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}
	line := lines[lineIdx]

	startByte := 0
	if pos.Column > 1 {
		startByte = min(pos.Column-1, len(line))
	}
	// If token.Position points past the end of valid UTF-8 or into empty space, clamp cleanly.
	for startByte > 0 && !utf8.RuneStart(line[startByte]) {
		startByte--
	}

	startChar := byteOffsetToChar(line, startByte, enc)
	endChar := byteOffsetToChar(line, len(line), enc)
	return Range{
		Start: Position{Line: lineIdx, Character: startChar},
		End:   Position{Line: lineIdx, Character: endChar},
	}
}

func uriToPath(rawURI string) (string, bool) {
	return uriToFSPath(rawURI, runtime.GOOS)
}

func pathToURI(path string) string {
	return fsPathToURI(path, runtime.GOOS)
}

func uriToFSPath(rawURI, goos string) (string, bool) {
	u, err := url.Parse(rawURI)
	if err != nil || u.Scheme != "file" {
		return "", false
	}
	p := u.Path
	if goos == "windows" {
		if u.Host != "" && u.Host != "localhost" {
			return `\\` + u.Host + strings.ReplaceAll(p, "/", `\`), true
		}
		if len(p) >= 3 && p[0] == '/' && isASCIIAlpha(p[1]) && p[2] == ':' {
			p = p[1:]
		}
		return strings.ReplaceAll(p, "/", `\`), true
	}
	if p == "" {
		return "/", true
	}
	// Slash-separated whatever the host is: goos names the client's system.
	return slashpath.Clean(p), true
}

func fsPathToURI(path, goos string) string {
	if goos == "windows" {
		slashed := strings.ReplaceAll(path, `\`, "/")
		if strings.HasPrefix(slashed, "//") {
			rest := strings.TrimPrefix(slashed, "//")
			host, sharePath, _ := strings.Cut(rest, "/")
			u := url.URL{
				Scheme: "file",
				Host:   host,
				Path:   "/" + sharePath,
			}
			return u.String()
		}
		if len(slashed) >= 2 && isASCIIAlpha(slashed[0]) && slashed[1] == ':' {
			slashed = "/" + slashed
		}
		u := url.URL{
			Scheme: "file",
			Path:   slashed,
		}
		return u.String()
	}
	u := url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(filepath.Clean(path)),
	}
	return u.String()
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
