package selfread

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripFooter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capsule")

	header := bytes.Repeat([]byte{0xAA}, 4096)
	binconfig := []byte(`{"compression":"zstd"}`)
	squashfs := bytes.Repeat([]byte{0xBB}, 8192)

	var buf bytes.Buffer
	buf.Write(header)
	buf.Write(binconfig)
	buf.Write(squashfs)
	if err := EncodeFooter(&buf, int64(len(binconfig)), int64(len(squashfs))); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}

	layout, err := ReadLayout(path)
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if layout.BinConfigSize != int64(len(binconfig)) {
		t.Errorf("binconfig size: got %d, want %d", layout.BinConfigSize, len(binconfig))
	}
	if layout.SquashfsSize != int64(len(squashfs)) {
		t.Errorf("squashfs size: got %d, want %d", layout.SquashfsSize, len(squashfs))
	}
	if layout.BinConfigOffset != int64(len(header)) {
		t.Errorf("binconfig offset: got %d, want %d", layout.BinConfigOffset, len(header))
	}
	if layout.SquashfsOffset != int64(len(header)+len(binconfig)) {
		t.Errorf("squashfs offset: got %d, want %d", layout.SquashfsOffset, len(header)+len(binconfig))
	}

	got, err := ReadBinConfig(path, layout)
	if err != nil {
		t.Fatalf("read binconfig: %v", err)
	}
	if !bytes.Equal(got, binconfig) {
		t.Errorf("binconfig content mismatch")
	}
}

func TestBadMagic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad")

	junk := bytes.Repeat([]byte{0}, 4096)
	if err := os.WriteFile(path, junk, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadLayout(path); err == nil {
		t.Fatalf("expected error on bad magic")
	}
}

func TestIsCapsule(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good")
	header := bytes.Repeat([]byte{0xAA}, 4096)
	binconfig := []byte(`{"compression":"zstd"}`)
	squashfs := bytes.Repeat([]byte{0xBB}, 8192)
	var buf bytes.Buffer
	buf.Write(header)
	buf.Write(binconfig)
	buf.Write(squashfs)
	if err := EncodeFooter(&buf, int64(len(binconfig)), int64(len(squashfs))); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(good, buf.Bytes(), 0755); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !IsCapsule(good) {
		t.Errorf("IsCapsule(valid) = false, want true")
	}

	random := filepath.Join(dir, "random")
	if err := os.WriteFile(random, bytes.Repeat([]byte{0x11}, 4096), 0644); err != nil {
		t.Fatalf("write random: %v", err)
	}
	if IsCapsule(random) {
		t.Errorf("IsCapsule(random) = true, want false")
	}

	tiny := filepath.Join(dir, "tiny")
	if err := os.WriteFile(tiny, []byte{1, 2, 3}, 0644); err != nil {
		t.Fatalf("write tiny: %v", err)
	}
	if IsCapsule(tiny) {
		t.Errorf("IsCapsule(tiny) = true, want false")
	}

	if IsCapsule(filepath.Join(dir, "missing")) {
		t.Errorf("IsCapsule(missing) = true, want false")
	}
}

func TestTooSmall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny")
	if err := os.WriteFile(path, []byte{1, 2, 3}, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadLayout(path); err == nil {
		t.Fatalf("expected error on tiny file")
	}
}
