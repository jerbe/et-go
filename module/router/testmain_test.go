package router

import (
	"os"
	"testing"

	"github.com/jerbe/et-go/module/login"
)

func TestMain(m *testing.M) {
	login.ConfigureLegacyAccessTokenForTests()
	os.Exit(m.Run())
}
