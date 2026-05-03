package hostexec

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"capsule/internal/format/binconfig"

	"golang.org/x/term"
)

// Run is the in-capsule client entrypoint (argv[0] == "capsule-host-exec").
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: capsule-host-exec <command> [args...]")
		return ExitUsage
	}

	sockPath := os.Getenv(binconfig.HostExecSocketEnv)
	if sockPath == "" {
		fmt.Fprintln(stderr, "capsule-host-exec: host_exec is not enabled in this capsule")
		return ExitUnavailable
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		fmt.Fprintf(stderr, "capsule-host-exec: dial %s: %v\n", sockPath, err)
		return ExitInternal
	}
	defer conn.Close()

	stdinFD, useTTY := detectTTY(stdin, stdout, args[0])

	req := buildHello(args, useTTY, stdinFD)
	payload, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(stderr, "capsule-host-exec: marshal hello: %v\n", err)
		return ExitInternal
	}

	var writeMu sync.Mutex
	if err := WriteFrame(conn, &writeMu, FrameHello, payload); err != nil {
		fmt.Fprintf(stderr, "capsule-host-exec: send hello: %v\n", err)
		return ExitInternal
	}

	clientCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if useTTY {
		restore, err := term.MakeRaw(stdinFD)
		if err == nil {
			defer term.Restore(stdinFD, restore)
		}
		go forwardWindowResize(clientCtx, conn, &writeMu, stdinFD)
	}

	go pumpStdin(clientCtx, conn, &writeMu, stdin)
	go forwardSignals(clientCtx, conn, &writeMu, useTTY)

	return readResponses(conn, stdout, stderr)
}

// detectTTY only treats real *os.File streams as candidates for TTY mode.
func detectTTY(stdin io.Reader, stdout io.Writer, cmd string) (int, bool) {
	sf, ok := stdin.(*os.File)
	if !ok {
		return 0, false
	}
	of, ok := stdout.(*os.File)
	if !ok {
		return int(sf.Fd()), false
	}
	fd := int(sf.Fd())
	return fd, term.IsTerminal(fd) && term.IsTerminal(int(of.Fd())) && !ptyBlocked(cmd)
}

func readResponses(conn net.Conn, stdout, stderr io.Writer) int {
	for {
		t, data, err := ReadFrame(conn)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				fmt.Fprintln(stderr, "capsule-host-exec: host runtime closed connection")
				return ExitInternal
			}
			fmt.Fprintf(stderr, "capsule-host-exec: read: %v\n", err)
			return ExitInternal
		}
		switch t {
		case FrameStdout:
			_, _ = stdout.Write(data)
		case FrameStderr:
			_, _ = stderr.Write(data)
		case FrameError:
			fmt.Fprintf(stderr, "capsule-host-exec: %s\n", data)
			return ExitInternal
		case FrameExit:
			if len(data) < 4 {
				return ExitInternal
			}
			return int(int32(binary.BigEndian.Uint32(data)))
		}
	}
}

func pumpStdin(ctx context.Context, conn net.Conn, mu *sync.Mutex, stdin io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := stdin.Read(buf)
		if n > 0 {
			if werr := WriteFrame(conn, mu, FrameStdin, buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			_ = WriteFrame(conn, mu, FrameStdinClose, nil)
			return
		}
	}
}

func forwardSignals(ctx context.Context, conn net.Conn, mu *sync.Mutex, ttyMode bool) {
	ch := make(chan os.Signal, 4)
	// Raw TTY delivers Ctrl-C/Ctrl-\ as bytes on stdin, so don't double-forward.
	if ttyMode {
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGHUP)
	} else {
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	}
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case s := <-ch:
			ssig, ok := s.(syscall.Signal)
			if !ok {
				continue
			}
			var payload [4]byte
			binary.BigEndian.PutUint32(payload[:], uint32(ssig))
			_ = WriteFrame(conn, mu, FrameSignal, payload[:])
		}
	}
}

func forwardWindowResize(ctx context.Context, conn net.Conn, mu *sync.Mutex, fd int) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	defer signal.Stop(ch)
	sendSize(conn, mu, fd)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			sendSize(conn, mu, fd)
		}
	}
}

func sendSize(conn net.Conn, mu *sync.Mutex, fd int) {
	w, h, err := term.GetSize(fd)
	if err != nil {
		return
	}
	var payload [4]byte
	binary.BigEndian.PutUint16(payload[0:2], uint16(w))
	binary.BigEndian.PutUint16(payload[2:4], uint16(h))
	_ = WriteFrame(conn, mu, FrameWindowResize, payload[:])
}

// ptyBlocked forces non-PTY mode
func ptyBlocked(cmd string) bool {
	return slices.Contains(binconfig.HostExecForwardedAliases, filepath.Base(cmd))
}

// forwardedEnvKeys are the session/GUI vars shipped to the host process.
var forwardedEnvKeys = []string{
	"DISPLAY",
	"WAYLAND_DISPLAY",
	"XDG_RUNTIME_DIR",
	"XDG_SESSION_TYPE",
	"DBUS_SESSION_BUS_ADDRESS",
	"XAUTHORITY",
	"PULSE_SERVER",
	"PULSE_COOKIE",
	"LANG",
	"TERM",
}

func buildHello(args []string, useTTY bool, stdinFD int) HelloRequest {
	env := make(map[string]string, len(forwardedEnvKeys)+4)
	for _, k := range forwardedEnvKeys {
		if v, ok := os.LookupEnv(k); ok {
			env[k] = v
		}
	}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "LC_") {
			eq := strings.IndexByte(kv, '=')
			if eq > 0 {
				env[kv[:eq]] = kv[eq+1:]
			}
		}
	}
	cwd, _ := os.Getwd()
	req := HelloRequest{Argv: args, Env: env, Cwd: cwd}
	if useTTY {
		req.Tty = true
		if w, h, err := term.GetSize(stdinFD); err == nil {
			req.Cols = uint16(w)
			req.Rows = uint16(h)
		}
	}
	return req
}
