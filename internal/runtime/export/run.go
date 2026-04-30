package export

import "os/exec"

func tryRun(name string, args ...string) {
	_ = exec.Command(name, args...).Run() //nolint:gosec
}
