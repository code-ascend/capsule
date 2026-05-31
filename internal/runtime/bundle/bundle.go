package bundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed files/utils.tar.gz
var tarGz []byte

type Extractor struct {
	Dir string
}

func New(dir string) *Extractor { return &Extractor{Dir: dir} }

func (e *Extractor) Extract() error {
	marker := filepath.Join(e.Dir, ".extracted")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	if err := os.MkdirAll(e.Dir, 0755); err != nil {
		return err
	}

	gz, err := gzip.NewReader(bytes.NewReader(tarGz))
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(hdr.Name, "utils/")
		if name == "" {
			continue
		}
		dest := filepath.Join(e.Dir, name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			_ = os.Remove(dest)
			if err := os.Symlink(hdr.Linkname, dest); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			if err := writeReg(dest, tr); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(marker, nil, 0644)
}

func writeReg(dest string, r io.Reader) (err error) {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(f, r)
	return err
}

func (e *Extractor) Bin(name string) string { return filepath.Join(e.Dir, name) }

func (e *Extractor) HasBin(name string) bool {
	st, err := os.Stat(e.Bin(name))
	return err == nil && st.Mode()&0111 != 0
}

func (e *Extractor) Loader() string {
	return filepath.Join(e.Dir, "ld-linux-x86-64.so.2")
}

// Command runs a bundled binary via bundled ld-linux so it picks up our libs.
func (e *Extractor) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	full := append([]string{"--library-path", e.Dir, e.Bin(name)}, args...)
	return exec.CommandContext(ctx, e.Loader(), full...)
}
