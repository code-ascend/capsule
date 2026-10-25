package exitcode

import (
	"syscall"
	"testing"
)

func TestFromStatus(t *testing.T) {
	cases := map[string]struct {
		ws   syscall.WaitStatus
		want int
	}{
		"exit 0":   {syscall.WaitStatus(0), 0},
		"exit 3":   {syscall.WaitStatus(3 << 8), 3},
		"sigterm":  {syscall.WaitStatus(syscall.SIGTERM), 143},
		"sigkill":  {syscall.WaitStatus(syscall.SIGKILL), 137},
		"exit 255": {syscall.WaitStatus(255 << 8), 255},
	}
	for name, c := range cases {
		if got := FromStatus(c.ws); got != c.want {
			t.Errorf("%s: FromStatus = %d, want %d", name, got, c.want)
		}
	}
}
