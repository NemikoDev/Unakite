package session

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Unakite/internal/signaling"
)

type event struct {
	name    string
	payload any
}

type recorder struct{ ch chan event }

func newRecorder() *recorder { return &recorder{ch: make(chan event, 50)} }

func (r *recorder) emit(name string, payload any) { r.ch <- event{name, payload} }

func expect[T any](t *testing.T, r *recorder, name string) T {
	t.Helper()
	var zero T
	select {
	case e := <-r.ch:
		if e.name != name {
			t.Fatalf("got event %q (%+v), want %q", e.name, e.payload, name)
		}
		v, ok := e.payload.(T)
		if !ok {
			t.Fatalf("event %q payload is %T, want %T", name, e.payload, zero)
		}
		return v
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for event %q", name)
	}
	return zero
}

func newSignaling(t *testing.T) *signaling.Client {
	t.Helper()
	ts := httptest.NewServer(signaling.NewServer(signaling.NewStore(time.Minute)).Handler())
	t.Cleanup(ts.Close)
	return signaling.NewClient(ts.URL)
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestSession_FullConversation(t *testing.T) {
	sc := newSignaling(t)
	atAlice, atBob := newRecorder(), newRecorder()
	alice, bob := New(sc, atAlice.emit), New(sc, atBob.emit)
	t.Cleanup(func() { alice.Leave(); bob.Leave() })

	code, err := alice.CreateRoom(testCtx(t), "Alice")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := bob.JoinRoom(testCtx(t), "Bob", code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Both sides learn the other's nickname through the hello exchange.
	joinedAtAlice := expect[PeerInfo](t, atAlice, EventPeerJoined)
	if joinedAtAlice.Nickname != "Bob" {
		t.Fatalf("Alice sees peer %q, want Bob", joinedAtAlice.Nickname)
	}
	joinedAtBob := expect[PeerInfo](t, atBob, EventPeerJoined)
	if joinedAtBob.Nickname != "Alice" {
		t.Fatalf("Bob sees peer %q, want Alice", joinedAtBob.Nickname)
	}

	// Bob -> Alice
	sent, err := bob.SendMessage(testCtx(t), "  hi Alice  ")
	if err != nil {
		t.Fatalf("Bob send: %v", err)
	}
	if !sent.IsOwn || sent.Content != "hi Alice" {
		t.Fatalf("local echo wrong: %+v", sent)
	}
	got := expect[Message](t, atAlice, EventMessage)
	if got.IsOwn || got.Content != "hi Alice" || got.Sender.Nickname != "Bob" || got.Sender.ID != joinedAtAlice.ID {
		t.Fatalf("Alice received %+v", got)
	}

	// Alice -> Bob
	if _, err := alice.SendMessage(testCtx(t), "hi Bob"); err != nil {
		t.Fatalf("Alice send: %v", err)
	}
	got = expect[Message](t, atBob, EventMessage)
	if got.Content != "hi Bob" || got.Sender.Nickname != "Alice" {
		t.Fatalf("Bob received %+v", got)
	}

	carol := New(sc, nil)
	t.Cleanup(carol.Leave)
	if err := carol.JoinRoom(testCtx(t), "Carol", code); !errors.Is(err, signaling.ErrNotFound) {
		t.Fatalf("Carol: got %v, want ErrNotFound", err)
	}

	bob.Leave()
	left := expect[PeerLeft](t, atAlice, EventPeerLeft)
	if left.Nickname != "Bob" {
		t.Fatalf("peer-left for %q, want Bob", left.Nickname)
	}
	if _, err := alice.SendMessage(testCtx(t), "hello?"); !errors.Is(err, ErrNotPeer) {
		t.Fatalf("send after peer left: got %v, want ErrNotPeer", err)
	}
}

func TestSession_ErrorsAndRecovery(t *testing.T) {
	s := New(newSignaling(t), nil)
	t.Cleanup(s.Leave)

	if _, err := s.SendMessage(testCtx(t), "hi"); !errors.Is(err, ErrNotInRoom) {
		t.Fatalf("send outside a room: got %v, want ErrNotInRoom", err)
	}
	if err := s.JoinRoom(testCtx(t), "   ", "ABCDEFGH"); !errors.Is(err, ErrInvalidNickname) {
		t.Fatalf("blank nickname: got %v, want ErrInvalidNickname", err)
	}
	if err := s.JoinRoom(testCtx(t), "Bob", "ZZZZZZZZ"); !errors.Is(err, signaling.ErrNotFound) {
		t.Fatalf("unknown code: got %v, want ErrNotFound", err)
	}

	// The failed join must not leave the session stuck.
	if _, err := s.CreateRoom(testCtx(t), "Alice"); err != nil {
		t.Fatalf("CreateRoom after failed join: %v", err)
	}
	if _, err := s.SendMessage(testCtx(t), "anyone?"); !errors.Is(err, ErrNotPeer) {
		t.Fatalf("send before anyone joined: got %v, want ErrNotPeer", err)
	}
	if _, err := s.SendMessage(testCtx(t), "   "); !errors.Is(err, ErrEmptyMessage) {
		t.Fatalf("blank message: got %v, want ErrEmptyMessage", err)
	}
	if _, err := s.CreateRoom(testCtx(t), "Alice"); !errors.Is(err, ErrAlreadyInRoom) {
		t.Fatalf("second room: got %v, want ErrAlreadyInRoom", err)
	}

	s.Leave()
	if _, err := s.CreateRoom(testCtx(t), "Alice"); err != nil {
		t.Fatalf("CreateRoom after Leave: %v", err)
	}
}

func TestValidateNickname(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"Bob", "Bob", false},
		{"  Bob ", "Bob", false},
		{"Zoë", "Zoë", false},
		{"a\nb", "ab", false},
		{strings.Repeat("a", 32), strings.Repeat("a", 32), false},
		{"", "", true},
		{"   ", "", true},
		{strings.Repeat("a", 33), "", true},
	}
	for _, tc := range cases {
		got, err := validateNickname(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("validateNickname(%q) = %q, %v; want %q, err=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestSanitizePeerNickname(t *testing.T) {
	if got := sanitizePeerNickname("\x00\x01"); got != "Anonymous" {
		t.Errorf("empty-after-cleaning: got %q, want Anonymous", got)
	}
	if got := sanitizePeerNickname(strings.Repeat("x", 100)); len([]rune(got)) != maxNickname {
		t.Errorf("long nickname not truncated: %d runes", len([]rune(got)))
	}
}

func TestSession_RoomExpiresIfNobodyJoins(t *testing.T) {
	rec := newRecorder()
	s := New(newSignaling(t), rec.emit)
	s.waitForGuest = 50 * time.Millisecond
	t.Cleanup(s.Leave)

	if _, err := s.CreateRoom(testCtx(t), "Alice"); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	closed := expect[RoomClosed](t, rec, EventRoomClosed)
	if closed.Reason != "expired" {
		t.Fatalf("reason %q, want expired", closed.Reason)
	}
	if s.State().InRoom {
		t.Fatal("session should be empty after expiry")
	}
	if _, err := s.CreateRoom(testCtx(t), "Alice"); err != nil {
		t.Fatalf("CreateRoom after expiry: %v", err)
	}
}

func TestSession_RoomDoesNotExpireOnceGuestJoined(t *testing.T) {
	sc := newSignaling(t)
	atAlice := newRecorder()
	alice, bob := New(sc, atAlice.emit), New(sc, nil)
	alice.waitForGuest = 400 * time.Millisecond
	t.Cleanup(func() { alice.Leave(); bob.Leave() })

	code, err := alice.CreateRoom(testCtx(t), "Alice")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := bob.JoinRoom(testCtx(t), "Bob", code); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	expect[PeerInfo](t, atAlice, EventPeerJoined)

	time.Sleep(700 * time.Millisecond)

	st := alice.State()
	if !st.InRoom || st.Code != code || st.Peer == nil || st.Peer.Nickname != "Bob" {
		t.Fatalf("unexpected state after the wait: %+v", st)
	}
	select {
	case e := <-atAlice.ch:
		t.Fatalf("unexpected event after join: %s", e.name)
	default:
	}
}

// Leaving

func connectedPair(t *testing.T, grace time.Duration) (alice, bob *Session, atAlice, atBob *recorder) {
	t.Helper()
	sc := newSignaling(t)
	atAlice, atBob = newRecorder(), newRecorder()
	alice, bob = New(sc, atAlice.emit), New(sc, atBob.emit)
	alice.gracePeriod, bob.gracePeriod = grace, grace
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
	return alice, bob, atAlice, atBob
}

func TestSession_PeerLeaves_RoomClosesAfterGrace(t *testing.T) {
	alice, bob, atAlice, _ := connectedPair(t, 150*time.Millisecond)

	before := time.Now().UnixMilli()
	bob.Leave()

	left := expect[PeerLeft](t, atAlice, EventPeerLeft)
	if left.Nickname != "Bob" || left.ClosesAt <= before {
		t.Fatalf("unexpected peer-left payload: %+v", left)
	}

	// During the grace period the room is still there, but read-only.
	st := alice.State()
	if !st.InRoom || st.Peer != nil || st.ClosesAt != left.ClosesAt {
		t.Fatalf("state during grace: %+v", st)
	}
	if _, err := alice.SendMessage(testCtx(t), "hello?"); !errors.Is(err, ErrNotPeer) {
		t.Fatalf("send during grace: got %v, want ErrNoPeer", err)
	}

	// Then it closes itself, and the session is free to start over.
	closed := expect[RoomClosed](t, atAlice, EventRoomClosed)
	if closed.Reason != "peer-left" {
		t.Fatalf("reason %q, want peer-left", closed.Reason)
	}
	if alice.State().InRoom {
		t.Fatal("room should be gone after the grace period")
	}
	if _, err := alice.CreateRoom(testCtx(t), "Alice"); err != nil {
		t.Fatalf("CreateRoom after close: %v", err)
	}
}

func TestSession_HostLeaves_GuestIsToldToo(t *testing.T) {
	alice, _, _, atBob := connectedPair(t, time.Minute)

	alice.Leave()

	left := expect[PeerLeft](t, atBob, EventPeerLeft)
	if left.Nickname != "Alice" {
		t.Fatalf("Bob sees %q leave, want Alice", left.Nickname)
	}
}

func TestSession_LeaveDuringGraceCancelsTheTimer(t *testing.T) {
	alice, bob, atAlice, _ := connectedPair(t, 100*time.Millisecond)

	bob.Leave()
	expect[PeerLeft](t, atAlice, EventPeerLeft)
	alice.Leave() // user closes the room themselves

	time.Sleep(300 * time.Millisecond) // well past the grace period
	select {
	case e := <-atAlice.ch:
		t.Fatalf("unexpected event after Leave: %s", e.name)
	default: // nothing fired: the timer was cancelled
	}
}
