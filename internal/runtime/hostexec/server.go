package hostexec

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"capsule/internal/sys/log"

	"github.com/creack/pty"
)

// Server hosts the abstract UNIX listener that capsule-host-exec clients dial.
type Server struct {
	ln       net.Listener
	sockPath string
}

// Listen binds an abstract-namespace socket unique to this capsule run.
func Listen() (*Server, error) {
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return nil, fmt.Errorf("hostexec: rand: %w", err)
	}
	name := "@capsule-host-exec-" + strconv.Itoa(os.Getpid()) + "-" + hex.EncodeToString(rnd[:])
	ln, err := net.Listen("unix", name)
	if err != nil {
		return nil, fmt.Errorf("hostexec: listen %s: %w", name, err)
	}
	return &Server{ln: ln, sockPath: name}, nil
}

// SocketPath returns the abstract socket name for clients.
func (s *Server) SocketPath() string { return s.sockPath }

// Close stops accepting new connections.
func (s *Server) Close() error { return s.ln.Close() }

// Serve accepts connections until ctx is cancelled or Close is called.
func (s *Server) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = s.ln.Close()
	}()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Debug("hostexec accept failed", "error", err)
			continue
		}
		go handleConn(ctx, conn)
	}
}

func handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	if err := checkPeerUID(conn); err != nil {
		log.Debug("hostexec peer rejected", "error", err)
		_ = WriteFrame(conn, nil, FrameError, []byte(err.Error()))
		return
	}

	t, data, err := ReadFrame(conn)
	if err != nil {
		log.Debug("hostexec read hello", "error", err)
		return
	}
	if t != FrameHello {
		_ = WriteFrame(conn, nil, FrameError, []byte("expected hello frame"))
		return
	}
	var req HelloRequest
	if err := json.Unmarshal(data, &req); err != nil {
		_ = WriteFrame(conn, nil, FrameError, []byte("hello decode: "+err.Error()))
		return
	}
	if len(req.Argv) == 0 || req.Argv[0] == "" {
		_ = WriteFrame(conn, nil, FrameError, []byte("empty argv"))
		return
	}

	runChild(ctx, conn, &req)
}

func runChild(ctx context.Context, conn net.Conn, req *HelloRequest) {
	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, req.Argv[0], req.Argv[1:]...)
	cmd.Env = mergeHostEnv(req.Env)
	cmd.Dir = chooseCwd(req.Cwd)

	if req.Tty {
		runChildPTY(conn, cmd, req, cancel)
		return
	}
	runChildPipes(conn, cmd, cancel)
}

func runChildPipes(conn net.Conn, cmd *exec.Cmd, cancel context.CancelFunc) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = WriteFrame(conn, nil, FrameError, []byte("stdin pipe: "+err.Error()))
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = WriteFrame(conn, nil, FrameError, []byte("stdout pipe: "+err.Error()))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = WriteFrame(conn, nil, FrameError, []byte("stderr pipe: "+err.Error()))
		return
	}

	if err := cmd.Start(); err != nil {
		_ = WriteFrame(conn, nil, FrameError, []byte("start: "+err.Error()))
		return
	}

	var writeMu sync.Mutex
	var ioWG sync.WaitGroup
	ioWG.Add(2)
	go pumpReader(&writeMu, conn, stdout, FrameStdout, &ioWG)
	go pumpReader(&writeMu, conn, stderr, FrameStderr, &ioWG)

	go readClientFrames(conn, cmd, stdin, nil, cancel)

	ioWG.Wait()
	werr := cmd.Wait()

	writeExit(conn, &writeMu, exitCodeFor(werr))
}

func runChildPTY(conn net.Conn, cmd *exec.Cmd, req *HelloRequest, cancel context.CancelFunc) {
	winsize := &pty.Winsize{Cols: req.Cols, Rows: req.Rows}
	if winsize.Cols == 0 {
		winsize.Cols = 80
	}
	if winsize.Rows == 0 {
		winsize.Rows = 24
	}
	ptmx, err := pty.StartWithSize(cmd, winsize)
	if err != nil {
		_ = WriteFrame(conn, nil, FrameError, []byte("pty start: "+err.Error()))
		return
	}
	defer ptmx.Close()

	var writeMu sync.Mutex
	var ioWG sync.WaitGroup
	ioWG.Add(1)
	go pumpReader(&writeMu, conn, ptmx, FrameStdout, &ioWG)

	go readClientFrames(conn, cmd, ptmx, ptmx, cancel)

	werr := cmd.Wait()
	ioWG.Wait()
	_ = ptmx.Close()

	writeExit(conn, &writeMu, exitCodeFor(werr))
}

func writeExit(conn net.Conn, mu *sync.Mutex, code int) {
	var payload [4]byte
	binary.BigEndian.PutUint32(payload[:], uint32(code))
	_ = WriteFrame(conn, mu, FrameExit, payload[:])
}

func pumpReader(mu *sync.Mutex, w io.Writer, r io.Reader, t FrameType, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if werr := WriteFrame(w, mu, t, buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// readClientFrames decodes inbound frames; ptyFile is non-nil only in TTY mode.
func readClientFrames(conn net.Conn, cmd *exec.Cmd, stdin io.WriteCloser, ptyFile *os.File, cancel context.CancelFunc) {
	defer func() {
		// Client dropped before child finished: SIGTERM, then SIGKILL after 2s.
		if cmd.ProcessState == nil && cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			time.AfterFunc(2*time.Second, func() {
				if cmd.ProcessState == nil {
					_ = cmd.Process.Kill()
				}
			})
		}
		cancel()
	}()
	for {
		t, data, err := ReadFrame(conn)
		if err != nil {
			_ = stdin.Close()
			return
		}
		switch t {
		case FrameStdin:
			if _, werr := stdin.Write(data); werr != nil {
				return
			}
		case FrameStdinClose:
			_ = stdin.Close()
		case FrameSignal:
			if len(data) >= 4 && cmd.Process != nil {
				sig := syscall.Signal(int32(binary.BigEndian.Uint32(data)))
				_ = cmd.Process.Signal(sig)
			}
		case FrameWindowResize:
			if ptyFile != nil && len(data) >= 4 {
				cols := binary.BigEndian.Uint16(data[0:2])
				rows := binary.BigEndian.Uint16(data[2:4])
				_ = pty.Setsize(ptyFile, &pty.Winsize{Cols: cols, Rows: rows})
			}
		}
	}
}

// checkPeerUID rejects connections from a different UID than the runtime's.
func checkPeerUID(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("hostexec: not a unix conn")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	var ucred *syscall.Ucred
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		ucred, sockErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if sockErr != nil {
		return sockErr
	}
	if int(ucred.Uid) != os.Getuid() {
		return fmt.Errorf("hostexec: peer uid %d != server uid %d", ucred.Uid, os.Getuid())
	}
	return nil
}

func chooseCwd(cwd string) string {
	if cwd != "" {
		if st, err := os.Stat(cwd); err == nil && st.IsDir() {
			return cwd
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "/"
}

// mergeHostEnv overlays forwarded entries onto the runtime's own env.
func mergeHostEnv(forwarded map[string]string) []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+len(forwarded))
	for _, kv := range base {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			out = append(out, kv)
			continue
		}
		if _, override := forwarded[key]; override {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range forwarded {
		out = append(out, k+"="+v)
	}
	return out
}

func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return ExitInternal
}
