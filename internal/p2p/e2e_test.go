package p2p

import (
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"

	"Unakite/internal/signaling"
)

func TestEndToEnd_CodeInEncryptedChatOut(t *testing.T) {
	sc := newSignalingClient(t)

	// A hosts the room, so A gets the gate. B joins. C is the intruder.
	gate := NewRoomGate()
	a := newTestHost(t, libp2p.ConnectionGater(gate))
	b := newTestHost(t)
	c := newTestHost(t)

	// Each side's chat handler pushes into a channel the test can wait on.
	atA := make(chan delivered, 10)
	atB := make(chan delivered, 10)
	chatA := NewChatService(a, func(from peer.ID, m ChatMessage) { atA <- delivered{from: from, msg: m} })
	chatB := NewChatService(b, func(from peer.ID, m ChatMessage) { atB <- delivered{from: from, msg: m} })
	chatC := NewChatService(c, nil)
	t.Cleanup(func() { chatA.Close(); chatB.Close(); chatC.Close() })

	// 1. A creates a room; B joins with only the code.
	code, err := CreateRoom(roomCtx(t), sc, a)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	hostID, err := JoinRoom(roomCtx(t), sc, b, code)
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if hostID != a.ID() {
		t.Fatalf("B joined %s, want %s", hostID, a.ID())
	}

	// 2. B -> A. A learns B's identity from the verified connection, not from the message.
	if err := chatB.Send(roomCtx(t), a.ID(), ChatMessage{Nickname: "Bob", Content: "hi host"}); err != nil {
		t.Fatalf("B send: %v", err)
	}
	gotA := waitFor(t, atA)
	if gotA.msg.Content != "hi host" || gotA.from != b.ID() {
		t.Fatalf("A got %+v from %s, want 'hi host' from %s", gotA.msg, gotA.from, b.ID())
	}

	// 3. A -> B, replying to whoever the message came from.
	if err := chatA.Send(roomCtx(t), gotA.from, ChatMessage{Nickname: "Alice", Content: "hi guest"}); err != nil {
		t.Fatalf("A reply: %v", err)
	}
	gotB := waitFor(t, atB)
	if gotB.msg.Content != "hi guest" || gotB.from != a.ID() {
		t.Fatalf("B got %+v from %s, want 'hi guest' from %s", gotB.msg, gotB.from, a.ID())
	}
	if gate.Guest() != b.ID() {
		t.Fatalf("gate guest is %s, want %s", gate.Guest(), b.ID())
	}

	// 4a. Intruder via the signaling server: the code is already burned.
	if _, err := JoinRoom(roomCtx(t), sc, c, code); !errors.Is(err, signaling.ErrNotFound) {
		t.Fatalf("C via code: got %v, want ErrNotFound", err)
	}

	// 4b. Intruder who learned A's address some other way and dials directly.
	// We don't assert that Connect itself fails, only that the end result is
	// "no connection and no delivered message", which is what matters.
	_, _ = Connect(roomCtx(t), c, loopbackAddr(t, a))
	_ = chatC.Send(roomCtx(t), a.ID(), ChatMessage{Nickname: "Mallory", Content: "let me in"})

	time.Sleep(300 * time.Millisecond) // give a wrongly-accepted connection time to show up
	if n := len(a.Network().ConnsToPeer(c.ID())); n != 0 {
		t.Fatalf("A has %d connection(s) to the intruder, want 0", n)
	}
	select {
	case d := <-atA:
		t.Fatalf("intruder's message was delivered to A: %+v", d.msg)
	default:
	}

	// 5. The legitimate conversation still works afterwards.
	if err := chatB.Send(roomCtx(t), a.ID(), ChatMessage{Nickname: "Bob", Content: "still here"}); err != nil {
		t.Fatalf("B second send: %v", err)
	}
	if got := waitFor(t, atA); got.msg.Content != "still here" {
		t.Fatalf("A got %q, want 'still here'", got.msg.Content)
	}
}
