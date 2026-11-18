//go:build genx

// Keep packages whose genx.meta.yaml generators are activated in genx.yaml
// required in go.mod so go mod tidy preserves them and go list resolves them.
package main

import (
	_ "github.com/epistemer/handlers"
	_ "github.com/epistemer/i18n"
	_ "github.com/epistemer/models"
)
