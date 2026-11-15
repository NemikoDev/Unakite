package p2p

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Unakite/internal/signaling"
)

func newSignalingClient(t *testing.T) *signaling.Client {
	t.Helper()
	ts := httptest.NewServer(signaling.NewServer(signaling.NewStore(time.Minute)).Handler())
	t.Cleanup(ts.Close)
	return signaling.NewClient(ts.URL)
}

func roomCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestRoom_JoinConnectsToHost(t *testing.T) {
	sc := newSignalingClient(t)
	a := newTestHost(t) // creates the room
	b := newTestHost(t) // joins with the code

	code, err := CreateRoom(roomCtx(t), sc, a)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	t.Logf("room code: %s", code)

	hostID, err := JoinRoom(roomCtx(t), sc, b, code)
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if hostID != a.ID() {
		t.Fatalf("joined %s, want host %s", hostID, a.ID())
	}
	if len(b.Network().ConnsToPeer(a.ID())) == 0 {
		t.Fatal("B has no connection to A after JoinRoom")
	}
}

func TestRoom_CodeWorksOnlyOnce(t *testing.T) {
	sc := newSignalingClient(t)
	a, b, c := newTestHost(t), newTestHost(t), newTestHost(t)

	code, err := CreateRoom(roomCtx(t), sc, a)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := JoinRoom(roomCtx(t), sc, b, code); err != nil {
		t.Fatalf("first join: %v", err)
	}

	_, err = JoinRoom(roomCtx(t), sc, c, code)
	if !errors.Is(err, signaling.ErrNotFound) {
		t.Fatalf("second join: got %v, want signaling.ErrNotFound", err)
	}
	if len(c.Network().Peers()) != 0 {
		t.Fatal("third peer must not have connected to anyone")
	}
}

func TestRoom_UnknownCode(t *testing.T) {
	sc := newSignalingClient(t)
	b := newTestHost(t)

	_, err := JoinRoom(roomCtx(t), sc, b, "ZZZZZZZZ")
	if !errors.Is(err, signaling.ErrNotFound) {
		t.Fatalf("got %v, want signaling.ErrNotFound", err)
	}
}

// A misbehaving server that mixes two different peers' addresses under one code.
func TestRoom_RejectsAddressesForDifferentPeers(t *testing.T) {
	sc := newSignalingClient(t)
	a, b, joiner := newTestHost(t), newTestHost(t), newTestHost(t)

	code, err := sc.CreateRoom(roomCtx(t), []string{loopbackAddr(t, a), loopbackAddr(t, b)})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	_, err = JoinRoom(roomCtx(t), sc, joiner, code)
	if err == nil || !strings.Contains(err.Error(), "exactly one peer") {
		t.Fatalf("got %v, want an 'exactly one peer' error", err)
	}
	if len(joiner.Network().Peers()) != 0 {
		t.Fatal("joiner must not connect to either peer")
	}
}
