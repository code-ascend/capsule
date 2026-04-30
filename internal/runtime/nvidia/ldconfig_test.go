package nvidia

import "testing"

const sampleLdConfigOutput = `1234 libs found in cache '/etc/ld.so.cache'
	libnvidia-glcore.so.550.78.01 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libnvidia-glcore.so.550.78.01
	libcuda.so.1 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libcuda.so.1
	libcuda.so.1 (libc6) => /usr/lib/i386-linux-gnu/libcuda.so.1
	libnvidia-ml.so.1 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1
	libnotify.so.4 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libnotify.so.4
	someweirdline-without-arrow
	libGL.so.1 (libc6,x86-64) => /usr/lib/x86_64-linux-gnu/libGL.so.1
`

func TestParseLdConfigBasic(t *testing.T) {
	entries := ParseLdConfig([]byte(sampleLdConfigOutput))
	if len(entries) != 6 {
		t.Fatalf("expected 6 entries, got %d", len(entries))
	}
	if entries[0].Soname != "libnvidia-glcore.so.550.78.01" {
		t.Errorf("soname = %q", entries[0].Soname)
	}
	if !entries[0].Is64Bit() {
		t.Error("first entry should be 64-bit")
	}
	if entries[2].Is64Bit() {
		t.Errorf("entry 2 (i386 libcuda) should be 32-bit, tag=%q", entries[2].Tag)
	}
}

func TestParseLdConfigSkipsHeader(t *testing.T) {
	entries := ParseLdConfig([]byte(sampleLdConfigOutput))
	for _, e := range entries {
		if e.Soname == "1234" {
			t.Fatalf("header leaked into entries")
		}
	}
}

func TestIs64BitDefaultsToTrue(t *testing.T) {
	e := LdEntry{Tag: ""}
	if !e.Is64Bit() {
		t.Error("empty tag should default to 64-bit")
	}
}
