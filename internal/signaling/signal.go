package signaling

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// local in-memory store for signaling rooms.
const (
	codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	CodeLength   = 8

	maxAddrs   = 16
	maxAddrLen = 256
)

// ErrNotFound is returned when a room code is not found in the store.
// ErrInvalidAddrs is returned when the provided addresses are invalid.
var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidAddrs = errors.New("invalid addrs")
)

type room struct {
	addrs     []string
	expiresAt time.Time
}

type Store struct {
	ttl time.Duration
	now func() time.Time

	mu    sync.Mutex
	rooms map[string]room
}

func NewStore(ttl time.Duration) *Store {
	return &Store{
		ttl:   ttl,
		now:   time.Now,
		rooms: make(map[string]room),
	}
}

// Create generates a new unique code for the provided addresses and stores it in the store.
func (s *Store) Create(addrs []string) (string, error) {
	if err := validateAddrs(addrs); err != nil {
		return "", err
	}
	stored := append([]string(nil), addrs...)

	s.mu.Lock()
	defer s.mu.Unlock()

	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateCode()
		if err != nil {
			return "", err
		}
		if _, exists := s.rooms[code]; exists {
			continue
		}
		s.rooms[code] = room{addrs: stored, expiresAt: s.now().Add(s.ttl)}
		return code, nil
	}
	return "", errors.New("failed to generate unique code")
}

// Consume retrieves the addresses associated with the provided code and removes the code from the store.
func (s *Store) Consume(code string) ([]string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))

	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.rooms[code]
	if !ok {
		return nil, ErrNotFound
	}
	delete(s.rooms, code)
	if s.now().After(r.expiresAt) {
		return nil, ErrNotFound
	}
	return r.addrs, nil
}

// DeleteExpired removes all expired codes from the store and returns the number of removed codes.
func (s *Store) DeleteExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	removed := 0
	for code, r := range s.rooms {
		if !now.Before(r.expiresAt) {
			delete(s.rooms, code)
			removed++
		}
	}
	return removed
}

// returns the number of active codes in the store.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rooms)
}

// generates a random code of length CodeLength using the characters in CodeAlphabet.
func generateCode() (string, error) {
	b := make([]byte, CodeLength)
	// read random bytes and map them using crypto
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("reading rand bytes: %w", err)
	}
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b), nil
}

// validates provided addresses
func validateAddrs(addrs []string) error {
	if len(addrs) == 0 || len(addrs) > maxAddrs {
		return fmt.Errorf("%w: need 1-%d addresses, got %d", ErrInvalidAddrs, maxAddrs, len(addrs))
	}
	for _, a := range addrs {
		if a == "" || len(a) > maxAddrLen {
			return fmt.Errorf("%w: invalid address length", ErrInvalidAddrs)
		}
	}
	return nil
}

// goroutine that runs cleanup loop
func (s *Store) RunCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.DeleteExpired()
		}
	}
}
