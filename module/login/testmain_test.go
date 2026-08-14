package login

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	ConfigureLegacyAccessTokenForTests()
	os.Exit(m.Run())
}
