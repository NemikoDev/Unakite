package p2p

import (
	"Unakite/internal/identity"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
)

func newTestHost(t *testing.T) host.Host {
	t.Helper()
	priv, _, err := identity.GenerateEphemeralIdentity()
	if err != nil {
		t.Fatalf("failed to generate ephemeral identity: %v", err)
	}
	h, err := NewHost(priv)
	if err != nil {
		t.Fatalf("failed to create test host: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func loopbackAddr(t *testing.T, h host.Host) string {
	t.Helper()
	addrs, err := FullAddrs(h)
	if err != nil {
		t.Fatalf("failed to get full addresses: %v", err)
	}
	for _, a := range addrs {
		if strings.HasPrefix(a, "/ip4/127.0.0.1/") {
			return a
		}
	}
	t.Fatalf("failed to find loopback address")
	return ""
}

func TestConnect_TwoPeersEstablishEncryptedConnection(t *testing.T) {
	a := newTestHost(t)
	b := newTestHost(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bAddr := loopbackAddr(t, b)
	t.Logf("A (%s) dialing B at %s", a.ID(), bAddr)

	connectedTo, err := Connect(ctx, a, bAddr)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if connectedTo != b.ID() {
		t.Fatalf("connected to %s, expected %s", connectedTo, b.ID())
	}

	conns := a.Network().ConnsToPeer(b.ID())
	if len(conns) == 0 {
		t.Fatal("A has no connection to B")
	}
	state := conns[0].ConnState()
	t.Logf("security=%s transport=%s muxer=%s", state.Security, state.Transport, state.StreamMultiplexer)
	if state.Security != noise.ID {
		t.Fatalf("expected security %s, got %s", noise.ID, state.Security)
	}

	deadline := time.Now().Add(2 * time.Second)
	for len(b.Network().ConnsToPeer(a.ID())) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("B never saw the inbound connection from A")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestConnect_WrongPeerIDIsRejected(t *testing.T) {
	a := newTestHost(t)
	b := newTestHost(t)
	impostor := newTestHost(t)

	real := loopbackAddr(t, b)
	base := real[:strings.Index(real, "/p2p/")]
	forged := base + "/p2p/" + impostor.ID().String()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := Connect(ctx, a, forged); err == nil {
		t.Fatal("expected connection to fail: remote identity doesn't match the expected PeerID")
	} else {
		t.Logf("correctly rejected: %v", err)
	}
}
