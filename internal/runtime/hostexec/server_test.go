package hostexec

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"capsule/internal/format/binconfig"
)

func TestServerSocketPathFormat(t *testing.T) {
	srv, err := Listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer srv.Close()
	want := "@capsule-host-exec-" + strconv.Itoa(os.Getpid()) + "-"
	if !strings.HasPrefix(srv.SocketPath(), want) {
		t.Errorf("socket path = %q, want prefix %q", srv.SocketPath(), want)
	}
}

func TestEndToEndEcho(t *testing.T) {
	if _, err := exec.LookPath("/bin/echo"); err != nil {
		t.Skip("/bin/echo unavailable")
	}
	srv, ctx, stop := startTestServer(t)
	defer stop()

	exitCode, stdout, stderr := runClientCapture(t, ctx, srv.SocketPath(), []string{"/bin/echo", "hi"})
	if exitCode != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", exitCode, stderr)
	}
	if strings.TrimSpace(stdout) != "hi" {
		t.Errorf("stdout = %q, want %q", stdout, "hi")
	}
}

func TestEndToEndStderrAndExit(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh unavailable")
	}
	srv, ctx, stop := startTestServer(t)
	defer stop()

	exitCode, _, stderr := runClientCapture(t, ctx, srv.SocketPath(),
		[]string{"/bin/sh", "-c", "echo err >&2; exit 3"})
	if exitCode != 3 {
		t.Errorf("exit = %d, want 3", exitCode)
	}
	if !strings.Contains(stderr, "err") {
		t.Errorf("stderr = %q, want to contain %q", stderr, "err")
	}
}

func TestEndToEndEnvForwarding(t *testing.T) {
	envBin, err := exec.LookPath("env")
	if err != nil {
		t.Skip("env unavailable")
	}
	t.Setenv("DISPLAY", ":42")
	srv, ctx, stop := startTestServer(t)
	defer stop()

	exitCode, stdout, stderr := runClientCapture(t, ctx, srv.SocketPath(), []string{envBin})
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr=%q", exitCode, stderr)
	}
	if !strings.Contains(stdout, "DISPLAY=:42") {
		t.Errorf("DISPLAY not forwarded: stdout=%q", stdout)
	}
}

func TestCancelKillsChild(t *testing.T) {
	if _, err := exec.LookPath("/bin/sleep"); err != nil {
		t.Skip("/bin/sleep unavailable")
	}
	srv, ctx, stop := startTestServer(t)
	defer stop()

	type result struct {
		code int
		dur  time.Duration
	}
	done := make(chan result, 1)
	go func() {
		start := time.Now()
		code, _, _ := runClientCapture(t, ctx, srv.SocketPath(), []string{"/bin/sleep", "30"})
		done <- result{code, time.Since(start)}
	}()

	time.Sleep(150 * time.Millisecond)
	stop()

	select {
	case r := <-done:
		if r.dur > 5*time.Second {
			t.Fatalf("client took %s — child likely not killed", r.dur)
		}
	case <-time.After(8 * time.Second):
		t.Fatalf("client did not return after stop()")
	}
}

func TestMergeHostEnvOverrides(t *testing.T) {
	t.Setenv("FOO_BASE", "base")
	t.Setenv("FOO_OVER", "old")

	merged := mergeHostEnv(map[string]string{"FOO_OVER": "new", "FOO_NEW": "x"})

	want := map[string]string{"FOO_BASE": "base", "FOO_OVER": "new", "FOO_NEW": "x"}
	got := make(map[string]string, len(merged))
	for _, kv := range merged {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestChooseCwdFallsBackOnInvalid(t *testing.T) {
	tmp := t.TempDir()
	if got := chooseCwd(tmp); got != tmp {
		t.Errorf("valid dir: got %q, want %q", got, tmp)
	}
	if got := chooseCwd("/nonexistent/" + t.Name()); got == "" {
		t.Errorf("invalid cwd should fall back, got empty")
	}
	if got := chooseCwd(""); got == "" {
		t.Errorf("empty cwd should fall back, got empty")
	}
}

func startTestServer(t *testing.T) (*Server, context.Context, func()) {
	t.Helper()
	srv, err := Listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Serve(ctx)
	return srv, ctx, func() {
		cancel()
		_ = srv.Close()
	}
}

func runClientCapture(t *testing.T, ctx context.Context, socket string, argv []string) (int, string, string) {
	t.Helper()
	t.Setenv(binconfig.HostExecSocketEnv, socket)

	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutW, stderrW
	defer func() {
		os.Stdout, os.Stderr = origOut, origErr
	}()

	var outBuf, errBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = outBuf.ReadFrom(stdoutR) }()
	go func() { defer wg.Done(); _, _ = errBuf.ReadFrom(stderrR) }()

	code := Run(ctx, argv)
	_ = stdoutW.Close()
	_ = stderrW.Close()
	wg.Wait()

	return code, outBuf.String(), errBuf.String()
}
