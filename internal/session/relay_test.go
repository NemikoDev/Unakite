package session

import (
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
)

func startRelay(t *testing.T) peer.AddrInfo {
	t.Helper()
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.ForceReachabilityPublic(),
		libp2p.EnableRelayService(relayv2.WithInfiniteLimits()),
	)
	if err != nil {
		t.Fatalf("relay: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return peer.AddrInfo{ID: h.ID(), Addrs: h.Addrs()}
}

func TestSession_ConversationThroughRelay(t *testing.T) {
	relay := startRelay(t)
	sc := newSignaling(t)
	atAlice, atBob := newRecorder(), newRecorder()
	alice, bob := New(sc, atAlice.emit), New(sc, atBob.emit)
	for _, s := range []*Session{alice, bob} {
		if err := s.UseRelay(relay); err != nil {
			t.Fatalf("UseRelay: %v", err)
		}
	}
	t.Cleanup(func() { alice.Leave(); bob.Leave() })

	code, err := alice.CreateRoom(testCtx(t), "Alice")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := bob.JoinRoom(testCtx(t), "Bob", code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	expect[PeerInfo](t, atAlice, EventPeerJoined)
	expect[PeerInfo](t, atBob, EventPeerJoined)

	if _, err := bob.SendMessage(testCtx(t), "through the relay"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got := expect[Message](t, atAlice, EventMessage); got.Content != "through the relay" {
		t.Fatalf("Alice received %+v", got)
	}
}
