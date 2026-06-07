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
	"time"

	"capsule/internal/format/binconfig"
	"capsule/internal/runtime/bundle"
	"capsule/internal/runtime/reaper"
	"capsule/internal/sys/fsutil"
	"capsule/internal/sys/log"
)

const reaperGracePeriod = 5 * time.Second

type Spec struct {
	RootPath      string
	RootWritable  bool
	MergedUserDir string
	Cmd           []string
	Cfg           *binconfig.Config
	Env           Env

	// Sandbox selects the isolation level: host mounts and PID/network namespaces
	Sandbox binconfig.Sandbox

	// Binds is a list of "src:dst" mounts from --bind CLI flags
	Binds []string

	// EnvSet holds "KEY=VAL" overrides from --env, applied after Cfg.EnvSet so CLI wins.
	EnvSet []string

	// EnvUnset holds keys to drop from --unsetenv, applied after EnvSet so unset wins.
	EnvUnset []string

	// Both empty disables host-exec; otherwise the ELF is bound in and the socket exported.
	HostExecSocket  string
	HostExecBinPath string
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
	args = append(args, s.hostExecArgs()...)
	args = append(args, "--")
	args = append(args, cmd...)
	return args
}

// namespaceArgs unshares PID (isolated, strict) and network (strict) namespaces.
func (s *Spec) namespaceArgs() []string {
	switch s.Sandbox {
	case binconfig.SandboxIsolated:
		return []string{"--unshare-pid"}
	case binconfig.SandboxStrict:
		return []string{"--unshare-pid", "--unshare-net"}
	default:
		return nil
	}
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

// hostExecArgs wires capsule-host-exec into the capsule when both fields are set.
func (s *Spec) hostExecArgs() []string {
	if s.HostExecSocket == "" || s.HostExecBinPath == "" {
		return nil
	}
	aliases := append([]string{binconfig.HostExecCommand}, binconfig.HostExecForwardedAliases...)
	args := make([]string, 0, 3*len(aliases)+4)
	if !s.RootWritable {
		args = append(args, "--tmpfs", "/usr/local/bin")
	}
	for _, name := range aliases {
		args = append(args, "--ro-bind", s.HostExecBinPath, "/usr/local/bin/"+name)
	}
	args = append(args, "--setenv", binconfig.HostExecSocketEnv, s.HostExecSocket)
	return args
}

func (s *Spec) Run(ctx context.Context, b *bundle.Extractor) (int, error) {
	args := s.Build()
	cmd := b.Command(ctx, "bwrap", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if log.IsDebug() {
		log.Debug("bwrap exec", "args", strings.Join(args, " "))
	}
	runErr := cmd.Run()
	// Hold the workspace alive while daemonized descendants (nginx, etc.)
	reaper.New(reaperGracePeriod).Wait(ctx)

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, runErr
	}
	return 0, nil
}

func (s *Spec) resolveCmd() []string {
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
		hostFile := filepath.Join("/etc", f)
		targetFile := filepath.Join(s.RootPath, "etc", f)
		if !fsutil.Exists(targetFile) {
			continue
		}
		args = append(args, "--ro-bind-try", hostFile, "/etc/"+f)
	}
	return args
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

// bindArgs emits --bind for each "src:dst" in Spec.Binds (bare path → src:src).
func (s *Spec) bindArgs() []string {
	var args []string
	for _, entry := range s.Binds {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		switch len(parts) {
		case 1:
			args = append(args, "--bind", parts[0], parts[0])
		case 2:
			args = append(args, "--bind", parts[0], parts[1])
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
