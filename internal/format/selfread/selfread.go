package selfread

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const (
	FooterSize = 32
	MagicSize  = 8
)

var Magic = [MagicSize]byte{'C', 'A', 'P', 'S', 'U', 'L', 'E', 0}

type Layout struct {
	BinConfigOffset int64
	BinConfigSize   int64
	SquashfsOffset  int64
	SquashfsSize    int64
}

func EncodeFooter(w io.Writer, binConfigSize, squashfsSize int64) error {
	var buf [FooterSize]byte
	copy(buf[0:MagicSize], Magic[:])
	binary.LittleEndian.PutUint64(buf[8:16], uint64(binConfigSize))
	binary.LittleEndian.PutUint64(buf[16:24], uint64(squashfsSize))
	_, err := w.Write(buf[:])
	return err
}

func ReadLayout(path string) (*Layout, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	total := st.Size()
	if total < FooterSize {
		return nil, fmt.Errorf("file too small: %d", total)
	}

	var buf [FooterSize]byte
	if _, err = f.ReadAt(buf[:], total-FooterSize); err != nil {
		return nil, err
	}
	for i := range MagicSize {
		if buf[i] != Magic[i] {
			return nil, fmt.Errorf("bad magic: %q", buf[:MagicSize])
		}
	}

	binConfigSize := int64(binary.LittleEndian.Uint64(buf[8:16]))
	squashfsSize := int64(binary.LittleEndian.Uint64(buf[16:24]))
	if binConfigSize < 0 || squashfsSize < 0 {
		return nil, fmt.Errorf("negative sizes: bin=%d sqfs=%d", binConfigSize, squashfsSize)
	}
	if binConfigSize+squashfsSize+FooterSize > total {
		return nil, fmt.Errorf("footer sizes exceed file")
	}

	squashfsOffset := total - FooterSize - squashfsSize
	return &Layout{
		BinConfigOffset: squashfsOffset - binConfigSize,
		BinConfigSize:   binConfigSize,
		SquashfsOffset:  squashfsOffset,
		SquashfsSize:    squashfsSize,
	}, nil
}

func ReadBinConfig(path string, layout *Layout) ([]byte, error) {
	if layout.BinConfigSize == 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, layout.BinConfigSize)
	_, err = f.ReadAt(buf, layout.BinConfigOffset)
	return buf, err
}

// SelfPath honours CAPSULE_SELF — /proc/self/exe points at /memfd:... when
// the caller execed us from memory.
func SelfPath() (string, error) {
	if env := os.Getenv("CAPSULE_SELF"); env != "" {
		return env, nil
	}
	return os.Readlink("/proc/self/exe")
}
