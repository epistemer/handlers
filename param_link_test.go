package handlers_test

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/epistemer/handlers"
	"github.com/epistemer/i18n/i18ntest"
)

// upperCanon is a text type's Canon whose canonical form is upper case: it must
// normalise "ab" to "AB", as a builder writing MarshalText would.
func upperCanon(s string) (string, error) {
	if strings.ContainsAny(s, "!") {
		return "", errors.New("bad code")
	}
	return strings.ToUpper(s), nil
}

func linkTable() *handlers.Table {
	return handlers.NewTable([]handlers.Route{
		{Key: "a.home", Method: "GET", Prefix: "/admin", Pattern: "/"},
		{Key: "a.kinds", Method: "GET", Prefix: "/admin", Pattern: "/k/{s}/{b}/{i8}/{u}/{t}", Params: []handlers.Param{
			{Name: "s", Kind: handlers.KindString},
			{Name: "b", Kind: handlers.KindBool},
			{Name: "i8", Kind: handlers.KindInt, Bits: 8},
			{Name: "u", Kind: handlers.KindUint, Bits: 64},
			{Name: "t", Kind: handlers.KindText, Canon: upperCanon},
		}},
		{Key: "a.files", Method: "GET", Prefix: "", Pattern: "/files/{path...}", Params: []handlers.Param{
			{Name: "path", Rest: true, Kind: handlers.KindString},
		}},
	})
}

func TestTableLink(t *testing.T) {
	tbl := linkTable()

	t.Run("writes every kind as its builder does", func(t *testing.T) {
		l, err := tbl.Link("a.kinds", url.Values{
			"s": {"a b/c"}, "b": {"TRUE"}, "i8": {"-0012"}, "u": {"18446744073709551615"}, "t": {"ab"},
			"extra": {"ignored"},
		})
		if err != nil {
			t.Fatal(err)
		}
		want := "/admin/k/a%20b%2Fc/true/-12/18446744073709551615/AB"
		if l.URL() != want {
			t.Errorf("URL = %q, want %q", l.URL(), want)
		}
		if l.Route().Key != "a.kinds" || l.Method() != "GET" {
			t.Errorf("Route = %q %q", l.Route().Key, l.Method())
		}
	})

	t.Run("a route without parameters", func(t *testing.T) {
		l, err := tbl.Link("a.home", nil)
		if err != nil || l.URL() != "/admin/" {
			t.Errorf("Link = %q, %v; want /admin/", l.URL(), err)
		}
	})

	t.Run("a rest parameter keeps its slashes", func(t *testing.T) {
		l, err := tbl.Link("a.files", url.Values{"path": {"a b/c"}})
		if err != nil || l.URL() != "/files/a%20b/c" {
			t.Errorf("Link = %q, %v; want /files/a%%20b/c", l.URL(), err)
		}
	})

	t.Run("an unknown key is ErrNotFound", func(t *testing.T) {
		if _, err := tbl.Link("a.missing", nil); !errors.Is(err, handlers.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
		var nilTable *handlers.Table
		if _, err := nilTable.Link("a.home", nil); !errors.Is(err, handlers.ErrNotFound) {
			t.Errorf("nil table: err = %v, want ErrNotFound", err)
		}
	})

	good := url.Values{"s": {"x"}, "b": {"false"}, "i8": {"1"}, "u": {"1"}, "t": {"q"}}
	bad := []struct {
		name string
		key  string
		set  func(v url.Values)
	}{
		{"missing", "a.kinds", func(v url.Values) { delete(v, "s") }},
		{"given twice", "a.kinds", func(v url.Values) { v["s"] = []string{"x", "y"} }},
		{"empty", "a.kinds", func(v url.Values) { v.Set("s", "") }},
		{"bool", "a.kinds", func(v url.Values) { v.Set("b", "on") }},
		{"int8 range", "a.kinds", func(v url.Values) { v.Set("i8", "128") }},
		{"uint negative", "a.kinds", func(v url.Values) { v.Set("u", "-1") }},
		{"text refused", "a.kinds", func(v url.Values) { v.Set("t", "a!") }},
		{"rest leading slash", "a.files", func(v url.Values) { v.Set("path", "/evil.example") }},
		{"rest trailing slash", "a.files", func(v url.Values) { v.Set("path", "a/") }},
		{"rest empty segment", "a.files", func(v url.Values) { v.Set("path", "a//b") }},
	}
	for _, tc := range bad {
		t.Run("bad: "+tc.name, func(t *testing.T) {
			v := url.Values{}
			for k, vs := range good {
				v[k] = append([]string(nil), vs...)
			}
			v.Set("path", "a/b")
			tc.set(v)
			if _, err := tbl.Link(tc.key, v); !errors.Is(err, handlers.ErrBadParam) {
				t.Errorf("err = %v, want ErrBadParam", err)
			}
		})
	}

	t.Run("an unknown kind, a rest that isn't a string, and a pattern without the parameter are refused", func(t *testing.T) {
		tb := handlers.NewTable([]handlers.Route{
			{Key: "a.k", Pattern: "/{x}", Params: []handlers.Param{{Name: "x", Kind: handlers.ParamKind(9)}}},
			{Key: "a.r", Pattern: "/{x...}", Params: []handlers.Param{{Name: "x", Rest: true, Kind: handlers.KindInt, Bits: 64}}},
			{Key: "a.p", Pattern: "/{x}/{y}", Params: []handlers.Param{{Name: "x", Kind: handlers.KindString}}},
			{Key: "a.o", Pattern: "/{x", Params: []handlers.Param{{Name: "x", Kind: handlers.KindString}}},
		})
		for _, k := range []string{"a.k", "a.r", "a.p", "a.o"} {
			if _, err := tb.Link(k, url.Values{"x": {"1"}}); !errors.Is(err, handlers.ErrBadParam) {
				t.Errorf("%s: err = %v, want ErrBadParam", k, err)
			}
		}
	})

	t.Run("{$} writes nothing", func(t *testing.T) {
		tb := handlers.NewTable([]handlers.Route{{Key: "a.d", Prefix: "/p", Pattern: "/posts/{$}"}})
		if l, err := tb.Link("a.d", nil); err != nil || l.URL() != "/p/posts/" {
			t.Errorf("Link = %q, %v; want /p/posts/", l.URL(), err)
		}
	})

	t.Run("a text parameter without Canon is refused", func(t *testing.T) {
		tb := handlers.NewTable([]handlers.Route{{Key: "a.t", Pattern: "/{t}", Params: []handlers.Param{{Name: "t", Kind: handlers.KindText}}}})
		if _, err := tb.Link("a.t", url.Values{"t": {"x"}}); !errors.Is(err, handlers.ErrBadParam) {
			t.Errorf("err = %v, want ErrBadParam", err)
		}
	})
}

func TestHitLink(t *testing.T) {
	tr := setupTestTranslator(t)
	tbl := handlers.NewTable([]handlers.Route{
		{Key: "tbltest.home", Method: "GET", Prefix: "/admin", Pattern: "/", Auth: handlers.Public, Searchable: true},
		{Key: "tbltest.account", Method: "GET", Prefix: "/admin", Pattern: "/accounts/{id}", Auth: handlers.Public, Searchable: true,
			Params: []handlers.Param{{Name: "id", Kind: handlers.KindInt, Bits: 64}}},
	})
	req := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))
	hits, err := tbl.Search(req, nil, tr, handlers.Query{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, h := range hits {
		got[h.Route.Key] = h.Link.URL()
	}
	if got["tbltest.home"] != "/admin/" {
		t.Errorf("home's link = %q, want /admin/", got["tbltest.home"])
	}
	if l, ok := got["tbltest.account"]; !ok || l != "" {
		t.Errorf("account's link = %q (found %v), want the zero Link", l, ok)
	}
	for _, h := range hits {
		if h.Route.Key == "tbltest.home" && h.Link.Route().Key != "tbltest.home" {
			t.Errorf("home's link goes to %q", h.Link.Route().Key)
		}
	}
}

func TestParamKindString(t *testing.T) {
	for k, want := range map[handlers.ParamKind]string{
		handlers.KindString: "string", handlers.KindBool: "bool", handlers.KindInt: "int",
		handlers.KindUint: "uint", handlers.KindText: "text", handlers.ParamKind(9): "ParamKind(9)",
	} {
		if k.String() != want {
			t.Errorf("%d.String() = %q, want %q", k, k.String(), want)
		}
	}
}
