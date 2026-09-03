package bwrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"capsule/internal/format/binconfig"
	"capsule/internal/runtime/bundle"
	"capsule/internal/sys/fsutil"
	"capsule/internal/sys/interrupt"
	"capsule/internal/sys/log"

	"golang.org/x/sys/unix"
)

type Spec struct {
	RootPath      string
	RootWritable  bool
	MergedUserDir string
	Cmd           []string
	Cfg           *binconfig.Config
	Env           Env

	// Sandbox selects the isolation level: host mounts and PID/network namespaces
	Sandbox binconfig.Sandbox

	// Binds is a list of "src:dst" mounts from the manifest and --bind CLI flags
	Binds []string

	// StartScript runs before the main command on every start; empty disables the wrapper.
	StartScript string

	// EnvSet holds "KEY=VAL" overrides from --env, applied after Cfg.EnvSet so CLI wins.
	EnvSet []string

	// EnvUnset holds keys to drop from --unsetenv, applied after EnvSet so unset wins.
	EnvUnset []string

	// SelfPath is the runtime ELF, bound into the sandbox as the host-exec client when HostExecSocket is set.
	SelfPath string

	// HostExecSocket enables host-exec: the client aliases are bound in and the socket exported.
	HostExecSocket string
}

// Env carries host-side variables that shape bwrap args.
type Env struct {
	Home          string
	CapsuleHome   string
	User          string
	Term          string
	Lang          string
	XDGDataDirs   string
	XDGRuntimeDir string
}

func EnvFromOS() Env {
	return Env{
		Home:          os.Getenv("HOME"),
		User:          os.Getenv("USER"),
		Term:          os.Getenv("TERM"),
		Lang:          os.Getenv("LANG"),
		XDGDataDirs:   os.Getenv("XDG_DATA_DIRS"),
		XDGRuntimeDir: xdgRuntimeDir(),
	}
}

// xdgRuntimeDir returns the per-user runtime dir, falling back to the conventional /run/user/UID.
func xdgRuntimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return "/run/user/" + strconv.Itoa(os.Getuid())
}

func (s *Spec) Build() []string {
	cmd := s.resolveCmd()

	var args []string
	args = append(args, s.namespaceArgs()...)
	args = append(args, s.rootBind()...)
	args = append(args,
		"--dev-bind", "/dev", "/dev",
		"--ro-bind", "/sys", "/sys",
		"--proc", "/proc",
	)
	for _, p := range []string{"/tmp", "/var/tmp"} {
		args = append(args, "--bind-try", p, p)
	}
	args = append(args, s.mediaMounts()...)
	args = append(args, s.runArgs()...)
	args = append(args, s.hostEtcBinds()...)
	args = append(args, s.mergedUserBinds()...)
	args = append(args, s.Env.homeBinds(s.RootWritable)...)
	args = append(args, s.bindArgs()...)
	args = append(args, s.Env.defaults()...)
	args = append(args, s.configEnv()...)
	args = append(args, s.cliEnv()...)
	args = append(args, s.hostExecBinds()...)
	args = append(args, "--")
	args = append(args, cmd...)
	return args
}

// namespaceArgs unshares the PID namespace (isolated, strict) and the network (strict); shared keeps both host namespaces.
func (s *Spec) namespaceArgs() []string {
	var args []string
	if s.isolated() {
		args = append(args, "--unshare-pid")
	}
	if s.Sandbox == binconfig.SandboxStrict {
		args = append(args, "--unshare-net")
	}
	return args
}

// mediaMounts binds host /mnt and /media (shared) or hides them behind tmpfs (isolated, strict).
func (s *Spec) mediaMounts() []string {
	if s.isolated() {
		return []string{"--tmpfs", "/mnt", "--tmpfs", "/media"}
	}
	return []string{"--bind-try", "/mnt", "/mnt", "--bind-try", "/media", "/media"}
}

// runArgs binds the host /run (shared) or a writable tmpfs with only the user/dbus sockets (isolated, strict).
func (s *Spec) runArgs() []string {
	if !s.isolated() {
		return []string{"--bind-try", "/run", "/run"}
	}
	args := []string{"--tmpfs", "/run"}
	if rt := s.Env.XDGRuntimeDir; rt != "" {
		args = append(args, "--bind-try", rt, rt)
	}
	args = append(args, "--bind-try", "/run/dbus", "/run/dbus")
	return args
}

// hostExecBinds mounts the runtime ELF as the host-exec client aliases and exports the socket; nothing when host-exec is off.
func (s *Spec) hostExecBinds() []string {
	if s.HostExecSocket == "" {
		return nil
	}
	names := append([]string{binconfig.HostExecCommand}, binconfig.HostExecForwardedAliases...)
	args := make([]string, 0, 3*len(names)+4)
	if !s.RootWritable {
		args = append(args, "--tmpfs", "/usr/local/bin")
	}
	for _, name := range names {
		args = append(args, "--ro-bind", s.SelfPath, "/usr/local/bin/"+name)
	}
	return append(args, "--setenv", binconfig.HostExecSocketEnv, s.HostExecSocket)
}

// Run launches the sandbox under the supervisor and blocks until its last process exits; cancelling ctx asks for a shutdown.
func (s *Spec) Run(ctx context.Context, b *bundle.Extractor) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	defer interrupt.Lend()()

	args := s.Build()
	// Cancellation goes to the supervisor as SIGTERM, not through the launcher context, so the sandbox can stop gracefully.
	cmd := supervisorCommand(b.Command(context.WithoutCancel(ctx), "bwrap", args...))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if log.IsDebug() {
		log.Debug("bwrap exec", "args", strings.Join(args, " "))
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	defer context.AfterFunc(ctx, func() { _ = cmd.Process.Signal(unix.SIGTERM) })()

	// The supervisor exits with the sandbox's code once every sandbox process is gone.
	err := cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), nil
		}
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

// supervisorCommand re-execs this binary under its supervisor name with bwrap's full command line as arguments.
func supervisorCommand(bw *exec.Cmd) *exec.Cmd {
	return &exec.Cmd{
		Path: "/proc/self/exe",
		Args: append([]string{binconfig.SupervisorCommand, bw.Path}, bw.Args[1:]...),
	}
}

// resolveCmd picks the command to run and wraps it with the on_start script when set.
func (s *Spec) resolveCmd() []string {
	cmd := s.baseCmd()
	if s.StartScript == "" {
		return cmd
	}
	wrapper := "set -e\n" + s.StartScript + "\nexec \"$@\""
	return append([]string{"/bin/bash", "-c", wrapper, "capsule-start"}, cmd...)
}

// baseCmd applies precedence: explicit command > configured launch > shell.
func (s *Spec) baseCmd() []string {
	if len(s.Cmd) > 0 {
		return s.Cmd
	}
	if s.Cfg != nil && s.Cfg.Launch != "" {
		return strings.Fields(s.Cfg.Launch)
	}
	return []string{"/bin/bash"}
}

func (s *Spec) rootBind() []string {
	flag := "--ro-bind"
	if s.RootWritable {
		flag = "--bind"
	}
	return []string{flag, s.RootPath, "/"}
}

// hostEtcBinds binds host /etc network/locale files into the capsule
func (s *Spec) hostEtcBinds() []string {
	files := []string{"resolv.conf", "hosts", "nsswitch.conf", "localtime", "machine-id", "asound.conf"}
	var args []string
	for _, f := range files {
		if !etcTargetUsable(s.RootPath, f) {
			continue
		}
		args = append(args, "--ro-bind-try", filepath.Join("/etc", f), "/etc/"+f)
	}
	return args
}

// etcTargetUsable reports whether root/etc/name is a regular file bwrap can bind onto;
// symlink dests are skipped: bwrap resolves them against the host and fails.
func etcTargetUsable(root, name string) bool {
	fi, err := os.Lstat(filepath.Join(root, "etc", name))
	return err == nil && fi.Mode()&os.ModeSymlink == 0
}

func (s *Spec) mergedUserBinds() []string {
	if s.MergedUserDir == "" {
		return nil
	}
	var args []string
	for _, f := range []string{"passwd", "group", "shadow"} {
		merged := filepath.Join(s.MergedUserDir, f)
		container := filepath.Join(s.RootPath, "etc", f)
		if fsutil.Exists(merged) && fsutil.Exists(container) {
			args = append(args, "--ro-bind", merged, "/etc/"+f)
		}
	}
	return args
}

// homeBinds binds the host home into the capsule both at /home/$USER and at its host path.
func (e Env) homeBinds(rootWritable bool) []string {
	home := e.CapsuleHome
	if home == "" {
		home = e.Home
	}
	if home == "" || !fsutil.IsDir(home) {
		return nil
	}
	if topComponent(home) == "/home" {
		return []string{"--bind-try", "/home", "/home"}
	}
	user := e.User
	if user == "" {
		user = "user"
	}
	containerHome := "/home/" + user
	args := []string{
		"--tmpfs", "/home",
		"--dir", containerHome,
		"--bind", home, containerHome,
	}
	if rootWritable {
		args = append(args, parentDirArgs(home)...)
	} else {
		args = append(args, "--tmpfs", filepath.Dir(home))
	}
	args = append(args,
		"--bind", home, home,
		"--setenv", "HOME", containerHome,
		"--setenv", "XDG_CONFIG_HOME", containerHome+"/.config",
		"--setenv", "XDG_DATA_HOME", containerHome+"/.local/share",
	)
	return args
}

// parentDirArgs emits --dir for each ancestor of path, shallowest first.
func parentDirArgs(path string) []string {
	var parents []string
	for d := filepath.Dir(path); d != "/" && d != "." && d != ""; d = filepath.Dir(d) {
		parents = append([]string{d}, parents...)
	}
	args := make([]string, 0, len(parents)*2)
	for _, d := range parents {
		args = append(args, "--dir", d)
	}
	return args
}

// cliEnv emits --setenv then --unsetenv from CLI flags, so unset wins on overlap.
func (s *Spec) cliEnv() []string {
	var args []string
	for _, e := range s.EnvSet {
		k, v, ok := strings.Cut(e, "=")
		if !ok || k == "" {
			continue
		}
		args = append(args, "--setenv", k, v)
	}
	for _, k := range s.EnvUnset {
		if k == "" {
			continue
		}
		args = append(args, "--unsetenv", k)
	}
	return args
}

// splitBind parses "SRC[:DST]"; ok is false for a blank entry.
func splitBind(entry string) (src, dst string, ok bool) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", "", false
	}
	src, dst, found := strings.Cut(entry, ":")
	if !found || dst == "" {
		dst = src
	}
	return src, dst, true
}

// PrepareBinds expands env vars and ~ in baked "SRC[:DST]" entries and creates missing source dirs.
func PrepareBinds(entries []string) ([]string, error) {
	var out []string
	for _, entry := range entries {
		src, dst, ok := splitBind(os.ExpandEnv(entry))
		if !ok {
			continue
		}
		src = fsutil.ExpandHome(src)
		if src == "" {
			continue
		}
		if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(src, 0o755); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		out = append(out, src+":"+dst)
	}
	return out, nil
}

// bindArgs emits --bind for each "src:dst" in Spec.Binds.
func (s *Spec) bindArgs() []string {
	var args []string
	for _, entry := range s.Binds {
		if src, dst, ok := splitBind(entry); ok {
			args = append(args, "--bind", src, dst)
		}
	}
	return args
}

func (e Env) defaults() []string {
	term := e.Term
	if term == "" {
		term = "xterm"
	}
	lang := e.Lang
	if lang == "" {
		lang = "C.UTF-8"
	}
	xdgDirs := "/usr/local/share:/usr/share"
	if e.XDGDataDirs != "" {
		xdgDirs += ":" + e.XDGDataDirs
	}
	return []string{
		"--setenv", "PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"--setenv", "TERM", term,
		"--setenv", "LANG", lang,
		"--setenv", "XDG_DATA_DIRS", xdgDirs,
		"--setenv", binconfig.InsideEnv, "1",
	}
}

func (s *Spec) isolated() bool {
	return s.Sandbox == binconfig.SandboxIsolated || s.Sandbox == binconfig.SandboxStrict
}

// configEnv emits unsetenv/setenv args from the baked-in config (YAML).
func (s *Spec) configEnv() []string {
	if s.Cfg == nil {
		return nil
	}
	var args []string
	for _, k := range s.Cfg.EnvUnset {
		args = append(args, "--unsetenv", k)
	}
	keys := make([]string, 0, len(s.Cfg.EnvSet))
	for k := range s.Cfg.EnvSet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--setenv", k, s.Cfg.EnvSet[k])
	}
	return args
}

func topComponent(p string) string {
	p = filepath.Clean(p)
	if !strings.HasPrefix(p, "/") {
		return p
	}
	if i := strings.Index(p[1:], "/"); i >= 0 {
		return p[:i+1]
	}
	return p
}
