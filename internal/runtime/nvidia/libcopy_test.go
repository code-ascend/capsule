package nvidia

import "testing"

func TestShouldSkipVersionedLib(t *testing.T) {
	const version = "535.183.01"
	cases := []struct {
		fname string
		want  bool
	}{
		{"libnvidia-ml.so", false},                // soname symlink — keep
		{"libnvidia-ml.so.1", false},              // soname major — keep
		{"libnvidia-ml.so.535.183.01", false},     // matches host driver — keep
		{"libnvidia-ml.so.530.30.02", true},       // other driver — skip
		{"libcuda.so.1", false},                   // soname major — keep
		{"libcuda.so.535.183.01", false},          // matches — keep
		{"libfoo.so", false},                      // not versioned — keep
		{"libnvidia-ml.so.tls", false},            // suffix has letters — keep
		{"libnvidia-ml.so.535", false},            // single dot only treated as version → not full
		{"libnvidia-encode.so.530", false},        // single number — not full version
		{"libnvidia-encode.so.530.30.02.5", true}, // 4-part version, mismatch
	}
	for _, c := range cases {
		if got := shouldSkipVersionedLib(c.fname, version); got != c.want {
			t.Errorf("shouldSkipVersionedLib(%q, %q) = %v, want %v",
				c.fname, version, got, c.want)
		}
	}
}

func TestLooksLikeFullVersion(t *testing.T) {
	cases := map[string]bool{
		"535.183.01": true,
		"1.0":        true,
		"530":        false, // no dots
		"":           false,
		"tls":        false,
		"535.x":      false,
		"1.2.3.4":    true,
	}
	for in, want := range cases {
		if got := looksLikeFullVersion(in); got != want {
			t.Errorf("looksLikeFullVersion(%q) = %v, want %v", in, got, want)
		}
	}
}
