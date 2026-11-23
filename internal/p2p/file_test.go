package p2p

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// --- helpers ---

type fileSide struct {
	svc  *FileService
	host host.Host
	dir  string        // this side's save folder
	prog chan Progress // every progress update this side emitted
}

func newFileSide(t *testing.T, h host.Host, cfg FileConfig) *fileSide {
	t.Helper()
	dir := t.TempDir()
	prog := make(chan Progress, 1024)
	cfg.Dir = dir
	cfg.OnProgress = func(p Progress) {
		select {
		case prog <- p:
		default: // never block the transfer goroutine on a test's channel
		}
	}
	svc := NewFileService(h, cfg)
	t.Cleanup(svc.Close)
	return &fileSide{svc: svc, host: h, dir: dir, prog: prog}
}

// filePair returns two connected sides: a (the sender) and b (the receiver).
func filePair(t *testing.T, cfgA, cfgB FileConfig) (a, b *fileSide) {
	t.Helper()
	hostA, hostB := newTestHost(t), newTestHost(t)
	if _, err := Connect(roomCtx(t), hostA, loopbackAddr(t, hostB)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return newFileSide(t, hostA, cfgA), newFileSide(t, hostB, cfgB)
}

func acceptAll(context.Context, peer.ID, Offer) bool { return true }

// waitStatus skips updates until one has the wanted status.
func waitStatus(t *testing.T, ch <-chan Progress, want TransferStatus) Progress {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case p := <-ch:
			if p.Status == want {
				return p
			}
		case <-timeout:
			t.Fatalf("timed out waiting for status %q", want)
		}
	}
}

func writeSource(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("writing source file: %v", err)
	}
	return p
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return b
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// hostileStream plays a peer that speaks the protocol by hand, so it can send
// things the real Send never would. It returns once the receiver has answered the offer.
func hostileStream(t *testing.T, from host.Host, to peer.ID, offer fileOffer) (network.Stream, fileReply) {
	t.Helper()
	s, err := from.NewStream(roomCtx(t), to, FileProtocolID)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	t.Cleanup(func() { _ = s.Reset() })
	_ = s.SetDeadline(time.Now().Add(5 * time.Second)) // a bug should fail the test, not hang it

	if err := writeFrame(s, offer); err != nil {
		t.Fatalf("writing offer: %v", err)
	}
	var reply fileReply
	if err := readFrame(s, &reply); err != nil {
		t.Fatalf("reading reply: %v", err)
	}
	return s, reply
}

// --- the happy path ---

func TestFile_RoundTrip(t *testing.T) {
	a, b := filePair(t, FileConfig{}, FileConfig{OnOffer: acceptAll})

	// Deliberately not a multiple of the chunk size, to exercise the short last chunk.
	data := randomBytes(t, 3*chunkSize+123)
	src := writeSource(t, "report.pdf", data)

	if err := a.svc.Send(roomCtx(t), b.host.ID(), src); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := waitStatus(t, b.prog, StatusComplete)
	saved, err := os.ReadFile(got.Path)
	if err != nil {
		t.Fatalf("reading received file: %v", err)
	}
	if !bytes.Equal(saved, data) {
		t.Fatal("received bytes differ from the sent bytes")
	}
	if filepath.Dir(got.Path) != b.dir || filepath.Base(got.Path) != "report.pdf" {
		t.Fatalf("saved at %q, want report.pdf inside %q", got.Path, b.dir)
	}
	if got.Direction != DirReceiving || got.Size != int64(len(data)) || got.Done != got.Size {
		t.Fatalf("bad receiver progress: %+v", got)
	}

	sent := waitStatus(t, a.prog, StatusComplete)
	if sent.Direction != DirSending || sent.Name != "report.pdf" {
		t.Fatalf("bad sender progress: %+v", sent)
	}
	if left := dirEntries(t, b.dir); len(left) != 1 {
		t.Fatalf("expected only the final file in the folder, got %v", left)
	}
}

func TestFile_EmptyFile(t *testing.T) {
	a, b := filePair(t, FileConfig{}, FileConfig{OnOffer: acceptAll})
	src := writeSource(t, "empty.txt", nil)

	if err := a.svc.Send(roomCtx(t), b.host.ID(), src); err != nil {
		t.Fatalf("Send: %v", err)
	}
	got := waitStatus(t, b.prog, StatusComplete)
	if info, err := os.Stat(got.Path); err != nil || info.Size() != 0 {
		t.Fatalf("expected an empty file, got %v (err %v)", info, err)
	}
}

// --- refusals ---

func TestFile_DeclinedByReceiver(t *testing.T) {
	a, b := filePair(t, FileConfig{}, FileConfig{
		OnOffer: func(context.Context, peer.ID, Offer) bool { return false },
	})
	src := writeSource(t, "nope.txt", []byte("hello"))

	err := a.svc.Send(roomCtx(t), b.host.ID(), src)
	if !errors.Is(err, ErrRejected) {
		t.Fatalf("got %v, want ErrRejected", err)
	}
	waitStatus(t, a.prog, StatusRejected)
	waitStatus(t, b.prog, StatusRejected)
	if left := dirEntries(t, b.dir); len(left) != 0 {
		t.Fatalf("a declined file left %v behind", left)
	}
}

func TestFile_TooLargeIsRefusedWithoutAskingTheUser(t *testing.T) {
	a, b := filePair(t, FileConfig{}, FileConfig{
		MaxSize: 10,
		OnOffer: func(context.Context, peer.ID, Offer) bool {
			t.Error("OnOffer must not be called for an oversized offer")
			return true
		},
	})
	src := writeSource(t, "big.bin", randomBytes(t, 100))

	err := a.svc.Send(roomCtx(t), b.host.ID(), src)
	if !errors.Is(err, ErrRejected) || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("got %v, want a rejection that mentions size", err)
	}
}

func TestFile_SenderRefusesOwnOversizedFile(t *testing.T) {
	a, b := filePair(t, FileConfig{MaxSize: 10}, FileConfig{OnOffer: acceptAll})
	src := writeSource(t, "big.bin", randomBytes(t, 100))

	if err := a.svc.Send(roomCtx(t), b.host.ID(), src); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("got %v, want ErrFileTooLarge", err)
	}
}

func TestFile_DirectoryIsNotSendable(t *testing.T) {
	a, b := filePair(t, FileConfig{}, FileConfig{OnOffer: acceptAll})
	if err := a.svc.Send(roomCtx(t), b.host.ID(), t.TempDir()); !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("got %v, want ErrNotRegularFile", err)
	}
}

// --- a hostile or broken sender ---

func TestFile_ReceiverSanitizesHostileNames(t *testing.T) {
	cases := []struct{ sent, want string }{
		{"../../evil.txt", "evil.txt"},
		{`..\..\evil.txt`, "evil.txt"},
		{"CON", "_CON"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			a, b := filePair(t, FileConfig{}, FileConfig{OnOffer: acceptAll})
			payload := []byte("payload")

			s, reply := hostileStream(t, a.host, b.host.ID(), fileOffer{Name: tc.sent, Size: int64(len(payload))})
			if !reply.Accept {
				t.Fatalf("offer refused: %+v", reply)
			}
			sum := sha256.Sum256(payload)
			if _, err := s.Write(payload); err != nil {
				t.Fatalf("writing payload: %v", err)
			}
			if err := writeFrame(s, fileTrailer{SHA256: hex.EncodeToString(sum[:])}); err != nil {
				t.Fatalf("writing trailer: %v", err)
			}

			done := waitStatus(t, b.prog, StatusComplete)
			if filepath.Dir(done.Path) != b.dir {
				t.Fatalf("file escaped its folder: %q", done.Path)
			}
			if filepath.Base(done.Path) != tc.want {
				t.Fatalf("saved as %q, want %q", filepath.Base(done.Path), tc.want)
			}
			if _, err := os.Stat(filepath.Join(b.dir, "..", "..", "evil.txt")); err == nil {
				t.Fatal("a file appeared two levels above the save folder")
			}
		})
	}
}

func TestFile_ChecksumMismatchIsRejectedAndCleanedUp(t *testing.T) {
	a, b := filePair(t, FileConfig{}, FileConfig{OnOffer: acceptAll})

	s, reply := hostileStream(t, a.host, b.host.ID(), fileOffer{Name: "x.bin", Size: 10})
	if !reply.Accept {
		t.Fatalf("offer refused: %+v", reply)
	}
	if _, err := s.Write([]byte("0123456789")); err != nil {
		t.Fatalf("writing payload: %v", err)
	}
	wrongHash := hex.EncodeToString(make([]byte, 32)) // 64 zeros
	if err := writeFrame(s, fileTrailer{SHA256: wrongHash}); err != nil {
		t.Fatalf("writing trailer: %v", err)
	}

	var res fileResult
	if err := readFrame(s, &res); err != nil || res.OK {
		t.Fatalf("expected a negative result frame, got %+v (err %v)", res, err)
	}
	failed := waitStatus(t, b.prog, StatusFailed)
	if !strings.Contains(failed.Error, "checksum") {
		t.Fatalf("unhelpful error: %q", failed.Error)
	}
	if left := dirEntries(t, b.dir); len(left) != 0 {
		t.Fatalf("corrupt transfer left %v behind", left)
	}
}

func TestFile_AbortedTransferLeavesNoFile(t *testing.T) {
	a, b := filePair(t, FileConfig{}, FileConfig{OnOffer: acceptAll})

	s, reply := hostileStream(t, a.host, b.host.ID(), fileOffer{Name: "half.bin", Size: 1000})
	if !reply.Accept {
		t.Fatalf("offer refused: %+v", reply)
	}
	if _, err := s.Write(make([]byte, 10)); err != nil { // promised 1000, sent 10
		t.Fatalf("writing: %v", err)
	}
	_ = s.Reset() // the sender vanishes mid-transfer

	waitStatus(t, b.prog, StatusFailed)
	if left := dirEntries(t, b.dir); len(left) != 0 {
		t.Fatalf("aborted transfer left %v behind", left)
	}
}

// --- unit tests for the helpers ---

func TestSanitizeFileName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"report.pdf", "report.pdf"},
		{"Zoë.txt", "Zoë.txt"},
		{"../../etc/passwd", "passwd"},
		{`C:\Users\bob\secret.txt`, "secret.txt"},
		{"a<b>c:d.txt", "a_b_c_d.txt"},
		{"tab\there.txt", "tab_here.txt"},
		{"..", "file"},
		{"", "file"},
		{"   ", "file"},
		{"trailing. . ", "trailing"},
		{"nul.txt", "_nul.txt"},
		{"COM1", "_COM1"},
		{"console.txt", "console.txt"},
		{strings.Repeat("a", 200) + ".pdf", strings.Repeat("a", 56) + ".pdf"},
	}
	for _, tc := range cases {
		if got := SanitizeFileName(tc.in); got != tc.want {
			t.Errorf("SanitizeFileName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMoveIntoPlace_NeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".x.part")
	if err := os.WriteFile(tmp, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	final, err := moveIntoPlace(tmp, dir, "a.txt")
	if err != nil {
		t.Fatalf("moveIntoPlace: %v", err)
	}
	if filepath.Base(final) != "a (1).txt" {
		t.Fatalf("got %q, want %q", filepath.Base(final), "a (1).txt")
	}
	if orig, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(orig) != "original" {
		t.Fatalf("the existing file was overwritten: %q", orig)
	}
}
