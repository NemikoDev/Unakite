package p2p

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// delivered bundles what the handler receives so we can pass it through a channel.
type delivered struct {
	from peer.ID
	msg  ChatMessage
}

// setupChatPair starts two hosts, connects A to B, and returns a chat service
// for A plus a channel that receives everything B's handler gets.
func setupChatPair(t *testing.T) (a, b *ChatService, aID, bID peer.ID, received chan delivered) {
	t.Helper()
	hostA := newTestHost(t)
	hostB := newTestHost(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Connect(ctx, hostA, loopbackAddr(t, hostB)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Buffered channel: the handler goroutine can push up to 100 messages
	// without waiting for the test goroutine to read them.
	received = make(chan delivered, 100)
	b = NewChatService(hostB, func(from peer.ID, msg ChatMessage) {
		received <- delivered{from: from, msg: msg}
	})
	a = NewChatService(hostA, nil)
	t.Cleanup(func() { a.Close(); b.Close() })

	return a, b, hostA.ID(), hostB.ID(), received
}

// waitFor receives one item from the channel, or fails the test after a timeout.
func waitFor(t *testing.T, ch <-chan delivered) delivered {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for message")
		return delivered{}
	}
}

func TestChat_MessageDeliveredWithAuthenticatedSender(t *testing.T) {
	a, _, aID, bID, received := setupChatPair(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sent := ChatMessage{Nickname: "Aris", Content: "pan paka pan!", Timestamp: time.Now().UnixMilli()}
	if err := a.Send(ctx, bID, sent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := waitFor(t, received)
	if got.msg != sent {
		t.Fatalf("got %+v, want %+v", got.msg, sent)
	}
	if got.from != aID {
		t.Fatalf("sender peer ID is %s, want %s", got.from, aID)
	}
}

func TestChat_MessagesArriveInOrder(t *testing.T) {
	a, _, _, bID, received := setupChatPair(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const n = 50
	for i := 0; i < n; i++ {
		msg := ChatMessage{Nickname: "Alice", Content: fmt.Sprintf("message %d", i)}
		if err := a.Send(ctx, bID, msg); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}

	for i := 0; i < n; i++ {
		got := waitFor(t, received)
		want := fmt.Sprintf("message %d", i)
		if got.msg.Content != want {
			t.Fatalf("out of order: got %q, want %q", got.msg.Content, want)
		}
	}
}

func TestChat_UnknownProtocolIsRefused(t *testing.T) {
	hostA := newTestHost(t)
	hostB := newTestHost(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Connect(ctx, hostA, loopbackAddr(t, hostB)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// B has no handler for this protocol ID, so negotiation must fail.
	_, err := hostA.NewStream(ctx, hostB.ID(), "/some-other-protocol/1.0.0")
	if err == nil {
		t.Fatal("expected stream to be refused for an unregistered protocol")
	}
	t.Logf("correctly refused: %v", err)
}
