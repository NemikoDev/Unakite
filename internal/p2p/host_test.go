package p2p

import (
	"Unakite/internal/identity"
	"strings"
	"testing"
)

func TestNewHost_UsesEphemeralIdentity(t *testing.T) {
	priv, id, err := identity.GenerateEphemeralIdentity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}

	h, err := NewHost(priv)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer h.Close()

	if h.ID() != id {
		t.Fatalf("host ID %s does not match generated ID %s", h.ID(), id)
	}
}

func TestNewHost_ListensOnRandomPort(t *testing.T) {
	priv1, _, _ := identity.GenerateEphemeralIdentity()
	priv2, _, _ := identity.GenerateEphemeralIdentity()

	h1, err := NewHost(priv1)
	if err != nil {
		t.Fatalf("host 1: %v", err)
	}
	defer h1.Close()

	h2, err := NewHost(priv2)
	if err != nil {
		t.Fatalf("host 2: %v", err)
	}
	defer h2.Close()

	if len(h1.Addrs()) == 0 || len(h2.Addrs()) == 0 {
		t.Fatal("expected both hosts to have listen addresses")
	}
	if h1.Addrs()[0].String() == h2.Addrs()[0].String() {
		t.Fatal("expected different addresses/ports for two hosts")
	}
}

func TestFullAddrs_ContainsPeerID(t *testing.T) {
	priv, id, _ := identity.GenerateEphemeralIdentity()
	h, err := NewHost(priv)
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer h.Close()

	addrs, err := FullAddrs(h)
	if err != nil {
		t.Fatalf("FullAddrs: %v", err)
	}
	if len(addrs) == 0 {
		t.Fatal("expected at least one address")
	}
	for _, a := range addrs {
		t.Logf("listening on: %s", a)
		if !strings.HasSuffix(a, "/p2p/"+id.String()) {
			t.Fatalf("addr %s does not end with peer ID %s", a, id)
		}
	}
}
