package handlers_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/handlerstest"
	"github.com/epistemer/i18n"
	"github.com/epistemer/i18n/i18ntest"
)

var (
	errTestAdmit = errors.New("admit error from guard")
)

func init() {
	// Register keys for table tests
	i18n.Key("route.tbltest.home", "Welcome Home")
	i18n.Key("route.tbltest.settings", "User Settings")
	i18n.Key("route.tbltest.billing", "Billing & Invoices")
	i18n.Key("route.tbltest.orders", "Order Management")
	i18n.Key("route.tbltest.hidden", "Hidden Page")
	i18n.Key("route.tbltest.searchable", "Searchable Home")
	i18n.Key("route.tbltest.account", "My Profile")
	i18n.Key("route.tbltest.router", "Route Planning")
	i18n.Key("route_param.tbltest.home.id", "Home ID")

	// Register 300 keys for BenchmarkSearch:
	// 100 public, 100 session, 100 permission (with 10 permissions).
	// Exactly 1 in 3 (100 total) contains "alpha", the rest contain "beta".
	for i := 0; i < 100; i++ {
		text := fmt.Sprintf("Benchmark public route %d beta", i)
		if i%3 == 0 {
			text = fmt.Sprintf("Benchmark public route %d alpha", i)
		}
		i18n.Key(fmt.Sprintf("route.tblbench.pub.%03d", i), text)
	}
	for i := 0; i < 100; i++ {
		text := fmt.Sprintf("Benchmark session route %d beta", i)
		if (100+i)%3 == 0 {
			text = fmt.Sprintf("Benchmark session route %d alpha", i)
		}
		i18n.Key(fmt.Sprintf("route.tblbench.sess.%03d", i), text)
	}
	for i := 0; i < 100; i++ {
		text := fmt.Sprintf("Benchmark permission route %d beta", i)
		if (200+i)%3 == 0 {
			text = fmt.Sprintf("Benchmark permission route %d alpha", i)
		}
		i18n.Key(fmt.Sprintf("route.tblbench.perm.%03d", i), text)
	}
}

func TestNewTable(t *testing.T) {
	t.Run("panics on a duplicate key across tables", func(t *testing.T) {
		r1 := handlers.Route{Key: "users.list"}
		r2 := handlers.Route{Key: "dashboard"}
		r3 := handlers.Route{Key: "users.list"}

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic on duplicate key across tables")
			}
			want := `handlers: NewTable: key "users.list" in two tables`
			if r != want {
				t.Fatalf("panic = %v, want %q", r, want)
			}
		}()

		handlers.NewTable([]handlers.Route{r1, r2}, []handlers.Route{r3})
	})

	t.Run("panics on a duplicate key within same table slice", func(t *testing.T) {
		r1 := handlers.Route{Key: "users.detail"}
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic on duplicate key within same table")
			}
			want := `handlers: NewTable: key "users.detail" in two tables`
			if r != want {
				t.Fatalf("panic = %v, want %q", r, want)
			}
		}()
		handlers.NewTable([]handlers.Route{r1, r1})
	})

	t.Run("ByKey", func(t *testing.T) {
		r1 := handlers.Route{Key: "alpha", Method: "GET"}
		r2 := handlers.Route{Key: "beta", Method: "POST"}
		tbl := handlers.NewTable([]handlers.Route{r1}, []handlers.Route{r2})

		got, ok := tbl.ByKey("alpha")
		if !ok || got.Key != "alpha" || got.Method != "GET" {
			t.Fatalf("ByKey(alpha) = (%+v, %v), want found", got, ok)
		}
		got2, ok := tbl.ByKey("beta")
		if !ok || got2.Key != "beta" || got2.Method != "POST" {
			t.Fatalf("ByKey(beta) = (%+v, %v), want found", got2, ok)
		}
		_, ok = tbl.ByKey("missing")
		if ok {
			t.Fatal("ByKey(missing) = true, want false")
		}
	})

	t.Run("Routes returns a copy", func(t *testing.T) {
		r1 := handlers.Route{
			Key:    "first",
			Params: []handlers.Param{{Name: "id"}},
		}
		r2 := handlers.Route{Key: "second"}
		r3 := handlers.Route{Key: "third"}

		tbl := handlers.NewTable([]handlers.Route{r1, r2}, []handlers.Route{r3})
		routes := tbl.Routes()
		if len(routes) != 3 {
			t.Fatalf("len(Routes()) = %d, want 3", len(routes))
		}
		if routes[0].Key != "first" || routes[1].Key != "second" || routes[2].Key != "third" {
			t.Fatalf("Routes() order wrong: %+v", routes)
		}

		// Mutate the returned slice
		routes[0].Key = "mutated"
		routes[0].Params[0].Name = "mutated_id"

		fresh := tbl.Routes()
		if fresh[0].Key != "first" {
			t.Errorf("table routes was mutated: got %q, want %q", fresh[0].Key, "first")
		}
		if fresh[0].Params[0].Name != "id" {
			t.Errorf("table params was mutated: got %q, want %q", fresh[0].Params[0].Name, "id")
		}
	})
}

type stubErrGuard struct {
	err error
}

func (s stubErrGuard) Prepare(handlers.Route) error { return nil }
func (s stubErrGuard) Check(*http.Request, handlers.Route) error {
	return s.err
}
func (s stubErrGuard) Admit(*http.Request) (func(handlers.Route) bool, error) {
	return nil, s.err
}

func setupTestTranslator(t testing.TB) *i18n.Translator {
	t.Helper()
	tr := i18ntest.New(t, i18n.Locale{Code: "en", NativeName: "English"})
	if err := tr.CreateLocale(i18n.Locale{Code: "fr", NativeName: "Français"}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route.tbltest.home", "Bienvenue à la maison"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route.tbltest.settings", "Paramètres utilisateur"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route.tbltest.billing", "Facturation"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route.tbltest.orders", "Gestion des commandes"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route.tbltest.account", "Mon profil"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route.tbltest.router", "Planification de route"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route_param.tbltest.home.id", "ID Maison"); err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestSearch(t *testing.T) {
	tr := setupTestTranslator(t)

	rHome := handlers.Route{Key: "tbltest.home", Auth: handlers.Public, Searchable: true}
	rSettings := handlers.Route{Key: "tbltest.settings", Auth: handlers.Session, Searchable: true}
	rBilling := handlers.Route{Key: "tbltest.billing", Auth: handlers.Permission, Permission: "perm.billing", Searchable: true}
	rOrders := handlers.Route{Key: "tbltest.orders", Auth: handlers.Permission, Permission: "perm.orders", Searchable: true}
	rHidden := handlers.Route{Key: "tbltest.hidden", Auth: handlers.Public, Searchable: false}
	rSearchable := handlers.Route{Key: "tbltest.searchable", Auth: handlers.Public, Searchable: true}
	rAccount := handlers.Route{Key: "tbltest.account", Auth: handlers.Public, Searchable: true}
	rRouter := handlers.Route{Key: "tbltest.router", Auth: handlers.Public, Searchable: true}

	tbl := handlers.NewTable([]handlers.Route{
		rHome, rSettings, rBilling, rOrders, rHidden, rSearchable, rAccount, rRouter,
	})

	t.Run("matches a translated text in another locale and the source text", func(t *testing.T) {
		reqFR := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "fr"))

		// Matching the translated text in "fr"
		hits, err := tbl.Search(reqFR, nil, tr, handlers.Query{Text: "bienvenue"})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hits) != 1 {
			t.Fatalf("len(hits) = %d, want 1", len(hits))
		}
		if hits[0].Route.Key != "tbltest.home" {
			t.Errorf("hit route key = %q, want tbltest.home", hits[0].Route.Key)
		}
		if hits[0].Text != "Bienvenue à la maison" {
			t.Errorf("hit text = %q, want 'Bienvenue à la maison'", hits[0].Text)
		}

		// Matching the source text ("Welcome Home") while in "fr"
		hits, err = tbl.Search(reqFR, nil, tr, handlers.Query{Text: "welcome"})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hits) != 1 {
			t.Fatalf("len(hits) = %d, want 1", len(hits))
		}
		if hits[0].Route.Key != "tbltest.home" {
			t.Errorf("hit route key = %q, want tbltest.home", hits[0].Route.Key)
		}
		if hits[0].Text != "Bienvenue à la maison" {
			t.Errorf("hit text = %q, want 'Bienvenue à la maison'", hits[0].Text)
		}
	})

	t.Run("ignores case", func(t *testing.T) {
		reqFR := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "fr"))
		for _, term := range []string{"BIENVENUE", "bIeNvEnUe", "WELCOME", "wElCoMe", "PARAMÈTRES", "paramètres"} {
			hits, err := tbl.Search(reqFR, handlerstest.Guard{Session: true}, tr, handlers.Query{Text: term})
			if err != nil {
				t.Fatalf("Search(%q) failed: %v", term, err)
			}
			if len(hits) != 1 {
				t.Fatalf("Search(%q) returned %d hits, want 1", term, len(hits))
			}
		}
	})

	t.Run("doesn't match on the key alone", func(t *testing.T) {
		reqEN := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))

		// tblOnlyAccount only has rAccount ("tbltest.account", label "My Profile")
		// It has key "route.tbltest.account" which contains "route".
		// But its label does not contain "route".
		tblOnlyAccount := handlers.NewTable([]handlers.Route{rAccount})
		hits, err := tblOnlyAccount.Search(reqEN, nil, tr, handlers.Query{Text: "route"})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hits) != 0 {
			t.Fatalf("Search for 'route' matched key alone, got %d hits, want 0", len(hits))
		}

		// In tbl, rRouter has label "Route Planning", which contains "route".
		hits, err = tbl.Search(reqEN, nil, tr, handlers.Query{Text: "route"})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hits) != 1 || hits[0].Route.Key != "tbltest.router" {
			t.Fatalf("Search for 'route' = %+v, want only tbltest.router", hits)
		}
	})

	t.Run("route_param.* entries never hit", func(t *testing.T) {
		reqEN := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))
		hits, err := tbl.Search(reqEN, nil, tr, handlers.Query{Text: "Home ID"})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hits) != 0 {
			t.Fatalf("Search matched route_param.*: %+v", hits)
		}
	})

	t.Run("Only filters", func(t *testing.T) {
		reqEN := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))
		hits, err := tbl.Search(reqEN, nil, tr, handlers.Query{
			Only: func(r handlers.Route) bool {
				return r.Searchable
			},
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		for _, h := range hits {
			if !h.Route.Searchable {
				t.Fatalf("found unsearchable route: %s", h.Route.Key)
			}
			if h.Route.Key == "tbltest.hidden" {
				t.Fatalf("hidden route returned when Only required Searchable")
			}
		}
	})

	t.Run("nil guard yields public routes only", func(t *testing.T) {
		reqEN := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))
		hits, err := tbl.Search(reqEN, nil, tr, handlers.Query{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("expected hits with nil guard")
		}
		for _, h := range hits {
			if h.Route.Auth != handlers.Public {
				t.Fatalf("non-public route returned with nil guard: %s (auth=%v)", h.Route.Key, h.Route.Auth)
			}
		}
	})

	t.Run("handlerstest.Guard without session yields public routes, with grant yields permission routes", func(t *testing.T) {
		reqEN := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))

		// No session
		gNoSession := handlerstest.Guard{Session: false}
		hits, err := tbl.Search(reqEN, gNoSession, tr, handlers.Query{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		for _, h := range hits {
			if h.Route.Auth != handlers.Public {
				t.Fatalf("non-public route returned without session: %s", h.Route.Key)
			}
		}

		// Session with grant for perm.billing
		gBilling := handlerstest.Guard{
			Session: true,
			Granted: map[string]bool{"perm.billing": true},
		}
		hits, err = tbl.Search(reqEN, gBilling, tr, handlers.Query{})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		var sawBilling, sawOrders, sawSettings bool
		for _, h := range hits {
			if h.Route.Key == "tbltest.billing" {
				sawBilling = true
			}
			if h.Route.Key == "tbltest.orders" {
				sawOrders = true
			}
			if h.Route.Key == "tbltest.settings" {
				sawSettings = true
			}
		}
		if !sawBilling {
			t.Error("expected tbltest.billing with grant, but not found")
		}
		if !sawSettings {
			t.Error("expected tbltest.settings with session, but not found")
		}
		if sawOrders {
			t.Error("saw tbltest.orders without its grant")
		}
	})

	t.Run("Limit", func(t *testing.T) {
		reqEN := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))

		// With Limit: 2
		hits2, err := tbl.Search(reqEN, nil, tr, handlers.Query{Limit: 2})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hits2) != 2 {
			t.Fatalf("len(hits2) = %d, want 2", len(hits2))
		}

		// With Limit: 1
		hits1, err := tbl.Search(reqEN, nil, tr, handlers.Query{Limit: 1})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hits1) != 1 {
			t.Fatalf("len(hits1) = %d, want 1", len(hits1))
		}
		if hits1[0].Route.Key != hits2[0].Route.Key {
			t.Errorf("Limit 1 gave %s, Limit 2 first hit is %s", hits1[0].Route.Key, hits2[0].Route.Key)
		}

		// With Limit: 0 (no limit)
		hitsAll, err := tbl.Search(reqEN, nil, tr, handlers.Query{Limit: 0})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(hitsAll) <= 2 {
			t.Fatalf("len(hitsAll) = %d, expected more than 2", len(hitsAll))
		}

		// Verify key order
		for i := 1; i < len(hitsAll); i++ {
			prevKey := "route." + hitsAll[i-1].Route.Key
			currKey := "route." + hitsAll[i].Route.Key
			if strings.Compare(prevKey, currKey) >= 0 {
				t.Errorf("hits out of key order: %s >= %s", prevKey, currKey)
			}
		}
	})

	t.Run("an Admit error is returned", func(t *testing.T) {
		reqEN := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))
		hits, err := tbl.Search(reqEN, stubErrGuard{err: errTestAdmit}, tr, handlers.Query{})
		if !errors.Is(err, errTestAdmit) {
			t.Fatalf("Search error = %v, want %v", err, errTestAdmit)
		}
		if hits != nil {
			t.Fatalf("expected nil hits on Admit error, got %+v", hits)
		}
	})
}

func BenchmarkSearch(b *testing.B) {
	// 300 routes: 100 public, 100 session, 100 permission with 10 permissions
	allRoutes := make([]handlers.Route, 0, 300)
	granted := make(map[string]bool, 10)
	for i := 0; i < 10; i++ {
		granted[fmt.Sprintf("bench.perm.%d", i)] = true
	}

	for i := 0; i < 100; i++ {
		allRoutes = append(allRoutes, handlers.Route{
			Key:        fmt.Sprintf("tblbench.pub.%03d", i),
			Auth:       handlers.Public,
			Searchable: true,
		})
	}
	for i := 0; i < 100; i++ {
		allRoutes = append(allRoutes, handlers.Route{
			Key:        fmt.Sprintf("tblbench.sess.%03d", i),
			Auth:       handlers.Session,
			Searchable: true,
		})
	}
	for i := 0; i < 100; i++ {
		allRoutes = append(allRoutes, handlers.Route{
			Key:        fmt.Sprintf("tblbench.perm.%03d", i),
			Auth:       handlers.Permission,
			Permission: fmt.Sprintf("bench.perm.%d", i%10),
			Searchable: true,
		})
	}

	tbl := handlers.NewTable(allRoutes)
	tr := i18ntest.New(b, i18n.Locale{Code: "en", NativeName: "English"})
	g := handlerstest.Guard{
		Session: true,
		Granted: granted,
	}
	r := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(b, tr, "en"))
	q := handlers.Query{Text: "alpha"} // matches about a third of them (exactly 100)

	// Quick correctness verification before benchmark loop
	hits, err := tbl.Search(r, g, tr, q)
	if err != nil {
		b.Fatalf("Search failed: %v", err)
	}
	if len(hits) != 100 {
		b.Fatalf("len(hits) = %d, want 100 (one third of 300)", len(hits))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		hits, err := tbl.Search(r, g, tr, q)
		if err != nil || len(hits) != 100 {
			b.Fatalf("benchmark failed: hits=%d err=%v", len(hits), err)
		}
	}
}
