package signaling

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestClient_CreateThenJoin(t *testing.T) {
	ts := newTestServer(t)
	c := NewClient(ts.URL)
	addrs := []string{"/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWExample"}

	code, err := c.CreateRoom(testCtx(t), addrs)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	got, err := c.JoinRoom(testCtx(t), code)
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if !slices.Equal(got, addrs) {
		t.Fatalf("got %v, want %v", got, addrs)
	}

	if _, err := c.JoinRoom(testCtx(t), code); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second join: got %v, want ErrNotFound", err)
	}
}

func TestClient_BaseURLTrailingSlashIsFine(t *testing.T) {
	ts := newTestServer(t)
	c := NewClient(ts.URL + "/")
	if _, err := c.CreateRoom(testCtx(t), []string{"/ip4/127.0.0.1/tcp/1"}); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
}

func TestClient_JoinUnknownCode(t *testing.T) {
	c := NewClient(newTestServer(t).URL)
	if _, err := c.JoinRoom(testCtx(t), "ZZZZZZZZ"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestClient_JoinEmptyCodeNeverHitsTheNetwork(t *testing.T) {
	c := NewClient("http://127.0.0.1:1") // nothing listens here; a network call would fail differently
	if _, err := c.JoinRoom(testCtx(t), "   "); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestClient_CreateWithInvalidAddrsSurfacesServerMessage(t *testing.T) {
	c := NewClient(newTestServer(t).URL)
	_, err := c.CreateRoom(testCtx(t), []string{})
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want a rejection error", err)
	}
	if !strings.Contains(err.Error(), "invalid address list") {
		t.Fatalf("error %q should include the server's message", err)
	}
}

func TestClient_ServerUnreachable(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	url := ts.URL
	ts.Close() // now nothing is listening

	if _, err := NewClient(url).CreateRoom(testCtx(t), []string{"/ip4/127.0.0.1/tcp/1"}); err == nil {
		t.Fatal("expected an error when the server is down")
	}
}

// Not-our-server cases: the client must not misreport these as "room not found".
func TestClient_NotASignalingServer(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"html with 200", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>hi</html>")) }},
		{"plain 404", http.NotFound},
		{"json 200 of wrong shape", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[1,2,3]`)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()
			c := NewClient(ts.URL)

			_, err := c.JoinRoom(testCtx(t), "ABCDEFGH")
			if err == nil || errors.Is(err, ErrNotFound) {
				t.Fatalf("got %v, want a non-ErrNotFound error", err)
			}
			if !strings.Contains(err.Error(), "is it really a signaling server") {
				t.Fatalf("unhelpful error: %v", err)
			}
		})
	}
}

func TestClient_OversizedResponseRejected(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 2*maxResponseBytes)))
	}))
	defer ts.Close()

	_, err := NewClient(ts.URL).JoinRoom(testCtx(t), "ABCDEFGH")
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("got %v, want a 'too large' error", err)
	}
}

func TestClient_RespectsContextDeadline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // hang until the client gives up
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := NewClient(ts.URL).JoinRoom(ctx, "ABCDEFGH")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded", err)
	}
}
