package p2p

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"

	"Unakite/internal/identity"
)

func startRelay(t *testing.T) peer.AddrInfo {
	t.Helper()
	h := newTestHost(t,
		libp2p.ForceReachabilityPublic(),
		libp2p.EnableRelayService(relayv2.WithInfiniteLimits()),
	)
	info, err := peer.AddrInfoFromString(loopbackAddr(t, h))
	if err != nil {
		t.Fatalf("relay addr: %v", err)
	}
	return *info
}

func TestRelay_RoomWorksThroughRelay(t *testing.T) {
	relay := startRelay(t)
	sc := newSignalingClient(t)

	gate := NewRoomGate(relay.ID)
	a := newTestHost(t, append(HostRelayOptions(relay), libp2p.ConnectionGater(gate))...)
	b := newTestHost(t, GuestRelayOptions()...)

	code, err := CreateRoomViaRelay(roomCtx(t), sc, a, relay)
	if err != nil {
		t.Fatalf("CreateRoomViaRelay: %v", err)
	}

	// What the signaling server hands out must be relay addresses only.
	addrs, err := sc.JoinRoom(roomCtx(t), code)
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	for _, ad := range addrs {
		if !strings.Contains(ad, "/p2p-circuit/p2p/"+a.ID().String()) {
			t.Fatalf("published a non-relay address: %s", ad)
		}
	}

	hostID, err := ConnectAddrs(roomCtx(t), b, addrs)
	if err != nil {
		t.Fatalf("ConnectAddrs: %v", err)
	}
	if hostID != a.ID() {
		t.Fatalf("connected to %s, want %s", hostID, a.ID())
	}

	atA := make(chan delivered, 10)
	chatA := NewChatService(a, func(from peer.ID, m ChatMessage) { atA <- delivered{from: from, msg: m} })
	chatB := NewChatService(b, nil)
	t.Cleanup(func() { chatA.Close(); chatB.Close() })

	if err := chatB.Send(roomCtx(t), a.ID(), ChatMessage{Nickname: "Bob", Content: "via relay"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	got := waitFor(t, atA)
	if got.msg.Content != "via relay" || got.from != b.ID() {
		t.Fatalf("got %+v from %s", got.msg, got.from)
	}
	if gate.Guest() != b.ID() {
		t.Fatalf("guest slot holds %s, want Bob (%s)", gate.Guest(), b.ID())
	}
}

func TestRelay_UnreachableRelayFailsClearly(t *testing.T) {
	_, relayID, err := identity.GenerateEphemeralIdentity()
	if err != nil {
		t.Fatal(err)
	}
	nowhere, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1")
	dead := peer.AddrInfo{ID: relayID, Addrs: []multiaddr.Multiaddr{nowhere}}

	a := newTestHost(t, HostRelayOptions(dead)...)
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	if _, err := CreateRoomViaRelay(ctx, newSignalingClient(t), a, dead); !errors.Is(err, ErrRelayUnavailable) {
		t.Fatalf("got %v, want ErrRelayUnavailable", err)
	}
}

func TestRoomGate_ExemptPeerDoesNotClaimTheSlot(t *testing.T) {
	_, relayID, _ := identity.GenerateEphemeralIdentity()
	_, guestID, _ := identity.GenerateEphemeralIdentity()
	_, strangerID, _ := identity.GenerateEphemeralIdentity()
	g := NewRoomGate(relayID)

	if !g.InterceptSecured(network.DirInbound, relayID, nil) {
		t.Fatal("the relay must be allowed to connect")
	}
	if g.Guest() != "" {
		t.Fatal("the relay must not take the guest slot")
	}
	if !g.InterceptSecured(network.DirInbound, guestID, nil) {
		t.Fatal("the first real peer must be admitted")
	}
	if g.InterceptSecured(network.DirInbound, strangerID, nil) {
		t.Fatal("a second inbound peer must be refused")
	}
}
