package p2p

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/libp2p/go-libp2p/core/protocol"
)

const FileProtocolID protocol.ID = "/poc-filesend/1.0.0"

const (
	DefaultMaxFileSize int64 = 1 << 30  // 1 GiB
	chunkSize                = 64 << 10 // 64 KiB
	maxFrameBytes            = 4 << 10  // 4 KiB
	maxFileNameRunes         = 60
	frameTimeout             = 10 * time.Second
	decisionTimeout          = 2 * time.Minute
	idleTimeout              = 30 * time.Second
	verifyTimeout            = 30 * time.Second
)

const (
	reasonDeclined = "declined"
	reasonTooLarge = "too-large"
	reasonBusy     = "busy"
)

var (
	ErrRejected       = errors.New("the other person did not accept the file")
	ErrFileTooLarge   = errors.New("file is too large")
	ErrNotRegularFile = errors.New("not a regular file")
)

func rejectionError(reason string) error {
	switch reason {
	case reasonTooLarge:
		return fmt.Errorf("%w: it is larger than they accept", ErrRejected)
	case reasonBusy:
		return fmt.Errorf("%w: they have too many transfers in progress", ErrRejected)
	}
	return ErrRejected
}

type fileOffer struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type fileReply struct {
	Accept bool   `json:"accept"`
	Reason string `json:"reason,omitempty"`
}

type fileTrailer struct {
	SHA256 string `json:"sha256"` // hex
}

type fileResult struct {
	OK bool `json:"ok"`
}

// writes a single JSON frame with a length prefix.
func writeFrame(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(body) > maxFrameBytes {
		return fmt.Errorf("frame too large (%d bytes)", len(body))
	}
	buf := make([]byte, 4, 4+len(body))
	binary.BigEndian.PutUint32(buf, uint32(len(body)))
	buf = append(buf, body...)
	_, err = w.Write(buf)
	return err
}

// reads a single JSON frame with a length prefix.
func readFrame(r io.Reader, v any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > maxFrameBytes {
		return fmt.Errorf("frame size %d out of range", n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// copies exactly n bytes from src to dst in chunks, calling touch before each chunk and onBytes with the running total.
func copyExactly(dst io.Writer, src io.Reader, n int64, touch func(), onBytes func(done int64)) error {
	buf := make([]byte, chunkSize)
	var done int64
	for done < n {
		touch()
		want := int64(len(buf))
		if n-done < want {
			want = n - done
		}
		m, err := io.ReadFull(src, buf[:want])
		if m > 0 {
			if _, werr := dst.Write(buf[:m]); werr != nil {
				return werr
			}
			done += int64(m)
			onBytes(done)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// reserved Windows filenames that cannot be used as the base name of a file.
var windowsReserved = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// reduces an untrusted name to a safe bare filename.
func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " ")
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "file"
	}

	if utf8.RuneCountInString(name) > maxFileNameRunes {
		ext := filepath.Ext(name)
		if utf8.RuneCountInString(ext) > 16 {
			ext = ""
		}
		stem := []rune(strings.TrimSuffix(name, ext))
		name = string(stem[:maxFileNameRunes-utf8.RuneCountInString(ext)]) + ext
	}

	stem := name
	if i := strings.IndexByte(name, '.'); i >= 0 {
		stem = name[:i]
	}
	if windowsReserved[strings.ToUpper(stem)] {
		name = "_" + name
	}
	return name
}

// moves a temporary file into its final place, claiming a unique name in the target directory.
func moveIntoPlace(tmpPath, dir, name string) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 1000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		final := filepath.Join(dir, candidate)

		claim, err := os.OpenFile(final, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		claim.Close()

		if err := os.Rename(tmpPath, final); err != nil {
			os.Remove(final)
			return "", err
		}
		return final, nil
	}
	return "", errors.New("too many files with the same name")
}
