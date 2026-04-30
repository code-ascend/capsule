package hostexec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
)

type FrameType byte

const (
	FrameHello        FrameType = 0x01
	FrameStdin        FrameType = 0x02
	FrameStdinClose   FrameType = 0x03
	FrameStdout       FrameType = 0x04
	FrameStderr       FrameType = 0x05
	FrameSignal       FrameType = 0x06
	FrameExit         FrameType = 0x07
	FrameError        FrameType = 0x08
	FrameWindowResize FrameType = 0x09
)

const MaxFrameSize = 1 << 20

// Exit codes
const (
	ExitInternal    = 1   // I/O or spawn failure
	ExitUsage       = 2   // bad invocation
	ExitUnavailable = 127 // host_exec disabled
)

var ErrFrameTooLarge = errors.New("hostexec: frame exceeds MaxFrameSize")

// HelloRequest is the first frame sent by the client.
type HelloRequest struct {
	Argv []string          `json:"argv"`
	Env  map[string]string `json:"env"`
	Cwd  string            `json:"cwd"`
	Tty  bool              `json:"tty,omitempty"`
	Cols uint16            `json:"cols,omitempty"`
	Rows uint16            `json:"rows,omitempty"`
}

// WriteFrame writes one frame atomically. mu may be nil for single-writer use.
func WriteFrame(w io.Writer, mu *sync.Mutex, t FrameType, data []byte) error {
	if len(data) > MaxFrameSize {
		return ErrFrameTooLarge
	}
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	var hdr [5]byte
	hdr[0] = byte(t)
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// ReadFrame reads one frame from r. Returns io.EOF on clean close.
func ReadFrame(r io.Reader) (FrameType, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	t := FrameType(hdr[0])
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxFrameSize {
		return 0, nil, fmt.Errorf("hostexec: frame size %d exceeds max %d", n, MaxFrameSize)
	}
	if n == 0 {
		return t, nil, nil
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, nil, err
	}
	return t, data, nil
}
