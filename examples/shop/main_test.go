package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/epistemer/auth"
	"github.com/epistemer/auth/access"
	"github.com/epistemer/auth/authtest"
	"github.com/epistemer/genx/genxtest"
	"github.com/epistemer/handlers/examples/shop/apiroutes"
	"github.com/epistemer/handlers/examples/shop/internal/adminroutes"
	"github.com/epistemer/handlers/examples/shop/internal/models"
	"github.com/epistemer/handlers/examples/shop/internal/settings"
	"github.com/epistemer/handlers/examples/shop/internal/shop"
	"github.com/epistemer/handlers/handlerstest"
	"github.com/epistemer/i18n"
	"github.com/epistemer/i18n/i18ntest"
	bolt "go.etcd.io/bbolt"
)

const testPassword = "orange tiger juggles 47 lamps"

func TestGenerated(t *testing.T) {
	genxtest.AssertGenerated(t)
}

func TestPublicRoutes(t *testing.T) {
	handlerstest.AssertPublic(t, adminroutes.Routes, "sign_in", "sign_in.submit", "sign_in.code")
	handlerstest.AssertPublic(t, apiroutes.Routes, "api.sign_in", "api.sign_in_code")
}

func TestVet(t *testing.T) {
	cmd := exec.Command("go", "tool", "handlers", "vet", "-strict", "./...")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("handlers vet -strict ./...: %v\n%s", err, out)
	}
}

type testWorld struct {
	t     *testing.T
	kit   *authtest.Kit
	ac    *access.Access
	tr    *i18n.Translator
	store *shop.Store
	srv   *httptest.Server
}

func newTestWorld(t *testing.T) *testWorld {
	t.Helper()

	tr := i18ntest.New(t, sourceLocale)
	if err := tr.CreateLocale(i18n.Locale{Code: "fr", NativeName: "Français"}); err != nil {
		t.Fatal(err)
	}

	db, err := bolt.Open(filepath.Join(t.TempDir(), "access.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ac, err := access.New(db)
	if err != nil {
		t.Fatal(err)
	}

	kit := authtest.New(t)
	// Align the fake clock with wall time so http.CookieJar does not drop
	// flow/session cookies whose MaxAge/Expires come from the test clock.
	kit.Clock.Advance(time.Since(kit.Clock.Now()))
	ctx := context.Background()

	// Sara is a shop manager holding orders.manage and orders.read.
	// Omar is a verified customer with no admin/order permissions.
	// Tariq holds only orders.read (the API permission, taken by /admin/orders).
	kit.AddUser("sara@toyshop.example", auth.User{ID: "u1", Name: "Sara"}, "sara@toyshop.example")
	kit.AddUser("omar@toyshop.example", auth.User{ID: "u2", Name: "Omar"}, "omar@toyshop.example")
	kit.AddUser("tariq@toyshop.example", auth.User{ID: "u3", Name: "Tariq"}, "tariq@toyshop.example")
	for _, uid := range []auth.UserID{"u1", "u2", "u3"} {
		if err := kit.Auth.SetPassword(ctx, uid, testPassword); err != nil {
			t.Fatal(err)
		}
	}

	managers, err := ac.CreateGroup(ctx, "Managers")
	if err != nil {
		t.Fatal(err)
	}
	if err := ac.Grant(ctx, managers.ID, adminroutes.PermOrdersManage); err != nil {
		t.Fatal(err)
	}
	if err := ac.Grant(ctx, managers.ID, apiroutes.PermOrdersRead); err != nil {
		t.Fatal(err)
	}
	if err := ac.Join(ctx, "u1", managers.ID); err != nil {
		t.Fatal(err)
	}

	readers, err := ac.CreateGroup(ctx, "Readers")
	if err != nil {
		t.Fatal(err)
	}
	if err := ac.Grant(ctx, readers.ID, apiroutes.PermOrdersRead); err != nil {
		t.Fatal(err)
	}
	if err := ac.Join(ctx, "u3", readers.ID); err != nil {
		t.Fatal(err)
	}

	store := shop.NewStore()
	h, err := newHandler(kit.Auth, ac, tr, store)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)

	return &testWorld{
		t:     t,
		kit:   kit,
		ac:    ac,
		tr:    tr,
		store: store,
		srv:   srv,
	}
}

func (w *testWorld) browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	c := w.srv.Client()
	c.Jar = jar
	return c
}

func (w *testWorld) signInAdmin(c *http.Client, email string) {
	w.t.Helper()
	resp, err := c.PostForm(w.srv.URL+adminroutes.SignInSubmit().URL(), url.Values{
		"login":    {email},
		"password": {testPassword},
	})
	if err != nil {
		w.t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		w.t.Fatalf("sign-in submit status = %d, want 200", resp.StatusCode)
	}

	w.kit.Clock.Advance(time.Second)
	msg, ok := w.kit.LastMessage(email)
	if !ok || msg.Code == "" {
		w.t.Fatalf("no verification code sent to %s", email)
	}

	resp, err = c.PostForm(w.srv.URL+adminroutes.SignInCode().URL(), url.Values{
		"code": {msg.Code},
	})
	if err != nil {
		w.t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		w.t.Fatalf("sign-in code status = %d, want 200", resp.StatusCode)
	}
}

func (w *testWorld) signInAPI(email string) string {
	w.t.Helper()
	client := w.srv.Client()

	body1, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": testPassword,
	})
	resp, err := client.Post(w.srv.URL+apiroutes.SignIn().URL(), "application/json", bytes.NewReader(body1))
	if err != nil {
		w.t.Fatal(err)
	}
	var step1 struct {
		Next   string `json:"next"`
		Flow   string `json:"flow"`
		Access string `json:"access"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&step1); err != nil {
		resp.Body.Close()
		w.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		w.t.Fatalf("API sign-in status = %d, want 200", resp.StatusCode)
	}
	if step1.Access != "" {
		return step1.Access
	}
	if step1.Next != "code" || step1.Flow == "" {
		w.t.Fatalf("unexpected API sign-in step: %+v", step1)
	}

	w.kit.Clock.Advance(time.Second)
	msg, ok := w.kit.LastMessage(email)
	if !ok || msg.Code == "" {
		w.t.Fatalf("no verification code sent to %s", email)
	}

	body2, _ := json.Marshal(map[string]string{
		"flow": step1.Flow,
		"code": msg.Code,
	})
	resp, err = client.Post(w.srv.URL+apiroutes.SignInCode().URL(), "application/json", bytes.NewReader(body2))
	if err != nil {
		w.t.Fatal(err)
	}
	var step2 struct {
		Access string `json:"access"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&step2); err != nil {
		resp.Body.Close()
		w.t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || step2.Access == "" {
		w.t.Fatalf("API sign-in code failed: status=%d access=%q", resp.StatusCode, step2.Access)
	}
	return step2.Access
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAdminPagesAndAuth(t *testing.T) {
	w := newTestWorld(t)

	if err := w.tr.Set("fr", "route_param.orders.cancel.id", "ID commande"); err != nil {
		t.Fatal(err)
	}
	if err := w.tr.Set("fr", "enum.order_status.paid", "payée"); err != nil {
		t.Fatal(err)
	}

	// Unauthenticated request -> 401 in ErrorPage HTML format.
	anon := w.browser()
	resp, err := anon.Get(w.srv.URL + adminroutes.Order(1).URL())
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anon status = %d, want 401", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("anon Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(body, "<h1>401 Unauthorized</h1>") {
		t.Errorf("anon error page = %q", body)
	}

	// Public sign-in page is accessible without a session.
	resp, err = anon.Get(w.srv.URL + adminroutes.SignInPage().URL())
	if err != nil {
		t.Fatal(err)
	}
	_ = readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("public sign-in page status = %d, want 200", resp.StatusCode)
	}

	// Verified user without orders.manage or orders.read -> 403 on orders page, 200 on products.
	omar := w.browser()
	w.signInAdmin(omar, "omar@toyshop.example")

	resp, err = omar.Get(w.srv.URL + adminroutes.Orders().URL())
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "<h1>403 Forbidden</h1>") {
		t.Errorf("omar orders list = (%d, %q), want 403 error page", resp.StatusCode, body)
	}

	resp, err = omar.Get(w.srv.URL + adminroutes.Order(1).URL())
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "<h1>403 Forbidden</h1>") {
		t.Errorf("omar order detail = (%d, %q), want 403 error page", resp.StatusCode, body)
	}

	// Verified user granted ONLY the API's permission (orders.read from apiroutes):
	// can open /admin/orders (takes orders.read from apiroutes), but 403 on /admin/orders/1 (orders.manage).
	tariq := w.browser()
	w.signInAdmin(tariq, "tariq@toyshop.example")

	resp, err = tariq.Get(w.srv.URL + adminroutes.Orders().URL())
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Orders") {
		t.Errorf("tariq orders list = (%d, %q), want 200", resp.StatusCode, body)
	}

	resp, err = tariq.Get(w.srv.URL + adminroutes.Order(1).URL())
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "<h1>403 Forbidden</h1>") {
		t.Errorf("tariq order detail = (%d, %q), want 403 error page", resp.StatusCode, body)
	}

	resp, err = omar.Get(w.srv.URL + adminroutes.Product(1).URL())
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Wooden Train") {
		t.Errorf("omar product profile = (%d, %q)", resp.StatusCode, body)
	}

	// Unknown product -> 404 declared error.
	resp, err = omar.Get(w.srv.URL + adminroutes.Product(999).URL())
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "product not found") {
		t.Errorf("missing product = (%d, %q)", resp.StatusCode, body)
	}

	// Verified user holding orders.manage -> 200 on order detail (translated in French).
	sara := w.browser()
	w.signInAdmin(sara, "sara@toyshop.example")

	req, _ := http.NewRequest("GET", w.srv.URL+adminroutes.Order(1).URL(), nil)
	req.Header.Set("Accept-Language", "fr")
	resp, err = sara.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "payée") {
		t.Errorf("sara order detail fr = (%d, %q)", resp.StatusCode, body)
	}

	// Cancel action with bad ID in French -> 400 with translated parameter label.
	badReq, _ := http.NewRequest("POST", w.srv.URL+adminroutes.Prefix+"/orders/notanint/cancel", nil)
	badReq.Header.Set("Accept-Language", "fr")
	resp, err = sara.Do(badReq)
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "ID commande is not valid") {
		t.Errorf("bad cancel id fr = (%d, %q), want 400 with 'ID commande is not valid'", resp.StatusCode, body)
	}

	// Valid cancel -> succeeds and transitions order to cancelled; second cancel -> 409 Conflict.
	resp, err = sara.Post(w.srv.URL+adminroutes.OrderCancel(1).URL(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "cancelled") {
		t.Errorf("cancel order 1 = (%d, %q)", resp.StatusCode, body)
	}

	resp, err = sara.Post(w.srv.URL+adminroutes.OrderCancel(1).URL(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "order already cancelled") {
		t.Errorf("double cancel = (%d, %q), want 409 'order already cancelled'", resp.StatusCode, body)
	}
}

func TestAPISurface(t *testing.T) {
	w := newTestWorld(t)
	client := w.srv.Client()

	decodeErr := func(t *testing.T, resp *http.Response) (string, string) {
		t.Helper()
		defer resp.Body.Close()
		var body struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.Code, body.Message
	}

	// No bearer -> 401.
	resp, err := client.Get(w.srv.URL + apiroutes.ListOrders().URL())
	if err != nil {
		t.Fatal(err)
	}
	code, _ := decodeErr(t, resp)
	if resp.StatusCode != http.StatusUnauthorized || code != "errors.auth.no_session" {
		t.Errorf("no bearer = (%d, %q)", resp.StatusCode, code)
	}

	// Omar has a valid bearer token, no orders.read permission -> 403 on orders, 200 on /me (x-auth: session).
	omarToken := w.signInAPI("omar@toyshop.example")

	req, _ := http.NewRequest("GET", w.srv.URL+apiroutes.GetOrder(1).URL(), nil)
	req.Header.Set("Authorization", "Bearer "+omarToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	code, _ = decodeErr(t, resp)
	if resp.StatusCode != http.StatusForbidden || code != "errors.auth.forbidden" {
		t.Errorf("omar getOrder = (%d, %q)", resp.StatusCode, code)
	}

	req, _ = http.NewRequest("GET", w.srv.URL+apiroutes.GetMe().URL(), nil)
	req.Header.Set("Authorization", "Bearer "+omarToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	meBody := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(meBody, `"id":"u2"`) {
		t.Errorf("omar /me = (%d, %q)", resp.StatusCode, meBody)
	}

	// Sara holds orders.read -> 200 with decimal money.
	saraToken := w.signInAPI("sara@toyshop.example")
	req, _ = http.NewRequest("GET", w.srv.URL+apiroutes.GetOrder(1).URL(), nil)
	req.Header.Set("Authorization", "Bearer "+saraToken)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	orderBody := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(orderBody, `"total":"24.5"`) {
		t.Errorf("sara getOrder = (%d, %q), want decimal total 24.5", resp.StatusCode, orderBody)
	}

	// Invalid request body failing schema -> 400 with no value echoed.
	badBody := strings.NewReader(`{"email":"a@b.example","password":"shortpw"}`)
	resp, err = client.Post(w.srv.URL+apiroutes.SignIn().URL(), "application/json", badBody)
	if err != nil {
		t.Fatal(err)
	}
	code, msg := decodeErr(t, resp)
	if resp.StatusCode != http.StatusBadRequest || code != "errors.handlers.invalid_field" ||
		!strings.Contains(msg, "/password") || strings.Contains(msg, "shortpw") {
		t.Errorf("bad body = (%d, %q, %q)", resp.StatusCode, code, msg)
	}
}

func TestSearch(t *testing.T) {
	w := newTestWorld(t)

	if err := w.tr.Set("fr", "route.products.list", "Produits"); err != nil {
		t.Fatal(err)
	}
	if err := w.tr.Set("fr", "route.orders.list", "Commandes"); err != nil {
		t.Fatal(err)
	}

	// Omar can open products.list and dashboard, but not orders.list.
	omar := w.browser()
	w.signInAdmin(omar, "omar@toyshop.example")

	req, _ := http.NewRequest("GET", w.srv.URL+adminroutes.Search().URL()+"?q=produits", nil)
	req.Header.Set("Accept-Language", "fr")
	resp, err := omar.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Produits") {
		t.Errorf("omar search 'produits' = (%d, %q)", resp.StatusCode, body)
	}

	req, _ = http.NewRequest("GET", w.srv.URL+adminroutes.Search().URL()+"?q=", nil)
	req.Header.Set("Accept-Language", "fr")
	resp, err = omar.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `data-key="products.list"`) || strings.Contains(body, `data-key="orders.list"`) {
		t.Errorf("omar empty search should show products and hide orders: %q", body)
	}

	// Sara holds orders.manage -> sees orders.list in French search.
	sara := w.browser()
	w.signInAdmin(sara, "sara@toyshop.example")

	req, _ = http.NewRequest("GET", w.srv.URL+adminroutes.Search().URL()+"?q=commandes", nil)
	req.Header.Set("Accept-Language", "fr")
	resp, err = sara.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body = readBody(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `data-key="orders.list"`) || !strings.Contains(body, "Commandes") {
		t.Errorf("sara search 'commandes' = (%d, %q)", resp.StatusCode, body)
	}
}

func TestModelsAndSettingsLabels(t *testing.T) {
	tr := i18ntest.New(t, sourceLocale)
	ctxEN := i18ntest.Context(t, tr, "en")

	if got := models.OrderStatusPaid.Label().T(ctxEN); got != "paid" {
		t.Errorf("OrderStatusPaid label = %q, want paid", got)
	}
	if got := models.L10nProduct.T(ctxEN); got != "Product" {
		t.Errorf("L10nProduct = %q, want Product", got)
	}
	if got := settings.L10nServerAddr.T(ctxEN); got != "Listen address" {
		t.Errorf("L10nServerAddr = %q, want Listen address", got)
	}
}
