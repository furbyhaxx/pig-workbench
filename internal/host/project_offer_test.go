package host

import (
	"path/filepath"
	"testing"
)

func TestProjectOfferIsClaimedOncePerRepository(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "offers")
	first, err := claimProjectOffer(dir, "/work/one")
	if err != nil || !first {
		t.Fatalf("first claim: %v %v", first, err)
	}
	if again, err := claimProjectOffer(dir, "/work/one"); err != nil || again {
		t.Fatalf("a repository was offered twice: %v %v", again, err)
	}
	if other, err := claimProjectOffer(dir, "/work/two"); err != nil || !other {
		t.Fatalf("a different repository must be offered: %v %v", other, err)
	}
}
