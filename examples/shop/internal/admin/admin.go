// Package admin implements the toy shop's HTML admin surface under /admin.
// Route metadata lives in //h: doc comments checked by `handlers vet` and
// turned into internal/adminroutes by `handlers gen` + `genx`.
package admin

import (
	"html/template"
	"log/slog"
	"net/http"
	"slices"

	"github.com/epistemer/auth"
	"github.com/epistemer/errors"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/examples/shop/internal/adminroutes"
	"github.com/epistemer/handlers/examples/shop/internal/ids"
	"github.com/epistemer/handlers/examples/shop/internal/models"
	"github.com/epistemer/handlers/examples/shop/internal/shop"
	"github.com/epistemer/i18n"
)

// H implements adminroutes.Handler.
type H struct {
	Auth  *auth.Auth
	Store *shop.Store
	Guard handlers.Guard
	Table *handlers.Table
	Tr    *i18n.Translator
}

// ErrorPage renders handler and guard errors as HTML using errors.T in the
// request's locale and handlers.Status for the HTTP status code.
func ErrorPage(w http.ResponseWriter, r *http.Request, err error) {
	status := handlers.Status(err)
	if status >= http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "admin", "status", status, "err", err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = errorTmpl.Execute(w, struct {
		Status  int
		Title   string
		Message string
		HomeURL string
	}{
		Status:  status,
		Title:   http.StatusText(status),
		Message: errors.T(r.Context(), err),
		HomeURL: adminroutes.Dashboard().URL(),
	})
}

//h:key sign_in
//h:method GET
//h:path /sign-in
//h:desc Sign in
//h:auth public
//h:view
func (h *H) SignInPage(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	return renderHTML(w, signInTmpl, struct {
		Title  string
		Action string
	}{
		Title:  adminroutes.L10nSignInPage.T(ctx),
		Action: adminroutes.SignInSubmit().URL(),
	})
}

//h:key sign_in.submit
//h:method POST
//h:path /sign-in
//h:desc Submit sign-in
//h:auth public
func (h *H) SignInSubmit(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	f, err := h.Auth.SignIn(ctx, r.FormValue("login"), auth.Cookie())
	if err == nil {
		f, err = h.Auth.Prove(ctx, f.Token, auth.Password(r.FormValue("password")))
	}
	if err != nil {
		return err
	}
	if !f.Verified && slices.Contains(f.Offers, auth.MethodEmailCode) {
		ch, err := h.Auth.Challenge(ctx, f.Token, auth.MethodEmailCode)
		if err != nil {
			return err
		}
		f.Write(w)
		return renderHTML(w, codeTmpl, struct {
			Title  string
			Action string
			SentTo string
		}{
			Title:  adminroutes.L10nSignInCode.T(ctx),
			Action: adminroutes.SignInCode().URL(),
			SentTo: ch.SentTo,
		})
	}
	res, err := h.Auth.Finish(ctx, f.Token)
	if err != nil {
		return err
	}
	res.Write(w)
	http.Redirect(w, r, adminroutes.Dashboard().URL(), http.StatusSeeOther)
	return nil
}

//h:key sign_in.code
//h:method POST
//h:path /sign-in/code
//h:desc Verify sign-in code
//h:auth public
func (h *H) SignInCode(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	tok, ok := auth.FlowToken(ctx)
	if !ok {
		return auth.ErrFlowExpired
	}
	f, err := h.Auth.Prove(ctx, tok, auth.EmailCode(r.FormValue("code")))
	if err != nil {
		return err
	}
	res, err := h.Auth.Finish(ctx, f.Token)
	if err != nil {
		return err
	}
	res.Write(w)
	http.Redirect(w, r, adminroutes.Dashboard().URL(), http.StatusSeeOther)
	return nil
}

//h:key dashboard
//h:method GET
//h:path /
//h:desc Dashboard
//h:auth session
//h:view
//h:search
func (h *H) Dashboard(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	return renderHTML(w, dashboardTmpl, struct {
		Title       string
		ProductsLbl string
		ProductsURL string
		OrdersLbl   string
		OrdersURL   string
		SearchLbl   string
		SearchURL   string
	}{
		Title:       adminroutes.L10nDashboard.T(ctx),
		ProductsLbl: adminroutes.L10nProducts.T(ctx),
		ProductsURL: adminroutes.Products().URL(),
		OrdersLbl:   adminroutes.L10nOrders.T(ctx),
		OrdersURL:   adminroutes.Orders().URL(),
		SearchLbl:   adminroutes.L10nSearch.T(ctx),
		SearchURL:   adminroutes.Search().URL(),
	})
}

//h:key products.list
//h:method GET
//h:path /products
//h:desc Products
//h:auth session
//h:view
//h:search
func (h *H) Products(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	type row struct {
		ID    ids.ProductID
		SKU   string
		Name  string
		Price string
		URL   string
	}
	items := h.Store.Products()
	rows := make([]row, len(items))
	for i, p := range items {
		rows[i] = row{
			ID:    p.ID,
			SKU:   p.SKU,
			Name:  p.Name,
			Price: p.Price.StringFixed(2),
			URL:   adminroutes.Product(p.ID).URL(),
		}
	}
	return renderHTML(w, productsTmpl, struct {
		Title      string
		ColID      string
		ColSKU     string
		ColName    string
		ColPrice   string
		Rows       []row
		DashURL    string
		DashLabel  string
		SearchURL  string
		SearchText string
	}{
		Title:      adminroutes.L10nProducts.T(ctx),
		ColID:      models.L10nProduct_Id.T(ctx),
		ColSKU:     models.L10nProduct_Sku.T(ctx),
		ColName:    models.L10nProduct_Name.T(ctx),
		ColPrice:   models.L10nProduct_Price.T(ctx),
		Rows:       rows,
		DashURL:    adminroutes.Dashboard().URL(),
		DashLabel:  adminroutes.L10nDashboard.T(ctx),
		SearchURL:  adminroutes.Search().URL(),
		SearchText: adminroutes.L10nSearch.T(ctx),
	})
}

//h:key products.profile
//h:method GET
//h:path /products/{id}
//h:desc Product profile
//h:auth session
//h:view
//h:entity products.list
//h:param id label Product ID
func (h *H) Product(w http.ResponseWriter, r *http.Request, id ids.ProductID) error {
	p, err := h.Store.Product(id)
	if err != nil {
		return err
	}
	ctx := r.Context()
	return renderHTML(w, productTmpl, struct {
		Title      string
		Entity     string
		IDLabel    string
		ID         ids.ProductID
		SKULabel   string
		SKU        string
		NameLabel  string
		Name       string
		PriceLabel string
		Price      string
		BackURL    string
		BackLabel  string
	}{
		Title:      adminroutes.L10nProduct.T(ctx),
		Entity:     models.L10nProduct.T(ctx),
		IDLabel:    models.L10nProduct_Id.T(ctx),
		ID:         p.ID,
		SKULabel:   models.L10nProduct_Sku.T(ctx),
		SKU:        p.SKU,
		NameLabel:  models.L10nProduct_Name.T(ctx),
		Name:       p.Name,
		PriceLabel: models.L10nProduct_Price.T(ctx),
		Price:      p.Price.StringFixed(2),
		BackURL:    adminroutes.Products().URL(),
		BackLabel:  adminroutes.L10nProducts.T(ctx),
	})
}

//h:key orders.list
//h:method GET
//h:path /orders
//h:desc Orders
//h:perm orders.read from apiroutes
//h:view
//h:search
func (h *H) Orders(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	type row struct {
		ID     ids.OrderID
		Status string
		Total  string
		URL    string
	}
	items := h.Store.Orders(nil)
	rows := make([]row, len(items))
	for i, o := range items {
		rows[i] = row{
			ID:     o.ID,
			Status: o.Status.Label().T(ctx),
			Total:  o.Total.StringFixed(2),
			URL:    adminroutes.Order(o.ID).URL(),
		}
	}
	return renderHTML(w, ordersTmpl, struct {
		Title     string
		ColID     string
		ColStatus string
		ColTotal  string
		Rows      []row
		DashURL   string
		DashLabel string
	}{
		Title:     adminroutes.L10nOrders.T(ctx),
		ColID:     models.L10nOrder_Id.T(ctx),
		ColStatus: models.L10nOrder_Status.T(ctx),
		ColTotal:  models.L10nOrder_Total.T(ctx),
		Rows:      rows,
		DashURL:   adminroutes.Dashboard().URL(),
		DashLabel: adminroutes.L10nDashboard.T(ctx),
	})
}

//h:key orders.detail
//h:method GET
//h:path /orders/{id}
//h:desc Order detail
//h:perm orders.manage Manage orders
//h:view
//h:entity orders.list
//h:param id label Order ID
func (h *H) Order(w http.ResponseWriter, r *http.Request, id ids.OrderID) error {
	o, err := h.Store.Order(id)
	if err != nil {
		return err
	}
	ctx := r.Context()
	return renderHTML(w, orderTmpl, struct {
		Title       string
		Entity      string
		IDLabel     string
		ID          ids.OrderID
		StatusLabel string
		Status      string
		TotalLabel  string
		Total       string
		CancelURL   string
		CancelLabel string
		CanCancel   bool
		BackURL     string
		BackLabel   string
	}{
		Title:       adminroutes.L10nOrder.T(ctx),
		Entity:      models.L10nOrder.T(ctx),
		IDLabel:     models.L10nOrder_Id.T(ctx),
		ID:          o.ID,
		StatusLabel: models.L10nOrder_Status.T(ctx),
		Status:      o.Status.Label().T(ctx),
		TotalLabel:  models.L10nOrder_Total.T(ctx),
		Total:       o.Total.StringFixed(2),
		CancelURL:   adminroutes.OrderCancel(o.ID).URL(),
		CancelLabel: adminroutes.L10nOrderCancel.T(ctx),
		CanCancel:   o.Status != models.OrderStatusCancelled,
		BackURL:     adminroutes.Orders().URL(),
		BackLabel:   adminroutes.L10nOrders.T(ctx),
	})
}

//h:key orders.cancel
//h:method POST
//h:path /orders/{id}/cancel
//h:desc Cancel order
//h:perm orders.manage Manage orders
//h:body 20MiB
//h:param id label Order ID
func (h *H) OrderCancel(w http.ResponseWriter, r *http.Request, id ids.OrderID) error {
	if _, err := h.Store.CancelOrder(id); err != nil {
		return err
	}
	http.Redirect(w, r, adminroutes.Order(id).URL(), http.StatusSeeOther)
	return nil
}

//h:key search
//h:method GET
//h:path /search
//h:desc Search
//h:auth session
func (h *H) Search(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query().Get("q")
	hits, err := h.Table.Search(r, h.Guard, h.Tr, handlers.Query{
		Text: q,
		Only: func(rt handlers.Route) bool { return rt.Searchable },
	})
	if err != nil {
		return err
	}
	type item struct {
		Key  string
		Text string
		URL  string
	}
	out := make([]item, len(hits))
	for i, hit := range hits {
		out[i] = item{
			Key:  hit.Route.Key,
			Text: hit.Text,
			URL:  searchableURL(hit.Route),
		}
	}
	ctx := r.Context()
	return renderHTML(w, searchTmpl, struct {
		Title     string
		Query     string
		Hits      []item
		Action    string
		DashURL   string
		DashLabel string
	}{
		Title:     adminroutes.L10nSearch.T(ctx),
		Query:     q,
		Hits:      out,
		Action:    adminroutes.Search().URL(),
		DashURL:   adminroutes.Dashboard().URL(),
		DashLabel: adminroutes.L10nDashboard.T(ctx),
	})
}

func searchableURL(rt handlers.Route) string {
	switch rt.Key {
	case "dashboard":
		return adminroutes.Dashboard().URL()
	case "products.list":
		return adminroutes.Products().URL()
	case "orders.list":
		return adminroutes.Orders().URL()
	default:
		return rt.Prefix + rt.Pattern
	}
}

func renderHTML(w http.ResponseWriter, tmpl *template.Template, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.Execute(w, data)
}

var (
	errorTmpl = template.Must(template.New("error").Parse(
		`<!doctype html><html><head><title>{{.Status}} {{.Title}}</title></head>` +
			`<body><h1>{{.Status}} {{.Title}}</h1><p>{{.Message}}</p>` +
			`<p><a href="{{.HomeURL}}">Dashboard</a></p></body></html>`,
	))
	signInTmpl = template.Must(template.New("signin").Parse(
		`<!doctype html><html><head><title>{{.Title}}</title></head><body><h1>{{.Title}}</h1>` +
			`<form method="post" action="{{.Action}}">` +
			`<label>Login <input name="login" required></label>` +
			`<label>Password <input name="password" type="password" required></label>` +
			`<button type="submit">{{.Title}}</button></form></body></html>`,
	))
	codeTmpl = template.Must(template.New("code").Parse(
		`<!doctype html><html><head><title>{{.Title}}</title></head><body><h1>{{.Title}}</h1>` +
			`<p>Code sent to {{.SentTo}}</p>` +
			`<form method="post" action="{{.Action}}">` +
			`<label>Code <input name="code" required></label>` +
			`<button type="submit">{{.Title}}</button></form></body></html>`,
	))
	dashboardTmpl = template.Must(template.New("dashboard").Parse(
		`<!doctype html><html><head><title>{{.Title}}</title></head><body><h1>{{.Title}}</h1>` +
			`<form method="get" action="{{.SearchURL}}"><input name="q" placeholder="{{.SearchLbl}}"><button type="submit">{{.SearchLbl}}</button></form>` +
			`<ul><li><a href="{{.ProductsURL}}">{{.ProductsLbl}}</a></li>` +
			`<li><a href="{{.OrdersURL}}">{{.OrdersLbl}}</a></li></ul></body></html>`,
	))
	productsTmpl = template.Must(template.New("products").Parse(
		`<!doctype html><html><head><title>{{.Title}}</title></head><body><h1>{{.Title}}</h1>` +
			`<p><a href="{{.DashURL}}">{{.DashLabel}}</a> | <a href="{{.SearchURL}}">{{.SearchText}}</a></p>` +
			`<table><tr><th>{{.ColID}}</th><th>{{.ColSKU}}</th><th>{{.ColName}}</th><th>{{.ColPrice}}</th></tr>` +
			`{{range .Rows}}<tr><td><a href="{{.URL}}">{{.ID}}</a></td><td>{{.SKU}}</td><td>{{.Name}}</td><td>{{.Price}}</td></tr>{{end}}` +
			`</table></body></html>`,
	))
	productTmpl = template.Must(template.New("product").Parse(
		`<!doctype html><html><head><title>{{.Title}}</title></head><body><h1>{{.Title}}</h1>` +
			`<p>{{.Entity}}</p>` +
			`<dl><dt>{{.IDLabel}}</dt><dd>{{.ID}}</dd><dt>{{.SKULabel}}</dt><dd>{{.SKU}}</dd>` +
			`<dt>{{.NameLabel}}</dt><dd>{{.Name}}</dd><dt>{{.PriceLabel}}</dt><dd>{{.Price}}</dd></dl>` +
			`<p><a href="{{.BackURL}}">{{.BackLabel}}</a></p></body></html>`,
	))
	ordersTmpl = template.Must(template.New("orders").Parse(
		`<!doctype html><html><head><title>{{.Title}}</title></head><body><h1>{{.Title}}</h1>` +
			`<p><a href="{{.DashURL}}">{{.DashLabel}}</a></p>` +
			`<table><tr><th>{{.ColID}}</th><th>{{.ColStatus}}</th><th>{{.ColTotal}}</th></tr>` +
			`{{range .Rows}}<tr><td><a href="{{.URL}}">{{.ID}}</a></td><td>{{.Status}}</td><td>{{.Total}}</td></tr>{{end}}` +
			`</table></body></html>`,
	))
	orderTmpl = template.Must(template.New("order").Parse(
		`<!doctype html><html><head><title>{{.Title}}</title></head><body><h1>{{.Title}}</h1>` +
			`<p>{{.Entity}}</p>` +
			`<dl><dt>{{.IDLabel}}</dt><dd>{{.ID}}</dd><dt>{{.StatusLabel}}</dt><dd>{{.Status}}</dd>` +
			`<dt>{{.TotalLabel}}</dt><dd>{{.Total}}</dd></dl>` +
			`{{if .CanCancel}}<form method="post" action="{{.CancelURL}}"><button type="submit">{{.CancelLabel}}</button></form>{{end}}` +
			`<p><a href="{{.BackURL}}">{{.BackLabel}}</a></p></body></html>`,
	))
	searchTmpl = template.Must(template.New("search").Parse(
		`<!doctype html><html><head><title>{{.Title}}</title></head><body><h1>{{.Title}}</h1>` +
			`<form method="get" action="{{.Action}}"><input name="q" value="{{.Query}}"><button type="submit">{{.Title}}</button></form>` +
			`<ul>{{range .Hits}}<li data-key="{{.Key}}"><a href="{{.URL}}">{{.Text}}</a></li>{{end}}</ul>` +
			`<p><a href="{{.DashURL}}">{{.DashLabel}}</a></p></body></html>`,
	))
)
