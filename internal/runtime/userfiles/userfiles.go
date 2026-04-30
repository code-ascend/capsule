package userfiles

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

type HostIdentity struct {
	User  string
	UID   int
	GID   int
	Group string
	Home  string
	Shell string
	Gecos string
}

func LookupHost() (*HostIdentity, error) {
	uid := os.Getuid()
	gid := os.Getgid()
	uname := os.Getenv("USER")

	var u *user.User
	var err error
	if uname != "" {
		u, err = user.Lookup(uname)
	}
	if u == nil {
		u, err = user.LookupId(strconv.Itoa(uid))
	}
	if err != nil {
		return nil, fmt.Errorf("lookup host user: %w", err)
	}
	g, err := user.LookupGroupId(strconv.Itoa(gid))
	gname := strconv.Itoa(gid)
	if err == nil {
		gname = g.Name
	}

	home := os.Getenv("HOME")
	if home == "" {
		home = u.HomeDir
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	return &HostIdentity{
		User:  u.Username,
		UID:   uid,
		GID:   gid,
		Group: gname,
		Home:  home,
		Shell: shell,
		Gecos: u.Username,
	}, nil
}

func (h *HostIdentity) MergeFromRoot(rootPath, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	passwdSrc := filepath.Join(rootPath, "etc", "passwd")
	if _, err := os.Stat(passwdSrc); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	if err := h.mergePasswd(passwdSrc, filepath.Join(outDir, "passwd")); err != nil {
		return err
	}
	if err := h.mergeGroup(filepath.Join(rootPath, "etc", "group"), filepath.Join(outDir, "group")); err != nil {
		return err
	}
	if err := h.mergeShadow(filepath.Join(rootPath, "etc", "shadow"), filepath.Join(outDir, "shadow")); err != nil {
		return err
	}
	return nil
}

func (h *HostIdentity) EnsureOverlayUser(rootPath, overlayEtcDir string) error {
	passwdFile := filepath.Join(overlayEtcDir, "passwd")
	if _, err := os.Stat(passwdFile); errors.Is(err, fs.ErrNotExist) {
		return h.MergeFromRoot(rootPath, overlayEtcDir)
	}
	if h.userEntryUpToDate(passwdFile) {
		return nil
	}
	return h.updateOverlayUser(overlayEtcDir)
}

func (h *HostIdentity) mergePasswd(src, dst string) error {
	lines, err := readLines(src)
	if err != nil {
		return err
	}
	out := filterByField3NotEqual(lines, h.UID)
	out = append(out, fmt.Sprintf("%s:x:%d:%d:%s:%s:%s",
		h.User, h.UID, h.GID, h.Gecos, h.Home, h.Shell))
	return writeLines(dst, out, 0644)
}

func (h *HostIdentity) mergeGroup(src, dst string) error {
	lines, err := readLines(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	out := filterByField3NotEqual(lines, h.GID)
	out = append(out, fmt.Sprintf("%s:x:%d:%s", h.Group, h.GID, h.User))
	out = appendUserToGroup(out, "wheel", h.User, h.GID)
	out = appendUserToGroup(out, "sudo", h.User, h.GID)
	return writeLines(dst, out, 0644)
}

func (h *HostIdentity) mergeShadow(src, dst string) error {
	lines, err := readLines(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := h.User + ":"
	var kept []string
	for _, l := range lines {
		if !strings.HasPrefix(l, prefix) {
			kept = append(kept, l)
		}
	}
	kept = append(kept, fmt.Sprintf("%s::19000:0:99999:7:::", h.User))
	return writeLines(dst, kept, 0600)
}

func (h *HostIdentity) userEntryUpToDate(passwdFile string) bool {
	lines, err := readLines(passwdFile)
	if err != nil {
		return false
	}
	prefix := h.User + ":"
	for _, l := range lines {
		if !strings.HasPrefix(l, prefix) {
			continue
		}
		fields := strings.Split(l, ":")
		if len(fields) >= 3 && fields[2] == strconv.Itoa(h.UID) {
			return true
		}
	}
	return false
}

func (h *HostIdentity) updateOverlayUser(dir string) error {
	if err := h.rewritePasswdEntry(filepath.Join(dir, "passwd")); err != nil {
		return err
	}
	if err := h.rewriteGroupEntry(filepath.Join(dir, "group")); err != nil {
		return err
	}
	if err := h.rewriteShadowEntry(filepath.Join(dir, "shadow")); err != nil {
		return err
	}
	return nil
}

func (h *HostIdentity) rewritePasswdEntry(path string) error {
	lines, err := readLines(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines = dropPrefix(lines, h.User+":")
	lines = append(lines, fmt.Sprintf("%s:x:%d:%d:%s:%s:%s",
		h.User, h.UID, h.GID, h.Gecos, h.Home, h.Shell))
	return writeLines(path, lines, 0644)
}

func (h *HostIdentity) rewriteGroupEntry(path string) error {
	lines, err := readLines(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines = dropPrefix(lines, h.User+":")
	lines = append(lines, fmt.Sprintf("%s:x:%d:", h.Group, h.GID))
	lines = appendUserToGroup(lines, "wheel", h.User, h.GID)
	lines = appendUserToGroup(lines, "sudo", h.User, h.GID)
	return writeLines(path, lines, 0644)
}

func (h *HostIdentity) rewriteShadowEntry(path string) error {
	lines, err := readLines(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines = dropPrefix(lines, h.User+":")
	lines = append(lines, fmt.Sprintf("%s:!!:::::::", h.User))
	return writeLines(path, lines, 0600)
}
