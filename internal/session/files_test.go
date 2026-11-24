package session

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("writing source file: %v", err)
	}
	return p
}

// waitTransfer returns the first file:transfer event with the wanted status.
// Other progress updates and non-file events (like peer-left) are skipped.
func waitTransfer(t *testing.T, r *recorder, want string) FileTransfer {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e := <-r.ch:
			if e.name != EventFileTransfer {
				continue
			}
			ft, ok := e.payload.(FileTransfer)
			if !ok {
				t.Fatalf("file event payload is %T", e.payload)
			}
			if ft.Status == want {
				return ft
			}
		case <-timeout:
			t.Fatalf("timed out waiting for a transfer with status %q", want)
		}
	}
}

func TestSession_FileTransfer_AcceptAndReceive(t *testing.T) {
	alice, bob, atAlice, atBob := connectedPair(t, time.Minute)

	data := bytes.Repeat([]byte("unakite "), 50_000) // 400 KB: several chunks
	src := writeTemp(t, "notes.txt", data)
	if err := alice.SendFile(src); err != nil {
		t.Fatalf("SendFile: %v", err)
	}

	// Bob is asked first; nothing has been written yet.
	offer := waitTransfer(t, atBob, "awaiting")
	if offer.Direction != "receiving" || offer.FileName != "notes.txt" || offer.FileSize != int64(len(data)) {
		t.Fatalf("unexpected offer row: %+v", offer)
	}
	if err := bob.AcceptFile(offer.ID); err != nil {
		t.Fatalf("AcceptFile: %v", err)
	}

	done := waitTransfer(t, atBob, "complete")
	if done.ID != offer.ID || done.Progress != 100 {
		t.Fatalf("unexpected completion row: %+v", done)
	}
	if filepath.Dir(done.Path) != bob.downloadDir {
		t.Fatalf("saved outside the download folder: %q", done.Path)
	}
	saved, err := os.ReadFile(done.Path)
	if err != nil || !bytes.Equal(saved, data) {
		t.Fatalf("saved file differs from the original (read error: %v)", err)
	}

	sent := waitTransfer(t, atAlice, "complete")
	if sent.Direction != "sending" || sent.FileName != "notes.txt" {
		t.Fatalf("unexpected sender row: %+v", sent)
	}

	// A UI reload can rebuild its list from State().
	st := bob.State()
	if len(st.Transfers) != 1 || st.Transfers[0].Status != "complete" {
		t.Fatalf("State transfers: %+v", st.Transfers)
	}
}

func TestSession_FileTransfer_Declined(t *testing.T) {
	alice, bob, atAlice, atBob := connectedPair(t, time.Minute)

	if err := alice.SendFile(writeTemp(t, "no.txt", []byte("hi"))); err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	offer := waitTransfer(t, atBob, "awaiting")
	if err := bob.DeclineFile(offer.ID); err != nil {
		t.Fatalf("DeclineFile: %v", err)
	}

	waitTransfer(t, atBob, "rejected")
	waitTransfer(t, atAlice, "rejected")
	if entries, _ := os.ReadDir(bob.downloadDir); len(entries) != 0 {
		t.Fatalf("a declined file left %d entries behind", len(entries))
	}
}

func TestSession_PendingOfferIsCancelledWhenSenderLeaves(t *testing.T) {
	alice, bob, _, atBob := connectedPair(t, time.Minute)

	if err := alice.SendFile(writeTemp(t, "late.txt", []byte("hi"))); err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	offer := waitTransfer(t, atBob, "awaiting")

	alice.Leave() // the sender goes away while Bob's prompt is open

	waitTransfer(t, atBob, "rejected") // the prompt closes itself
	if err := bob.AcceptFile(offer.ID); !errors.Is(err, ErrUnknownOffer) {
		t.Fatalf("answering a dead offer: got %v, want ErrUnknownOffer", err)
	}
}

func TestSession_SendFileRules(t *testing.T) {
	s := New(newSignaling(t), nil)
	t.Cleanup(s.Leave)
	src := writeTemp(t, "a.txt", []byte("x"))

	if err := s.SendFile(src); !errors.Is(err, ErrNotInRoom) {
		t.Fatalf("outside a room: got %v, want ErrNotInRoom", err)
	}
	if err := s.AcceptFile("r1"); !errors.Is(err, ErrNotInRoom) {
		t.Fatalf("accept outside a room: got %v, want ErrNotInRoom", err)
	}
	if _, err := s.CreateRoom(testCtx(t), "Alice"); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := s.SendFile(src); !errors.Is(err, ErrNotPeer) {
		t.Fatalf("before anyone joined: got %v, want ErrNotPeer", err)
	}
	if err := s.AcceptFile("r1"); !errors.Is(err, ErrUnknownOffer) {
		t.Fatalf("unknown offer: got %v, want ErrUnknownOffer", err)
	}
}

func TestSession_SendFilePreflightErrorsAreImmediate(t *testing.T) {
	alice, _, _, _ := connectedPair(t, time.Minute)

	if err := alice.SendFile(t.TempDir()); err == nil {
		t.Fatal("sending a directory should fail")
	}
	if err := alice.SendFile(filepath.Join(t.TempDir(), "missing.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: got %v, want os.ErrNotExist", err)
	}
	if st := alice.State(); len(st.Transfers) != 0 {
		t.Fatalf("failed preflight must not create rows, got %+v", st.Transfers)
	}
}
