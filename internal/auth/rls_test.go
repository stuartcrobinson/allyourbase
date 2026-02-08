package auth

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestSetRLSContextNilClaims(t *testing.T) {
	// Nil claims should be a no-op.
	err := SetRLSContext(nil, nil, nil)
	testutil.NoError(t, err)
}
