package bwrap

import (
	"capsule/internal/sys/fsutil"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"capsule/internal/format/binconfig"
	"capsule/internal/runtime/bundle"
	"capsule/internal/sys/log"
)

type Spec struct {
	RootPath      string
	RootWritable  bool
	MergedUserDir string
	Cmd           []string
	Cfg           *binconfig.Config
	Env           Env

	// Both empty disables host-exec; otherwise the ELF is bound in and the
	// abstract socket name is exported as CAPSULE_HOST_SOCKET.
	HostExecSocket  string
	HostExecBinPath string
}

// Env carries host-side variables that shape bwrap args. Pass EnvFromOS()
// in production; populate explicitly in tests.
type Env struct {
	Home        string
	CapsuleHome string
	User        string
	Term        string
	Lang        string
	XDGDataDirs string
	CapsuleBind string
}

func EnvFromOS() Env {
	return Env{
		Home:        os.Getenv("HOME"),
		CapsuleHome: os.Getenv("CAPSULE_HOME"),
		User:        os.Getenv("USER"),
		Term:        os.Getenv("TERM"),
		Lang:        os.Getenv("LANG"),
		XDGDataDirs: os.Getenv("XDG_DATA_DIRS"),
		CapsuleBind: os.Getenv("CAPSULE_BIND"),
	}
}

func (s *Spec) Build() []string {
	cmd := s.resolveCmd()

	var args []string
	args = append(args, s.rootBind()...)
	args = append(args,
		"--dev-bind", "/dev", "/dev",
		"--ro-bind", "/sys", "/sys",
		"--proc", "/proc",
	)
	for _, p := range []string{"/tmp", "/run", "/run/dbus", "/var/tmp", "/mnt", "/media"} {
		args = append(args, "--bind-try", p, p)
	}
	args = append(args, hostEtcBinds(s.RootPath)...)
	args = append(args, s.mergedUserBinds()...)
	args = append(args, s.Env.homeBinds()...)
	args = append(args, s.Env.capsuleBinds()...)
	args = append(args, s.Env.defaults()...)
	args = append(args, configEnv(s.Cfg)...)
	args = append(args, s.hostExecArgs()...)
	args = append(args, "--")
	args = append(args, cmd...)
	return args
}

// hostExecArgs wires capsule-host-exec into the capsule when both fields are set.
func (s *Spec) hostExecArgs() []string {
	if s.HostExecSocket == "" || s.HostExecBinPath == "" {
		return nil
	}
	aliases := append([]string{binconfig.HostExecCommand}, binconfig.HostExecForwardedAliases...)
	args := make([]string, 0, 3*len(aliases)+2)
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
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, err
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

// hostEtcBinds binds host /etc network/locale files into the capsule.
func hostEtcBinds(_ string) []string {
	files := []string{"resolv.conf", "hosts", "nsswitch.conf", "localtime", "machine-id", "asound.conf"}
	var args []string
	for _, f := range files {
		hostFile := filepath.Join("/etc", f)
		if fsutil.Exists(hostFile) {
			args = append(args, "--ro-bind-try", hostFile, "/etc/"+f)
		}
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

// homeBinds remaps non-/home host homes (e.g. /var/home on ALT/Silverblue) to
// /home/$USER so apps that hardcode /home/* keep working.
func (e Env) homeBinds() []string {
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
	return []string{
		"--tmpfs", "/home",
		"--dir", containerHome,
		"--bind", home, containerHome,
		"--setenv", "HOME", containerHome,
		"--setenv", "XDG_CONFIG_HOME", containerHome + "/.config",
		"--setenv", "XDG_DATA_HOME", containerHome + "/.local/share",
	}
}

// capsuleBinds parses CAPSULE_BIND="src:dst,src,..." (bare path → src:src).
func (e Env) capsuleBinds() []string {
	if e.CapsuleBind == "" {
		return nil
	}
	var args []string
	for entry := range strings.SplitSeq(e.CapsuleBind, ",") {
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

func configEnv(c *binconfig.Config) []string {
	if c == nil {
		return nil
	}
	var args []string
	for _, k := range c.EnvUnset {
		args = append(args, "--unsetenv", k)
	}
	keys := make([]string, 0, len(c.EnvSet))
	for k := range c.EnvSet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--setenv", k, c.EnvSet[k])
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
