package signaling

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

var validAddrs = []string{"/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWExample"}

// checks that the code returned by Create is well-formed: correct length and only contains characters from the alphabet.
func TestCreate_ReturnsWellFormedCode(t *testing.T) {
	s := NewStore(time.Minute)
	code, err := s.Create(validAddrs)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(code) != CodeLength {
		t.Fatalf("code %q has length %d, want %d", code, len(code), CodeLength)
	}
	for _, c := range code {
		if !strings.ContainsRune(codeAlphabet, c) {
			t.Fatalf("code %q contains %q, which is not in the alphabet", code, c)
		}
	}
}

// checks that Consume returns the correct addresses and that the code is burned after consumption.
func TestConsume_ReturnsAddrsAndBurnsCode(t *testing.T) {
	s := NewStore(time.Minute)
	code, _ := s.Create(validAddrs)

	got, err := s.Consume(code)
	if err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if len(got) != 1 || got[0] != validAddrs[0] {
		t.Fatalf("got %v, want %v", got, validAddrs)
	}

	if _, err := s.Consume(code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Consume: got %v, want ErrNotFound", err)
	}
}

// checks that Consume is case-insensitive and ignores whitespace.
func TestConsume_UnknownCode(t *testing.T) {
	s := NewStore(time.Minute)
	if _, err := s.Consume("ZZZZZZZZ"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

// checks that Consume is case-insensitive and ignores whitespace.
func TestConsume_IsCaseAndWhitespaceInsensitive(t *testing.T) {
	s := NewStore(time.Minute)
	code, _ := s.Create(validAddrs)

	if _, err := s.Consume("  " + strings.ToLower(code) + " "); err != nil {
		t.Fatalf("expected lowercase/padded code to work, got: %v", err)
	}
}

// checks that Consume rejects expired codes and removes them from the store.
func TestConsume_ExpiredCodeRejected(t *testing.T) {
	s := NewStore(10 * time.Minute)
	clock := time.Now()
	s.now = func() time.Time { return clock }

	code, _ := s.Create(validAddrs)
	clock = clock.Add(10*time.Minute + time.Second)

	if _, err := s.Consume(code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound for expired code", err)
	}
	if s.Len() != 0 {
		t.Fatal("expired code should have been removed on lookup")
	}
}

// checks that DeleteExpired removes only expired codes and returns the correct count.
func TestDeleteExpired_RemovesOnlyExpired(t *testing.T) {
	s := NewStore(10 * time.Minute)
	clock := time.Now()
	s.now = func() time.Time { return clock }

	s.Create(validAddrs)
	clock = clock.Add(6 * time.Minute)
	s.Create(validAddrs)
	clock = clock.Add(5 * time.Minute)

	if removed := s.DeleteExpired(); removed != 1 {
		t.Fatalf("removed %d, want 1", removed)
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}
}

// helper function to generate a slice of n valid addresses for testing.
func manyAddrs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 4000+i)
	}
	return out
}

// checks that Create rejects invalid addresses.
func TestCreate_RejectsInvalidAddrs(t *testing.T) {
	cases := []struct {
		name  string
		addrs []string
	}{
		{"nil list", nil},
		{"empty list", []string{}},
		{"empty address", []string{""}},
		{"too many addresses", manyAddrs(maxAddrs + 1)},
		{"address too long", []string{strings.Repeat("a", maxAddrLen+1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore(time.Minute)
			if _, err := s.Create(tc.addrs); !errors.Is(err, ErrInvalidAddrs) {
				t.Fatalf("got %v, want ErrInvalidAddrs", err)
			}
		})
	}
}

// checks that Create stores a copy of the input addresses, so that mutations to the caller's slice do not affect the store.
func TestCreate_StoresACopyOfTheInput(t *testing.T) {
	s := NewStore(time.Minute)
	input := []string{"/ip4/127.0.0.1/tcp/1"}
	code, _ := s.Create(input)

	input[0] = "/ip4/6.6.6.6/tcp/666"

	got, _ := s.Consume(code)
	if got[0] != "/ip4/127.0.0.1/tcp/1" {
		t.Fatalf("store was affected by caller mutation: %v", got)
	}
}

// checks that Create generates unique codes even under concurrent calls.
func TestCreate_CodesAreUnique(t *testing.T) {
	s := NewStore(time.Minute)
	seen := make(map[string]bool)
	for i := 0; i < 2000; i++ {
		code, err := s.Create(validAddrs)
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		if seen[code] {
			t.Fatalf("duplicate code %q", code)
		}
		seen[code] = true
	}
}

// checks that the store can be used concurrently without data races or panics.
func TestStore_ConcurrentUse(t *testing.T) {
	s := NewStore(time.Minute)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, err := s.Create(validAddrs)
			if err != nil {
				t.Errorf("Create: %v", err)
			}
			if _, err := s.Consume(code); err != nil {
				t.Errorf("Consume: %v", err)
			}
		}()
	}
	wg.Wait()

	if s.Len() != 0 {
		t.Fatalf("Len = %d, want 0 after every code was consumed", s.Len())
	}
}

// checks that only one goroutine can successfully consume a given code, even under high concurrency.
func TestConsume_OnlyOneConcurrentWinner(t *testing.T) {
	s := NewStore(time.Minute)
	code, _ := s.Create(validAddrs)

	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Consume(code); err == nil {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if winners != 1 {
		t.Fatalf("%d goroutines consumed the same code, want exactly 1", winners)
	}
}

// checks that the cleanup goroutine removes expired rooms as expected.
func TestRunCleanup_SweepsExpiredRooms(t *testing.T) {
	s := NewStore(20 * time.Millisecond)
	s.Create(validAddrs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.RunCleanup(ctx, 10*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for s.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("cleanup never removed the expired room")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
