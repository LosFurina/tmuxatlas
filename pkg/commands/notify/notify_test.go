package notify

import (
	"path/filepath"
	"testing"
)

func TestPostViaSocketFailsWhenLocalServiceIsUnavailable(t *testing.T) {
	_, err := postViaSocket(filepath.Join(t.TempDir(), "missing.sock"), []byte(`{}`))
	if err == nil {
		t.Fatal("postViaSocket unexpectedly succeeded without a local service")
	}
}
