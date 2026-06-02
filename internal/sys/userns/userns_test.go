package userns

import "testing"

func TestHasSubIDEntry(t *testing.T) {
	data := []byte("# comment\nroot:100000:65536\ndm:165536:65536\n\n1001:231072:65536\n")
	cases := []struct {
		name, uid string
		want      bool
	}{
		{"dm", "1000", true},      // by username
		{"someone", "1001", true}, // by uid
		{"root", "0", true},       // by username
		{"absent", "9999", false}, // neither
	}
	for _, c := range cases {
		if got := hasSubIDEntry(data, c.name, c.uid); got != c.want {
			t.Errorf("hasSubIDEntry(%q,%q) = %v, want %v", c.name, c.uid, got, c.want)
		}
	}
}

func TestHasSubIDEntryEmpty(t *testing.T) {
	if hasSubIDEntry(nil, "dm", "1000") {
		t.Fatal("empty file must not match")
	}
}

func TestIsALTRelease(t *testing.T) {
	cases := map[string]bool{
		"NAME=\"ALT\"\nID=altlinux\nVERSION_ID=20260316\n":                            true, // ALT Regular
		"NAME=ALT\nID=altlinux\nVERSION_CODENAME=prometheus\n":                        true, // ALT Workstation
		"NAME=\"ALT Atomic\"\nID=\"alt-atomic-onyx-nightly\"\nID_LIKE=\"altlinux\"\n": true, // ALT Atomic
		"NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\n":                                false,
		"ID=\"fedora\"\n": false,
		"NAME=NoID\n":     false,
	}
	for in, want := range cases {
		if got := isALTRelease([]byte(in)); got != want {
			t.Errorf("isALTRelease(%q) = %v, want %v", in, got, want)
		}
	}
}
