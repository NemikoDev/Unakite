package signaling

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(NewServer(NewStore(time.Minute)).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// decode is a generic function
func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	return v
}

func TestCreateThenJoin_RoundTrip(t *testing.T) {
	ts := newTestServer(t)
	addrs := `["/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWExample"]`

	resp := post(t, ts.URL+"/rooms", `{"addrs":`+addrs+`}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d, want 201", resp.StatusCode)
	}
	code := decode[CreateRoomResponse](t, resp).Code
	if len(code) != CodeLength {
		t.Fatalf("code %q has wrong length", code)
	}

	resp = post(t, ts.URL+"/rooms/"+code+"/join", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("join: status %d, want 200", resp.StatusCode)
	}
	got := decode[JoinRoomResponse](t, resp).Addrs
	if len(got) != 1 || got[0] != "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWExample" {
		t.Fatalf("join returned %v", got)
	}

	// The code is burned: a second join must fail.
	resp = post(t, ts.URL+"/rooms/"+code+"/join", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second join: status %d, want 404", resp.StatusCode)
	}
}

func TestJoin_UnknownCodeIs404(t *testing.T) {
	ts := newTestServer(t)
	resp := post(t, ts.URL+"/rooms/ZZZZZZZZ/join", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", resp.StatusCode)
	}
}

func TestCreate_BadRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty body", ``},
		{"malformed JSON", `{"addrs": [`},
		{"missing addrs", `{}`},
		{"empty addrs list", `{"addrs": []}`},
		{"empty address string", `{"addrs": [""]}`},
		{"unknown field", `{"addrs": ["/ip4/1.2.3.4/tcp/1"], "admin": true}`},
	}
	ts := newTestServer(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := post(t, ts.URL+"/rooms", tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status %d, want 400", resp.StatusCode)
			}
		})
	}
}

func TestCreate_OversizedBodyIs413(t *testing.T) {
	ts := newTestServer(t)
	body := `{"addrs":["` + strings.Repeat("a", 32<<10) + `"]}`
	resp := post(t, ts.URL+"/rooms", body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", resp.StatusCode)
	}
}

func TestWrongMethodIs405(t *testing.T) {
	ts := newTestServer(t)
	for _, path := range []string{"/rooms", "/rooms/ABCDEFGH/join"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s: status %d, want 405", path, resp.StatusCode)
		}
	}
}

func TestResponsesAreNotCacheable(t *testing.T) {
	ts := newTestServer(t)
	resp := post(t, ts.URL+"/rooms", `{"addrs":["/ip4/127.0.0.1/tcp/1"]}`)
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}
