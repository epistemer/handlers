// Command shop is the toy shop example for github.com/epistemer/handlers.
//
// Run it with:
//
//	SHOP_AUTH_ISSUER=toyshop SHOP_AUTH_SECRET=$(openssl rand -hex 32) \
//	SHOP_AUTH_COOKIE_INSECURE=true \
//	SHOP_DEMO_PASSWORD='a long phrase of your own' SHOP_DEMO_MAIL=$(mktemp -d) \
//	go run . serve
//
// With SHOP_DEMO_PASSWORD set, sara@toyshop.example is seeded in access.Root.
// With SHOP_DEMO_MAIL set to a directory, emailed verification codes are
// written there, one file per message.
package main

//go:generate go tool genx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/epistemer/auth"
	"github.com/epistemer/auth/access"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/authguard"
	"github.com/epistemer/handlers/examples/shop/api"
	"github.com/epistemer/handlers/examples/shop/apiroutes"
	"github.com/epistemer/handlers/examples/shop/internal/admin"
	"github.com/epistemer/handlers/examples/shop/internal/adminroutes"
	"github.com/epistemer/handlers/examples/shop/internal/settings"
	"github.com/epistemer/handlers/examples/shop/internal/shop"
	"github.com/epistemer/handlers/openapi"
	"github.com/epistemer/i18n"
	"github.com/spf13/cobra"
)

var sourceLocale = i18n.Locale{Code: "en", NativeName: "English"}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := newRoot().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:          "shop",
		Short:        "Toy shop example application",
		SilenceUsage: true,
	}
	settings.BindRoot(root)
	root.AddCommand(
		settings.ServeCommand(&cobra.Command{
			Use:   "serve",
			Short: "Run the HTTP server",
		}, serve),
		settings.ConfigCommand(),
	)
	return root
}

func serve(ctx context.Context, cfg *settings.Serve) error {
	// cfgx opens the bbolt store named by server.store (needed for i18n's
	// mutable default_locale) and closes it when serve returns.
	db := cfg.Store().DB()

	tr := i18n.New(db, sourceLocale, cfg)
	if err := tr.Load(); err != nil {
		return err
	}

	ac, err := access.New(db)
	if err != nil {
		return err
	}

	dir := shop.NewDirectory()
	dir.Add("u1", "sara@toyshop.example", "Sara", "sara@toyshop.example")
	var out auth.Sender = shop.NewOutbox()
	if d := os.Getenv("SHOP_DEMO_MAIL"); d != "" {
		out = mailbox(d)
	}
	a, err := auth.New(db, cfg, dir, out, auth.WithReject(admin.ErrorPage))
	if err != nil {
		return err
	}

	if pw := os.Getenv("SHOP_DEMO_PASSWORD"); pw != "" {
		if err := a.SetPassword(ctx, "u1", pw); err != nil {
			return err
		}
		if err := ac.Join(ctx, "u1", access.Root); err != nil {
			return err
		}
	}

	handler, err := newHandler(a, ac, tr, shop.NewStore())
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	defer wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Go(func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			if err := a.Sweep(ctx); err != nil && ctx.Err() == nil {
				slog.Error("shop: sweep", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})

	srv := &http.Server{
		Addr:              cfg.ServerAddr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	stopped := make(chan error, 1)
	wg.Go(func() {
		<-ctx.Done()
		shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		stopped <- srv.Shutdown(shutdown)
	})

	slog.Info("shop: listening", "addr", cfg.ServerAddr())
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return <-stopped
}

// newHandler wires the admin HTML surface (/admin) and JSON API (/api/v1), each
// on its own ServeMux with its own auth.Load carrier, wrapped by i18n.
func newHandler(a *auth.Auth, ac *access.Access, tr *i18n.Translator, store *shop.Store) (http.Handler, error) {
	guard := authguard.New(a, ac)
	table := handlers.NewTable(adminroutes.Routes, apiroutes.Routes)

	adminMux := http.NewServeMux()
	adminH := &admin.H{
		Auth:  a,
		Store: store,
		Guard: guard,
		Table: table,
		Tr:    tr,
	}
	adminroutes.Register(adminMux, adminH, guard, handlers.WithErrors(admin.ErrorPage))

	spec, err := api.GetSpec()
	if err != nil {
		return nil, err
	}
	apiMux := http.NewServeMux()
	apiSrv := &api.Server{Auth: a, Store: store}
	apiroutes.Mount(apiMux, guard, spec, func(rt *openapi.Router) {
		api.HandlerWithOptions(
			api.NewStrictHandlerWithOptions(apiSrv, nil, api.StrictHTTPServerOptions{
				RequestErrorHandlerFunc:  rt.RequestError,
				ResponseErrorHandlerFunc: rt.WriteError,
			}),
			api.StdHTTPServerOptions{
				BaseURL:          apiroutes.Prefix,
				BaseRouter:       rt,
				ErrorHandlerFunc: rt.RequestError,
			},
		)
	})

	root := http.NewServeMux()
	root.Handle(adminroutes.Prefix+"/", a.Load(auth.Cookie())(adminMux))
	root.Handle(apiroutes.Prefix+"/", a.Load(auth.Bearer())(apiMux))
	return tr.Middleware()(root), nil
}

// mailbox delivers each message to its own file for local demo runs.
type mailbox string

func (d mailbox) Send(_ context.Context, m auth.Message) error {
	body := fmt.Sprintf("To: %s\nCode: %s\nToken: %s\n", m.To, m.Code, m.Token)
	name := filepath.Join(string(d), fmt.Sprintf("%d-%d.txt", time.Now().UnixNano(), m.Kind))
	return os.WriteFile(name, []byte(body), 0o600)
}
