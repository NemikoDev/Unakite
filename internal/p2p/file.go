package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	maxIncoming   = 3
	progressEvery = 100 * time.Millisecond
)

type TransferStatus string

const (
	StatusAwaiting TransferStatus = "awaiting"
	StatusActive   TransferStatus = "in-progress"
	StatusComplete TransferStatus = "complete"
	StatusFailed   TransferStatus = "failed"
	StatusRejected TransferStatus = "rejected"
)

type Direction string

const (
	DirSending   Direction = "sending"
	DirReceiving Direction = "receiving"
)

type Offer struct {
	ID   string
	Name string
	Size int64
}

type Progress struct {
	ID        string
	Peer      peer.ID
	Name      string
	Size      int64
	Direction Direction
	Done      int64
	Status    TransferStatus
	Path      string
	Error     string
}

type FileConfig struct {
	Dir        string
	MaxSize    int64
	OnOffer    func(ctx context.Context, from peer.ID, o Offer) bool
	OnProgress func(Progress)
}

type FileService struct {
	host  host.Host
	cfg   FileConfig
	life  context.Context
	end   context.CancelFunc
	slots chan struct{}
	seq   atomic.Uint64
}

// creates a new FileService with the given host and configuration.
func NewFileService(h host.Host, cfg FileConfig) *FileService {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = DefaultMaxFileSize
	}
	if cfg.OnOffer == nil {
		cfg.OnOffer = func(context.Context, peer.ID, Offer) bool { return false }
	}
	if cfg.OnProgress == nil {
		cfg.OnProgress = func(Progress) {}
	}
	life, end := context.WithCancel(context.Background())
	f := &FileService{host: h, cfg: cfg, life: life, end: end, slots: make(chan struct{}, maxIncoming)}
	h.SetStreamHandler(FileProtocolID, f.handleStream)
	return f
}

// checks if the file at the given path is valid for transfer, ensuring it is a regular file and within the allowed size limit.
func (f *FileService) Preflight(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ErrNotRegularFile
	}
	if info.Size() > f.cfg.MaxSize {
		return ErrFileTooLarge
	}
	return nil
}

func (f *FileService) Close() {
	f.end()
	f.host.RemoveStreamHandler(FileProtocolID)
}

func (f *FileService) nextID(prefix string) string {
	return prefix + strconv.FormatUint(f.seq.Add(1), 10)
}

type reporter struct {
	f    *FileService
	p    Progress
	last time.Time
}

// creates a new reporter for tracking the progress of a file transfer.
func (f *FileService) newReporter(p Progress) *reporter { return &reporter{f: f, p: p} }

// sets the current status, done bytes, error message, and path for the reporter and triggers the progress callback.
func (r *reporter) set(status TransferStatus, done int64, errMsg, path string) {
	r.p.Status, r.p.Done, r.p.Error, r.p.Path = status, done, errMsg, path
	r.last = time.Now()
	r.f.cfg.OnProgress(r.p)
}

// updates the reporter with the number of bytes transferred and triggers a progress update if necessary.
func (r *reporter) bytes(done int64) {
	r.p.Done = done
	if done < r.p.Size && time.Since(r.last) < progressEvery {
		return
	}
	r.set(StatusActive, done, "", "")
}

// marks the reporter as failed with the given error.
func (r *reporter) fail(err error) { r.set(StatusFailed, r.p.Done, err.Error(), "") }

// sends a file to the specified peer, reporting progress and handling errors appropriately.
func (f *FileService) Send(ctx context.Context, to peer.ID, path string) (err error) {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("reading file info: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ErrNotRegularFile
	}
	size := info.Size()
	if size > f.cfg.MaxSize {
		return ErrFileTooLarge
	}

	name := SanitizeFileName(filepath.Base(path))
	rep := f.newReporter(Progress{ID: f.nextID("s"), Peer: to, Name: name, Size: size, Direction: DirSending})
	rep.set(StatusAwaiting, 0, "", "")

	defer func() {
		if err == nil || errors.Is(err, ErrRejected) {
			return
		}
		if ctx.Err() != nil {
			err = fmt.Errorf("cancelled: %w", ctx.Err())
		}
		rep.fail(err)
	}()

	s, err := f.host.NewStream(ctx, to, FileProtocolID)
	if err != nil {
		return fmt.Errorf("opening file stream: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = s.Reset() })
	defer stop()

	clean := false
	defer func() {
		if clean {
			_ = s.Close()
		} else {
			_ = s.Reset()
		}
	}()
	// send the file offer to the peer and wait for their response.
	_ = s.SetWriteDeadline(time.Now().Add(frameTimeout))
	if err := writeFrame(s, fileOffer{Name: name, Size: size}); err != nil {
		return fmt.Errorf("sending offer: %w", err)
	}
	_ = s.SetReadDeadline(time.Now().Add(decisionTimeout + frameTimeout))
	var reply fileReply
	if err := readFrame(s, &reply); err != nil {
		return fmt.Errorf("waiting for the other person to answer: %w", err)
	}
	if !reply.Accept {
		rep.set(StatusRejected, 0, "", "")
		clean = true
		return rejectionError(reply.Reason)
	}

	rep.set(StatusActive, 0, "", "")
	hasher := sha256.New()
	touch := func() { _ = s.SetWriteDeadline(time.Now().Add(idleTimeout)) }
	if err := copyExactly(s, io.TeeReader(file, hasher), size, touch, rep.bytes); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return errors.New("the file changed while it was being sent")
		}
		return fmt.Errorf("sending data: %w", err)
	}

	touch()
	if err := writeFrame(s, fileTrailer{SHA256: hex.EncodeToString(hasher.Sum(nil))}); err != nil {
		return fmt.Errorf("sending checksum: %w", err)
	}
	_ = s.SetReadDeadline(time.Now().Add(verifyTimeout))
	var res fileResult
	if err := readFrame(s, &res); err != nil {
		return fmt.Errorf("waiting for confirmation: %w", err)
	}
	if !res.OK {
		return errors.New("the other side could not save the file intact")
	}

	rep.set(StatusComplete, size, "", "")
	clean = true
	return nil
}

// handles an incoming file stream from a peer.
// It reads the file offer, checks if it can accept the file, and then receives the file data into a temporary file.
// After receiving, it verifies the checksum and moves the file into place if everything is correct.
func (f *FileService) handleStream(s network.Stream) {
	from := s.Conn().RemotePeer()

	clean := false
	defer func() {
		if clean {
			_ = s.Close()
		} else {
			_ = s.Reset()
		}
	}()

	write := func(v any) bool {
		_ = s.SetWriteDeadline(time.Now().Add(frameTimeout))
		return writeFrame(s, v) == nil
	}
	reject := func(reason string) { clean = write(fileReply{Reason: reason}) }
	result := func(ok bool) { clean = write(fileResult{OK: ok}) }

	_ = s.SetReadDeadline(time.Now().Add(frameTimeout))
	var offer fileOffer
	if err := readFrame(s, &offer); err != nil {
		return
	}
	if offer.Size < 0 || offer.Size > f.cfg.MaxSize {
		reject(reasonTooLarge)
		return
	}
	select {
	case f.slots <- struct{}{}:
		defer func() { <-f.slots }()
	default:
		reject(reasonBusy)
		return
	}

	name := SanitizeFileName(offer.Name)
	rep := f.newReporter(Progress{ID: f.nextID("r"), Peer: from, Name: name, Size: offer.Size, Direction: DirReceiving})
	rep.set(StatusAwaiting, 0, "", "")

	ctx, cancel := context.WithTimeout(f.life, decisionTimeout)
	accepted := f.cfg.OnOffer(ctx, from, Offer{ID: rep.p.ID, Name: name, Size: offer.Size})
	cancel()
	if !accepted {
		reject(reasonDeclined)
		rep.set(StatusRejected, 0, "", "")
		return
	}

	var tmp *os.File
	err := os.MkdirAll(f.cfg.Dir, 0o700)
	if err == nil {
		tmp, err = os.CreateTemp(f.cfg.Dir, ".unakite-*.part")
	}
	if err != nil {
		reject(reasonDeclined)
		rep.fail(err)
		return
	}
	tmpPath := tmp.Name()

	fail := func(err error) {
		tmp.Close()
		os.Remove(tmpPath)
		rep.fail(err)
		result(false)
	}

	if !write(fileReply{Accept: true}) {
		fail(errors.New("could not tell the sender we accepted"))
		return
	}

	hasher := sha256.New()
	touch := func() { _ = s.SetReadDeadline(time.Now().Add(idleTimeout)) }
	rep.set(StatusActive, 0, "", "")
	if err := copyExactly(io.MultiWriter(tmp, hasher), s, offer.Size, touch, rep.bytes); err != nil {
		fail(fmt.Errorf("receiving data: %w", err))
		return
	}

	_ = s.SetReadDeadline(time.Now().Add(verifyTimeout))
	var trailer fileTrailer
	if err := readFrame(s, &trailer); err != nil {
		fail(fmt.Errorf("reading checksum: %w", err))
		return
	}
	if trailer.SHA256 != hex.EncodeToString(hasher.Sum(nil)) {
		fail(errors.New("file was corrupted in transit (checksum mismatch)"))
		return
	}
	if err := tmp.Close(); err != nil {
		fail(err)
		return
	}
	final, err := moveIntoPlace(tmpPath, f.cfg.Dir, name)
	if err != nil {
		fail(err)
		return
	}

	rep.set(StatusComplete, offer.Size, "", final)
	result(true)
}
