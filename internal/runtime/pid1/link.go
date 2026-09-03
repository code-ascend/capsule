package pid1

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// linkFD is where the sandbox end lands: the first exec.Cmd.ExtraFiles slot.
const linkFD = 3

// The link speaks fixed 2-byte frames from PID 1 to the host: [kind, data].
const (
	frameSize = 2
	// kindMainExited: the launched command is gone, orphans remain; data unused.
	kindMainExited = 'M'
	// kindExit: the last frame, data is the sandbox exit code.
	kindExit = 'E'
)

// Link is the host end of the socket shared with the sandbox's PID 1: half-close asks for shutdown, EOF means gone.
type Link struct {
	host  *net.UnixConn
	child *os.File
	stop  sync.Once
}

// NewLink creates the socket pair; pass Child to exec.Cmd.ExtraFiles and call CloseChild after Start.
func NewLink() (*Link, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("pid1 link: socketpair: %w", err)
	}
	hostFile := os.NewFile(uintptr(fds[0]), "pid1-link-host")
	child := os.NewFile(uintptr(fds[1]), "pid1-link")
	host, err := unixConn(hostFile)
	_ = hostFile.Close() // unixConn duplicated the descriptor
	if err != nil {
		_ = child.Close()
		return nil, fmt.Errorf("pid1 link: %w", err)
	}
	return &Link{host: host, child: child}, nil
}

// Child is the sandbox end for exec.Cmd.ExtraFiles.
func (l *Link) Child() *os.File { return l.child }

// CloseChild drops the host's copy of the sandbox end; until it is gone Wait can never see EOF.
func (l *Link) CloseChild() { _ = l.child.Close() }

// Stop asks PID 1 to shut the sandbox down; safe to call more than once.
func (l *Link) Stop() { l.stop.Do(func() { _ = l.host.CloseWrite() }) }

// Wait reads frames until the sandbox exits; ok is false when PID 1 never reported a code.
func (l *Link) Wait(onMainExited func()) (code int, ok bool) {
	var buf [frameSize]byte
	for {
		if _, err := io.ReadFull(l.host, buf[:]); err != nil {
			return 0, false
		}
		switch buf[0] {
		case kindMainExited:
			if onMainExited != nil {
				onMainExited()
			}
		case kindExit:
			return int(buf[1]), true
		}
	}
}

// writeFrame sends one PID 1 -> host frame on the sandbox end of the link.
func writeFrame(conn *net.UnixConn, kind, data byte) error {
	_, err := conn.Write([]byte{kind, data})
	return err
}

// Close releases whatever is still open.
func (l *Link) Close() {
	_ = l.host.Close()
	_ = l.child.Close()
}

// hostLink returns the inherited sandbox end, or nil when PID 1 was started without one.
func hostLink() *net.UnixConn {
	var st unix.Stat_t
	if err := unix.Fstat(linkFD, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFSOCK {
		return nil // not ours: an os.File finalizer would close a foreign descriptor
	}
	f := os.NewFile(linkFD, "pid1-link")
	defer func() { _ = f.Close() }() // the original must not leak into the app
	conn, err := unixConn(f)
	if err != nil {
		return nil
	}
	return conn
}

func unixConn(f *os.File) (*net.UnixConn, error) {
	conn, err := net.FileConn(f)
	if err != nil {
		return nil, err
	}
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("not a unix socket")
	}
	return uc, nil
}
