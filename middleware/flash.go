package middleware

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/epistemer/errors"
	"github.com/epistemer/i18n"
)

// Flash message levels.
const (
	LevelInfo    = "info"
	LevelSuccess = "success"
	LevelWarning = "warning"
	LevelError   = "error"
)

const (
	defaultFlashName   = "flash"
	flashCookieMaxAge  = 300
	maxCookieReadBytes = 4 << 10 // 4 KiB
	maxCookieWriteByte = 3 << 10 // 3 KiB
)

type flashOptions struct {
	name     string
	insecure bool
}

// FlashOption configures a Flash.
type FlashOption func(*flashOptions)

// FlashName sets the flash cookie's base name (default "flash").
func FlashName(name string) FlashOption {
	return func(o *flashOptions) {
		if name != "" {
			o.name = name
		}
	}
}

// FlashInsecure omits the __Host- prefix and the Secure attribute on the flash
// cookie, for local development over plain HTTP.
func FlashInsecure() FlashOption {
	return func(o *flashOptions) {
		o.insecure = true
	}
}

// Flash carries messages across redirects in a cookie.
type Flash struct {
	name     string
	insecure bool
}

// NewFlash constructs a Flash with the given options.
func NewFlash(opts ...FlashOption) *Flash {
	o := flashOptions{name: defaultFlashName}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return &Flash{
		name:     o.name,
		insecure: o.insecure,
	}
}

func (f *Flash) cookieName() string {
	if f.insecure {
		return f.name
	}
	return "__Host-" + f.name
}

type flashCtxKey struct {
	f *Flash
}

type flashState struct {
	incoming   []flashMsg
	dirty      bool // the cookie held something that was dropped on reading
	canReceive bool
	mu         sync.Mutex
	added      []flashMsg
}

type flashMsg struct {
	Level string
	Key   string
	Args  []any
	Text  string
}

func (m flashMsg) toTer() i18n.Ter {
	if m.Key == "" {
		return i18n.Static(m.Text)
	}
	return flashKeyTer{
		key:  i18n.DynamicWrap(m.Key),
		args: m.Args,
	}
}

type flashKeyTer struct {
	key  i18n.TKer
	args []any
}

func (t flashKeyTer) T(ctx context.Context, _ ...any) string {
	return t.key.T(ctx, t.args...)
}

// Middleware reads the request's flash cookie into the context and writes it
// back with the response's header when messages are delivered or added.
func (f *Flash) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var incoming []flashMsg
		dirty := false
		if c, err := r.Cookie(f.cookieName()); err == nil {
			incoming, dirty = f.decodeCookie(r.Context(), c.Value)
		}

		canReceive := r.Method != http.MethodHead && (!HTMX(r) || Boosted(r))
		st := &flashState{
			incoming:   incoming,
			dirty:      dirty,
			canReceive: canReceive,
		}
		req := r.WithContext(context.WithValue(r.Context(), flashCtxKey{f}, st))
		rw := &flashWriter{
			w:  w,
			r:  req,
			f:  f,
			st: st,
		}

		next.ServeHTTP(rw, req)
		rw.finish()
	})
}

// Add carries msg to the next page. It panics on a level other than the four
// Level constants, and outside f.Middleware.
func (f *Flash) Add(ctx context.Context, level string, msg i18n.Ter, args ...any) {
	if !validLevel(level) {
		panic(fmt.Sprintf("middleware: unknown flash level %q", level))
	}
	if f == nil || ctx == nil {
		panic("middleware: Flash.Add outside its Middleware")
	}
	st, ok := ctx.Value(flashCtxKey{f}).(*flashState)
	if !ok || st == nil {
		panic("middleware: Flash.Add outside its Middleware")
	}

	m := flashMsg{Level: level}
	if msg == nil {
		m.Text = ""
	} else if _, isErr := msg.(error); isErr {
		m.Text = msg.T(ctx, args...)
	} else if k, isKer := msg.(i18n.Ker); isKer && k.K() != "" {
		m.Key = k.K()
		m.Args = snapshotArgs(ctx, args)
	} else {
		m.Text = msg.T(ctx, args...)
	}

	st.mu.Lock()
	st.added = append(st.added, m)
	st.mu.Unlock()
}

// Messages returns the messages this request delivers, oldest first, with
// their levels. It yields nothing outside f.Middleware, for HEAD requests, or
// for non-boosted htmx requests, and never includes messages added during this
// request.
func (f *Flash) Messages(ctx context.Context) iter.Seq2[string, i18n.Ter] {
	return func(yield func(string, i18n.Ter) bool) {
		if f == nil || ctx == nil {
			return
		}
		st, ok := ctx.Value(flashCtxKey{f}).(*flashState)
		if !ok || st == nil || !st.canReceive {
			return
		}
		for _, m := range st.incoming {
			if !yield(m.Level, m.toTer()) {
				return
			}
		}
	}
}

func validLevel(level string) bool {
	switch level {
	case LevelInfo, LevelSuccess, LevelWarning, LevelError:
		return true
	default:
		return false
	}
}

func snapshotArgs(ctx context.Context, args []any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, len(args))
	for i, arg := range args {
		out[i] = snapshotArg(ctx, arg)
	}
	return out
}

func snapshotArg(ctx context.Context, arg any) any {
	if arg == nil {
		return "<nil>"
	}
	// Errors are tested before keys, since a declared error also implements i18n.Ker.
	if err, ok := arg.(error); ok {
		return errors.T(ctx, err)
	}
	if k, ok := arg.(i18n.Ker); ok {
		return i18n.DynamicWrap(k.K())
	}
	if ter, ok := arg.(i18n.Ter); ok {
		return ter.T(ctx)
	}
	v := reflect.ValueOf(arg)
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint()
	case reflect.Float32:
		return float32(v.Float())
	case reflect.Float64:
		return v.Float()
	default:
		return fmt.Sprint(arg)
	}
}

type wireMsg struct {
	Level string    `json:"l"`
	Key   string    `json:"k,omitempty"`
	Args  []wireArg `json:"a,omitempty"`
	Text  string    `json:"t,omitempty"`
}

type wireArg struct {
	Tag string          `json:"t"`
	Val json.RawMessage `json:"v"`
}

func encodeWireArg(arg any) wireArg {
	switch v := arg.(type) {
	case i18n.Ker:
		b, _ := json.Marshal(v.K())
		return wireArg{Tag: "k", Val: b}
	case string:
		b, _ := json.Marshal(v)
		return wireArg{Tag: "s", Val: b}
	case bool:
		b, _ := json.Marshal(v)
		return wireArg{Tag: "b", Val: b}
	case int64:
		b, _ := json.Marshal(v)
		return wireArg{Tag: "i", Val: b}
	case uint64:
		b, _ := json.Marshal(v)
		return wireArg{Tag: "u", Val: b}
	case float32:
		if b, err := json.Marshal(v); err == nil {
			return wireArg{Tag: "f32", Val: b}
		}
	case float64:
		if b, err := json.Marshal(v); err == nil {
			return wireArg{Tag: "f64", Val: b}
		}
	}
	b, _ := json.Marshal(fmt.Sprint(arg))
	return wireArg{Tag: "s", Val: b}
}

func decodeWireArg(a wireArg) (any, bool) {
	trimmed := bytes.TrimSpace(a.Val)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, false
	}
	switch a.Tag {
	case "s":
		var v string
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, false
		}
		return v, true
	case "b":
		var v bool
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, false
		}
		return v, true
	case "i":
		var v int64
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, false
		}
		return v, true
	case "u":
		var v uint64
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, false
		}
		return v, true
	case "f32":
		var v float32
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, false
		}
		return v, true
	case "f64":
		var v float64
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, false
		}
		return v, true
	case "k":
		var v string
		if err := json.Unmarshal(trimmed, &v); err != nil || v == "" {
			return nil, false
		}
		return i18n.DynamicWrap(v), true
	default:
		return nil, false
	}
}

func encodeMessages(msgs []flashMsg) string {
	if len(msgs) == 0 {
		return ""
	}
	wire := make([]wireMsg, len(msgs))
	for i, m := range msgs {
		wm := wireMsg{Level: m.Level}
		if m.Key != "" {
			wm.Key = m.Key
			if len(m.Args) > 0 {
				wm.Args = make([]wireArg, len(m.Args))
				for j, a := range m.Args {
					wm.Args[j] = encodeWireArg(a)
				}
			}
		} else {
			wm.Text = m.Text
		}
		wire[i] = wm
	}
	raw, _ := json.Marshal(wire)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// decodeCookie returns the cookie's messages, and whether it held anything
// that was dropped, so the cookie is rewritten.
func (f *Flash) decodeCookie(ctx context.Context, val string) (msgs []flashMsg, dirty bool) {
	if val == "" {
		return nil, false
	}
	if len(val) > maxCookieReadBytes {
		slog.Default().DebugContext(ctx, "middleware: flash cookie exceeds 4 KiB")
		return nil, true
	}
	var (
		raw []byte
		err error
	)
	if strings.ContainsRune(val, '=') {
		raw, err = base64.URLEncoding.DecodeString(val)
	} else {
		raw, err = base64.RawURLEncoding.DecodeString(val)
	}
	if err != nil {
		slog.Default().DebugContext(ctx, "middleware: malformed flash cookie", "err", err)
		return nil, true
	}
	var wire []wireMsg
	if err := json.Unmarshal(raw, &wire); err != nil {
		slog.Default().DebugContext(ctx, "middleware: malformed flash cookie JSON", "err", err)
		return nil, true
	}

	var out []flashMsg
	for _, wm := range wire {
		if !validLevel(wm.Level) {
			slog.Default().DebugContext(ctx, "middleware: unknown flash level in cookie", "level", wm.Level)
			dirty = true
			continue
		}
		if wm.Key == "" {
			if len(wm.Args) > 0 {
				slog.Default().DebugContext(ctx, "middleware: malformed flash message in cookie")
				dirty = true
				continue
			}
			out = append(out, flashMsg{Level: wm.Level, Text: wm.Text})
			continue
		}
		var args []any
		badArg := false
		if len(wm.Args) > 0 {
			args = make([]any, len(wm.Args))
			for i, wa := range wm.Args {
				decoded, ok := decodeWireArg(wa)
				if !ok {
					slog.Default().DebugContext(ctx, "middleware: unknown or malformed flash arg in cookie", "tag", wa.Tag)
					badArg = true
					break
				}
				args[i] = decoded
			}
		}
		if badArg {
			dirty = true
			continue
		}
		out = append(out, flashMsg{
			Level: wm.Level,
			Key:   wm.Key,
			Args:  args,
		})
	}
	return out, dirty
}

type flashWriter struct {
	w        http.ResponseWriter
	r        *http.Request
	f        *Flash
	st       *flashState
	decided  bool
	hijacked bool
	held     int // a final status held until the first write, when no Content-Type is set
}

func (w *flashWriter) Header() http.Header {
	return w.w.Header()
}

// WriteHeader decides at once when the content type is known; otherwise it
// holds the status until the first write can be sniffed, as net/http does.
func (w *flashWriter) WriteHeader(code int) {
	if code >= 100 && code <= 199 {
		w.w.WriteHeader(code)
		return
	}
	if w.decided || w.hijacked {
		w.w.WriteHeader(code)
		return
	}
	if w.held != 0 {
		return // a second WriteHeader is superfluous, as in net/http
	}
	if ct := w.contentType(nil); ct != "" {
		w.decide(code, ct)
		w.w.WriteHeader(code)
		return
	}
	w.held = code
}

func (w *flashWriter) Write(p []byte) (int, error) {
	if !w.decided && !w.hijacked {
		if len(p) == 0 {
			return 0, nil
		}
		status := http.StatusOK
		if w.held != 0 {
			status = w.held
		}
		w.decide(status, w.contentType(p))
		w.sendHeld()
	}
	return w.w.Write(p)
}

// sendHeld sends a held status after the decision.
func (w *flashWriter) sendHeld() {
	if w.held != 0 {
		code := w.held
		w.held = 0
		w.w.WriteHeader(code)
	}
}

// finish decides for a handler that returned without writing a body.
func (w *flashWriter) finish() {
	if w.hijacked {
		return
	}
	if !w.decided {
		status := http.StatusOK
		if w.held != 0 {
			status = w.held
		}
		w.decide(status, w.contentType(nil))
	}
	w.sendHeld()
}

func (w *flashWriter) Unwrap() http.ResponseWriter {
	return w.w
}

func (w *flashWriter) Flush() {
	_ = w.FlushError()
}

func (w *flashWriter) FlushError() error {
	if !w.decided && !w.hijacked {
		status := http.StatusOK
		if w.held != 0 {
			status = w.held
		}
		w.decide(status, w.contentType(nil))
	}
	w.sendHeld()
	return http.NewResponseController(w.w).Flush()
}

func (w *flashWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw := w.w
	for {
		if h, ok := rw.(http.Hijacker); ok {
			conn, brw, err := h.Hijack()
			if err == nil {
				w.hijacked = true
			}
			return conn, brw, err
		}
		u, ok := rw.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return nil, nil, http.ErrNotSupported
		}
		rw = u.Unwrap()
	}
}

func (w *flashWriter) contentType(firstWrite []byte) string {
	hdr := w.w.Header()
	if vals, ok := hdr["Content-Type"]; ok {
		if len(vals) == 0 {
			return ""
		}
		return vals[0]
	}
	if len(firstWrite) > 0 {
		return http.DetectContentType(firstWrite)
	}
	return ""
}

func isHTMLContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		base, _, _ := strings.Cut(ct, ";")
		return strings.EqualFold(strings.TrimSpace(base), "text/html")
	}
	return mt == "text/html"
}

func (w *flashWriter) decide(status int, ct string) {
	w.decided = true
	ctx := w.r.Context()

	w.st.mu.Lock()
	added := w.st.added
	w.st.mu.Unlock()

	if status >= http.StatusInternalServerError {
		added = nil
	}

	delivered := w.st.canReceive &&
		len(w.st.incoming) > 0 &&
		isHTMLContentType(ct) &&
		(status < http.StatusMultipleChoices || (status >= http.StatusBadRequest && status <= 599))

	if !delivered && len(added) == 0 && !w.st.dirty {
		return
	}

	// A message added now that can't fit on its own is dropped first, so it
	// doesn't push out older ones that do.
	kept := added[:0:0]
	for _, m := range added {
		if len(encodeMessages([]flashMsg{m})) > maxCookieWriteByte {
			slog.Default().WarnContext(ctx, "middleware: flash message exceeds 3 KiB, dropped")
			continue
		}
		kept = append(kept, m)
	}

	var remaining []flashMsg
	if !delivered {
		remaining = append(remaining, w.st.incoming...)
	}
	remaining = append(remaining, kept...)

	encoded := encodeMessages(remaining)
	if len(encoded) > maxCookieWriteByte {
		dropped := 0
		for len(remaining) > 0 && len(encoded) > maxCookieWriteByte {
			remaining = remaining[1:]
			dropped++
			encoded = encodeMessages(remaining)
		}
		slog.Default().WarnContext(ctx, "middleware: flash cookie exceeds 3 KiB, dropped oldest messages", "dropped", dropped)
	}

	if len(remaining) == 0 && len(w.st.incoming) == 0 && !w.st.dirty {
		return
	}

	cookie := &http.Cookie{
		Name:     w.f.cookieName(),
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   !w.f.insecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   flashCookieMaxAge,
	}
	if len(remaining) == 0 {
		cookie.Value = ""
		cookie.MaxAge = -1
	}
	http.SetCookie(w.w, cookie)
}
