package settings

import (
	"testing"

	"github.com/epistemer/auth"
	"github.com/epistemer/cfgx/cfgxtest"
	"github.com/epistemer/i18n"
)

// Verify at compile time that Serve satisfies auth.Config and i18n.Config.
var (
	_ auth.Config = (*Serve)(nil)
	_ i18n.Config = (*Serve)(nil)
)

func TestGenerated(t *testing.T) {
	cfgxtest.AssertGenerated(t)
}
