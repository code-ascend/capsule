package hostexec

import (
	"bytes"
	"errors"
	"testing"
)

func TestFrameRoundtrip(t *testing.T) {
	cases := []struct {
		name string
		t    FrameType
		data []byte
	}{
		{"empty", FrameStdinClose, nil},
		{"hello-payload", FrameHello, []byte(`{"argv":["/bin/echo","hi"]}`)},
		{"binary", FrameStdout, []byte{0x00, 0xff, 0x10, 0x20, 0x80}},
		{"stderr-text", FrameStderr, []byte("error: thing failed\n")},
		{"signal-payload", FrameSignal, []byte{0x00, 0x00, 0x00, 0x0f}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteFrame(&buf, nil, c.t, c.data); err != nil {
				t.Fatalf("write: %v", err)
			}
			gotT, gotData, err := ReadFrame(&buf)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if gotT != c.t {
				t.Errorf("type = %d, want %d", gotT, c.t)
			}
			if !bytes.Equal(gotData, c.data) {
				t.Errorf("data = %x, want %x", gotData, c.data)
			}
		})
	}
}

func TestWriteFrameTooLarge(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFrame(&buf, nil, FrameStdout, make([]byte, MaxFrameSize+1))
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("oversize write must not emit bytes; got %d", buf.Len())
	}
}

func TestReadFrameRejectsOversizeHeader(t *testing.T) {
	// Header advertises payload bigger than MaxFrameSize — must fail before reading.
	hdr := []byte{byte(FrameStdout), 0x10, 0x00, 0x00, 0x01}
	_, _, err := ReadFrame(bytes.NewReader(hdr))
	if err == nil {
		t.Fatal("expected error for oversize advertised length")
	}
}

func TestReadFrameTruncated(t *testing.T) {
	// Header says payload is 4 bytes but only 2 follow.
	hdr := []byte{byte(FrameStdin), 0x00, 0x00, 0x00, 0x04, 0x01, 0x02}
	_, _, err := ReadFrame(bytes.NewReader(hdr))
	if err == nil {
		t.Fatal("expected error on truncated payload")
	}
}
