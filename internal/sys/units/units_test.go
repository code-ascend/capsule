package units

import "testing"

func TestBytes(t *testing.T) {
	cases := map[int64]string{
		0:          "0 B",
		999:        "999 B",
		1000:       "1.0 kB",
		617272235:  "617.3 MB",
		1071031290: "1.1 GB",
		1e12:       "1.0 TB",
		1e15:       "1000.0 TB",
	}
	for n, want := range cases {
		if got := Bytes(n); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", n, got, want)
		}
	}
}
