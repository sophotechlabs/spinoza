package auth

import "testing"

func TestAnEndpointEqualToTheInternalIssuerUsesThePublicIssuer(t *testing.T) {
	const internal = "https://keycloak.internal/realms/main"
	const public = "https://login.example.com/realms/main"

	if got := swapBase(internal, internal, public); got != public {
		t.Fatalf("endpoint = %q, want %q", got, public)
	}
}
