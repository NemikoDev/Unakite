package session

import (
	"Unakite/internal/identity"
	"Unakite/internal/p2p"
	"Unakite/internal/signaling"
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	maxNickname         = 32
	maxMessage          = 4000
	helloTimeout        = 5 * time.Second
	defaultWaitForGuest = 10 * time.Minute
	defaultGracePeriod  = 5 * time.Minute
)

var (
	ErrAlreadyInRoom   = errors.New("already in a room")
	ErrNotInRoom       = errors.New("not in a room")
	ErrNotPeer         = errors.New("nobody has joined yet")
	ErrEmptyMessage    = errors.New("message is empty")
	ErrMessageTooLong  = errors.New("message is too long")
	ErrInvalidNickname = errors.New("invalid nickname")
)

type room struct {
	host     host.Host
	gate     *p2p.RoomGate
	chat     *p2p.ChatService
	notify   *network.NotifyBundle
	self     PeerInfo
	life     context.Context
	end      context.CancelFunc
	code     string
	remote   peer.ID
	peerNick string
	peerHere bool
	closesAt time.Time
	grace    *time.Timer
}

// returns the peer ID of the host or the guest if there is no host
func (r *room) expectedPeer() peer.ID {
	if r.gate != nil {
		return r.gate.Guest()
	}
	return r.remote
}

// close the room and stop the signaling client
func (r *room) close() {
	r.end()
	r.host.Network().StopNotify(r.notify)
	r.chat.Close()
	r.host.Close()
}

func (r *room) stopGrace() {
	if r.grace != nil {
		r.grace.Stop()
	}
}

type Session struct {
	sc           *signaling.Client
	emit         Emitter
	waitForGuest time.Duration
	gracePeriod  time.Duration
	opMu         sync.Mutex
	mu           sync.Mutex
	cur          *room
	seq          atomic.Uint64
}

// create a new session with the given signaling client and event emitter.
func New(sc *signaling.Client, emit Emitter) *Session {
	if emit == nil {
		emit = func(string, any) {}
	}
	return &Session{
		sc:           sc,
		emit:         emit,
		waitForGuest: defaultWaitForGuest,
		gracePeriod:  defaultGracePeriod,
	}
}

// create a new room with the given nickname and hosting status. Returns an error if the session is already in a room or the nickname is invalid.
func (s *Session) CreateRoom(ctx context.Context, nickname string) (string, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	r, err := s.begin(nickname, true)
	if err != nil {
		return "", err
	}
	code, err := p2p.CreateRoom(ctx, s.sc, r.host)
	if err != nil {
		s.abort(r)
		return "", err
	}
	s.mu.Lock()
	r.code = code
	s.mu.Unlock()
	go s.watchExpiry(r)
	return code, nil
}

// join an existing room with the given nickname and code. Returns an error if the session is already in a room, the nickname is invalid, or the room cannot be found.
func (s *Session) JoinRoom(ctx context.Context, nickname, code string) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	r, err := s.begin(nickname, false)
	if err != nil {
		return err
	}
	hostID, err := p2p.JoinRoom(ctx, s.sc, r.host, code)
	if err != nil {
		s.abort(r)
		return err
	}

	s.mu.Lock()
	r.remote = hostID
	r.code = strings.ToUpper(strings.TrimSpace(code))
	s.mu.Unlock()

	hello := p2p.ChatMessage{Type: p2p.TypeHello, Nickname: r.self.Nickname, Timestamp: time.Now().UnixMilli()}
	if err := r.chat.Send(ctx, hostID, hello); err != nil {
		s.abort(r)
		return fmt.Errorf("introducing ourselves: %w", err)
	}
	return nil
}

// send a message to the current peer. Returns an error if not in a room, no peer has joined, the message is empty, or the message is too long.
func (s *Session) SendMessage(ctx context.Context, content string) (Message, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Message{}, ErrEmptyMessage
	}
	if utf8.RuneCountInString(content) > maxMessage {
		return Message{}, ErrMessageTooLong
	}

	s.mu.Lock()
	r := s.cur
	var to peer.ID
	here := false
	if r != nil {
		to, here = r.expectedPeer(), r.peerHere
	}
	s.mu.Unlock()

	if r == nil {
		return Message{}, ErrNotInRoom
	}
	if !here {
		return Message{}, ErrNotPeer
	}

	ts := time.Now().UnixMilli()
	msg := p2p.ChatMessage{Nickname: r.self.Nickname, Content: content, Timestamp: ts}
	if err := r.chat.Send(ctx, to, msg); err != nil {
		return Message{}, fmt.Errorf("sending message: %w", err)
	}
	return Message{ID: s.nextID(), Sender: r.self, Content: content, Timestamp: ts, IsOwn: true}, nil
}

// leave the current room, if any.
func (s *Session) Leave() {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.mu.Lock()
	r := s.cur
	s.cur = nil
	if r != nil {
		r.stopGrace()
	}
	s.mu.Unlock()

	if r != nil {
		r.close()
	}
}

// initializes a new room with the given nickname and hosting status. Returns an error if the session is already in a room or the nickname is invalid.
func (s *Session) begin(nickname string, hosting bool) (*room, error) {
	nickname, err := validateNickname(nickname)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	busy := s.cur != nil
	s.mu.Unlock()
	if busy {
		return nil, ErrAlreadyInRoom
	}

	r, err := s.newRoom(nickname, hosting)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cur = r
	s.mu.Unlock()
	return r, nil
}

// aborts the current room if it matches the given room, closing it and clearing the session's current room pointer.
func (s *Session) abort(r *room) {
	s.mu.Lock()
	if s.cur == r {
		s.cur = nil
	}
	r.stopGrace()
	s.mu.Unlock()
	r.close()
}

// creates a new room with the given nickname and hosting status. Returns the room and any error encountered.
func (s *Session) newRoom(nickname string, hosting bool) (*room, error) {
	priv, _, err := identity.GenerateEphemeralIdentity()
	if err != nil {
		return nil, err
	}

	r := &room{}
	var opts []libp2p.Option
	if hosting {
		r.gate = p2p.NewRoomGate()
		opts = append(opts, libp2p.ConnectionGater(r.gate))
	}
	h, err := p2p.NewHost(priv, opts...)
	if err != nil {
		return nil, err
	}

	r.host = h
	r.life, r.end = context.WithCancel(context.Background())
	r.self = PeerInfo{ID: h.ID().String(), Nickname: nickname}
	r.chat = p2p.NewChatService(h, func(from peer.ID, m p2p.ChatMessage) { s.onChat(r, from, m) })
	r.notify = &network.NotifyBundle{
		DisconnectedF: func(n network.Network, c network.Conn) { s.onDisconnected(r, n, c) },
	}
	h.Network().Notify(r.notify)
	return r, nil
}

// handles an incoming chat message, classifying it and emitting the appropriate event.
func (s *Session) onChat(r *room, from peer.ID, m p2p.ChatMessage) {
	event, payload, replyHello := s.classify(r, from, m)
	log.Printf("onChat: type=%q from=%s -> event=%q", m.Type, from, event)
	if event == "" {
		return
	}
	s.emit(event, payload)
	if replyHello {
		go s.sendHello(r, from)
	}
}

// classifies an incoming chat message, determining the event type, payload, and whether a hello reply is needed.
func (s *Session) classify(r *room, from peer.ID, m p2p.ChatMessage) (event string, payload any, replyHello bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cur != r || from == "" || from != r.expectedPeer() {
		return "", nil, false
	}

	switch m.Type {
	case p2p.TypeHello:
		if r.peerHere || !r.closesAt.IsZero() {
			return "", nil, false
		}
		r.peerHere = true
		r.peerNick = sanitizePeerNickname(m.Nickname)
		return EventPeerJoined, PeerInfo{ID: from.String(), Nickname: r.peerNick}, r.gate != nil

	case p2p.TypeChat:
		content := strings.TrimSpace(m.Content)
		if !r.peerHere || content == "" || utf8.RuneCountInString(content) > maxMessage {
			return "", nil, false
		}
		return EventMessage, Message{
			ID:        s.nextID(),
			Sender:    PeerInfo{ID: from.String(), Nickname: r.peerNick},
			Content:   content,
			Timestamp: time.Now().UnixMilli(),
		}, false
	}
	return "", nil, false
}

// sends a hello message to the specified peer, with a timeout.
func (s *Session) sendHello(r *room, to peer.ID) {
	ctx, cancel := context.WithTimeout(context.Background(), helloTimeout)
	defer cancel()
	_ = r.chat.Send(ctx, to, p2p.ChatMessage{
		Type: p2p.TypeHello, Nickname: r.self.Nickname, Timestamp: time.Now().UnixMilli(),
	})
}

func (s *Session) onDisconnected(r *room, n network.Network, c network.Conn) {
	p := c.RemotePeer()
	if n.Connectedness(p) == network.Connected {
		return
	}

	s.mu.Lock()
	if s.cur != r || p != r.expectedPeer() || !r.closesAt.IsZero() {
		s.mu.Unlock()
		return
	}
	r.peerHere = false
	closesAt := time.Now().Add(s.gracePeriod)
	r.closesAt = closesAt
	r.grace = time.AfterFunc(s.gracePeriod, func() { s.closeAfterGrace(r) })
	info := PeerLeft{
		PeerInfo: PeerInfo{ID: p.String(), Nickname: r.peerNick},
		ClosesAt: closesAt.UnixMilli(),
	}
	s.mu.Unlock()

	s.emit(EventPeerLeft, info)
}

func (s *Session) closeAfterGrace(r *room) {
	s.mu.Lock()
	still := s.cur == r
	if still {
		s.cur = nil
	}
	s.mu.Unlock()

	if !still {
		return
	}
	r.close()
	s.emit(EventRoomClosed, RoomClosed{Reason: "peer-left"})
}

// generates the next unique message ID for the session.
func (s *Session) nextID() string { return strconv.FormatUint(s.seq.Add(1), 10) }

// removes control characters and trims whitespace from the input string.
func stripControl(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
}

// validates a nickname, ensuring it is non-empty and does not exceed the maximum length.
func validateNickname(s string) (string, error) {
	s = stripControl(s)
	if n := utf8.RuneCountInString(s); n == 0 || n > maxNickname {
		return "", ErrInvalidNickname
	}
	return s, nil
}

// sanitizes a peer nickname by removing control characters, truncating to the maximum length, and defaulting to "Anonymous" if empty.
func sanitizePeerNickname(s string) string {
	s = stripControl(s)
	if r := []rune(s); len(r) > maxNickname {
		s = string(r[:maxNickname])
	}
	if s == "" {
		return "Anonymous"
	}
	return s
}

// monitors the room for expiry, triggering if the guest does not arrive in time.
func (s *Session) watchExpiry(r *room) {
	t := time.NewTimer(s.waitForGuest)
	defer t.Stop()
	select {
	case <-r.life.Done():
	case <-t.C:
		s.expireIfLonely(r)
	}
}

func (s *Session) expireIfLonely(r *room) {
	s.mu.Lock()
	lonely := s.cur == r && !r.peerHere && r.gate.Guest() == ""
	if lonely {
		s.cur = nil
	}
	s.mu.Unlock()

	if !lonely {
		return
	}
	r.close()
	s.emit(EventRoomClosed, RoomClosed{Reason: "expired"})
}

// reports what the session currently holds.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.cur
	if r == nil {
		return State{}
	}
	st := State{InRoom: true, Code: r.code, Self: r.self}
	if r.peerHere {
		st.Peer = &PeerInfo{ID: r.expectedPeer().String(), Nickname: r.peerNick}
	}
	if !r.closesAt.IsZero() {
		st.ClosesAt = r.closesAt.UnixMilli()
	}
	return st
}
