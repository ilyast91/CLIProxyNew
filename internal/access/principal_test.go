package access

import "testing"

func TestDecodePrincipalSupportsVersionedAndLegacyFormats(t *testing.T) {
	principal, err := DecodePrincipal(EncodePrincipal(42, 17))
	if err != nil || principal.UserID != 42 || principal.APIKeyID == nil || *principal.APIKeyID != 17 {
		t.Fatalf("DecodePrincipal(versioned) = %+v, %v", principal, err)
	}
	legacy, err := DecodePrincipal("42")
	if err != nil || legacy.UserID != 42 || legacy.APIKeyID != nil {
		t.Fatalf("DecodePrincipal(legacy) = %+v, %v", legacy, err)
	}
}
