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

func TestConsume_UnknownCode(t *testing.T) {
	s := NewStore(time.Minute)
	if _, err := s.Consume("ZZZZZZZZ"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestConsume_IsCaseAndWhitespaceInsensitive(t *testing.T) {
	s := NewStore(time.Minute)
	code, _ := s.Create(validAddrs)

	if _, err := s.Consume("  " + strings.ToLower(code) + " "); err != nil {
		t.Fatalf("expected lowercase/padded code to work, got: %v", err)
	}
}

func TestConsume_ExpiredCodeRejected(t *testing.T) {
	s := NewStore(10 * time.Minute)
	clock := time.Now()
	s.now = func() time.Time { return clock } // fake clock we control

	code, _ := s.Create(validAddrs)
	clock = clock.Add(10*time.Minute + time.Second) // fast-forward past the TTL

	if _, err := s.Consume(code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound for expired code", err)
	}
	if s.Len() != 0 {
		t.Fatal("expired code should have been removed on lookup")
	}
}

func TestDeleteExpired_RemovesOnlyExpired(t *testing.T) {
	s := NewStore(10 * time.Minute)
	clock := time.Now()
	s.now = func() time.Time { return clock }

	s.Create(validAddrs) // created at t=0, expires at t=10m
	clock = clock.Add(6 * time.Minute)
	s.Create(validAddrs)               // created at t=6m, expires at t=16m
	clock = clock.Add(5 * time.Minute) // now t=11m: first expired, second alive

	if removed := s.DeleteExpired(); removed != 1 {
		t.Fatalf("removed %d, want 1", removed)
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}
}

func manyAddrs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 4000+i)
	}
	return out
}

// Table-driven test: the standard Go way to run one test body over many inputs.
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
		t.Run(tc.name, func(t *testing.T) { // each case shows up as its own sub-test
			s := NewStore(time.Minute)
			if _, err := s.Create(tc.addrs); !errors.Is(err, ErrInvalidAddrs) {
				t.Fatalf("got %v, want ErrInvalidAddrs", err)
			}
		})
	}
}

func TestCreate_StoresACopyOfTheInput(t *testing.T) {
	s := NewStore(time.Minute)
	input := []string{"/ip4/127.0.0.1/tcp/1"}
	code, _ := s.Create(input)

	input[0] = "/ip4/6.6.6.6/tcp/666" // caller mutates their slice afterwards

	got, _ := s.Consume(code)
	if got[0] != "/ip4/127.0.0.1/tcp/1" {
		t.Fatalf("store was affected by caller mutation: %v", got)
	}
}

func TestCreate_CodesAreUnique(t *testing.T) {
	s := NewStore(time.Minute)
	seen := make(map[string]bool) // a Go "set" is a map with bool values
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

func TestStore_ConcurrentUse(t *testing.T) {
	s := NewStore(time.Minute)
	var wg sync.WaitGroup // like a CountDownLatch

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { // start a goroutine
			defer wg.Done()
			code, err := s.Create(validAddrs)
			if err != nil {
				t.Errorf("Create: %v", err) // Errorf, not Fatalf: Fatal must not be called from other goroutines
				return
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

func TestRunJanitor_SweepsExpiredRooms(t *testing.T) {
	s := NewStore(20 * time.Millisecond) // real clock, very short TTL
	s.Create(validAddrs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stops the janitor goroutine when the test ends
	go s.RunCleanup(ctx, 10*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for s.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("janitor never removed the expired room")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
