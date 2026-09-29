package identity

import "testing"

func TestGenerateEphemeralIdentity(t *testing.T) {
	_, id, err := GenerateEphemeralIdentity()
	if err != nil {
		t.Fatalf("errored: %v", err)
	}
	if id.String() == "" {
		t.Fatal("expected non-empty peer id")
	}

}

func TestGenerateEphemeralIdentity_IsEphemeral(t *testing.T) {
	_, id1, err := GenerateEphemeralIdentity()
	if err != nil {
		t.Fatalf("first generation failed: %v", err)
	}

	_, id2, err := GenerateEphemeralIdentity()
	if err != nil {
		t.Fatalf("second generation failed: %v", err)
	}

	if id1 == id2 {
		t.Fatal("expected two separate calls to produce different peer IDs, got identical IDs")
	}
}

func TestGenerateEphemeralIdentity_KeyTypeIsEd25519(t *testing.T) {
	priv, _, err := GenerateEphemeralIdentity()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if priv.Type().String() != "Ed25519" {
		t.Fatalf("expected Ed25519 key type, got: %s", priv.Type().String())
	}
}
