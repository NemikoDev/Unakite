package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/peer"

	"Unakite/internal/p2p"
)

var ErrUnknownOffer = errors.New("that file offer is no longer pending")

// returns the default download directory for received files.
func DefaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "Unakite")
	}
	return filepath.Join(home, "Downloads", "Unakite")
}

func (r *room) cancelOffers() {
	for _, ch := range r.offers {
		select {
		case ch <- false:
		default:
		}
	}
}

// start sending a file to the current peer. Returns an error if not in a room, no peer has joined, or the file cannot be preflighted.
func (s *Session) SendFile(path string) error {
	s.mu.Lock()
	r := s.cur
	var to peer.ID
	here := false
	if r != nil {
		to, here = r.expectedPeer(), r.peerHere
	}
	s.mu.Unlock()

	if r == nil {
		return ErrNotInRoom
	}
	if !here {
		return ErrNotPeer
	}
	if err := r.files.Preflight(path); err != nil {
		return err
	}

	go func() { _ = r.files.Send(r.life, to, path) }()
	return nil
}

func (s *Session) AcceptFile(id string) error  { return s.decide(id, true) }
func (s *Session) DeclineFile(id string) error { return s.decide(id, false) }

// handle the decision for a pending file offer. Returns an error if not in a room or the offer is unknown.
func (s *Session) decide(id string, accept bool) error {
	s.mu.Lock()
	r := s.cur
	var ch chan bool
	if r != nil {
		ch = r.offers[id]
	}
	s.mu.Unlock()

	if r == nil {
		return ErrNotInRoom
	}
	if ch == nil {
		return ErrUnknownOffer
	}
	select {
	case ch <- accept:
	default:
	}
	return nil
}

// accept or decline a pending file offer by its ID. Returns an error if not in a room or the offer is unknown.
func (s *Session) onOffer(ctx context.Context, r *room, from peer.ID, o p2p.Offer) bool {
	decision := make(chan bool, 1)

	s.mu.Lock()
	allowed := s.cur == r && from == r.expectedPeer() && r.peerHere && r.closesAt.IsZero()
	if allowed {
		r.offers[o.ID] = decision
	}
	s.mu.Unlock()
	if !allowed {
		return false
	}
	defer func() {
		s.mu.Lock()
		delete(r.offers, o.ID)
		s.mu.Unlock()
	}()

	s.publish(r, FileTransfer{
		ID: o.ID, FileName: o.Name, FileSize: o.Size,
		Direction: string(p2p.DirReceiving), Status: string(p2p.StatusAwaiting),
	})

	select {
	case ok := <-decision:
		return ok
	case <-ctx.Done():
		return false
	}
}

// handle progress updates for a file transfer. Updates are ignored if the room or peer is not current.
func (s *Session) onFileProgress(r *room, p p2p.Progress) {
	if p.Direction == p2p.DirReceiving && p.Status == p2p.StatusAwaiting {
		return
	}
	s.mu.Lock()
	ok := s.cur == r && p.Peer == r.expectedPeer()
	s.mu.Unlock()
	if !ok {
		return
	}
	s.publish(r, toTransfer(p))
}

// publish records the file transfer and emits an event to the UI.
func (s *Session) publish(r *room, t FileTransfer) {
	s.mu.Lock()
	if s.cur != r {
		s.mu.Unlock()
		return
	}
	if _, seen := r.transfers[t.ID]; !seen {
		r.order = append(r.order, t.ID)
	}
	r.transfers[t.ID] = t
	s.mu.Unlock()

	s.emit(EventFileTransfer, t)
}

// convert a p2p.Progress to a FileTransfer structure for UI updates.
func toTransfer(p p2p.Progress) FileTransfer {
	pct := 0
	switch {
	case p.Status == p2p.StatusComplete:
		pct = 100
	case p.Size > 0:
		pct = int(p.Done * 100 / p.Size)
		if pct > 100 {
			pct = 100
		}
	}
	return FileTransfer{
		ID: p.ID, FileName: p.Name, FileSize: p.Size,
		Direction: string(p.Direction), Progress: pct, Status: string(p.Status),
		Error: p.Error, Path: p.Path,
	}
}
