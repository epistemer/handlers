// Package schema declares the toy shop's configuration using cfgx, combining
// auth's settings, i18n's runtime-mutable default locale, and the HTTP server's
// listener address and bbolt store file.
package schema

import (
	"github.com/epistemer/auth/authschema"
	"github.com/epistemer/cfgx/dsl"
	"github.com/epistemer/i18n/i18nschema"
)

// Auth is auth's configuration group.
var Auth = authschema.Group()

// I18n is i18n's configuration group (default_locale, mutable at runtime).
var I18n = i18nschema.Group()

// Store names the bbolt database file shared by cfgx, i18n, and auth.
var Store = dsl.Path("store", "Store file").Default("shop.db")

// Server is the HTTP listener and store path.
var Server = dsl.Group("server", "Web server",
	dsl.String("addr", "Listen address").Default("localhost:8080"),
	Store,
)

// App describes the shop application. Store enables mutable settings in bbolt;
// Labels emits zz_settings.labels.json for i18n's labels generator.
var App = dsl.App("shop", "A toy shop built on handlers", Auth, I18n, Server).
	Store(Store).
	Labels()

// Serve runs the HTTP server.
var Serve = dsl.Command("Serve").Uses(Auth, I18n, Server)
