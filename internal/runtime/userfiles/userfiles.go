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
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}
	srcDir := filepath.Join(rootPath, "etc")
	if _, err := os.Stat(filepath.Join(srcDir, "passwd")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	return h.merge(srcDir, outDir)
}

func (h *HostIdentity) EnsureOverlayUser(rootPath, overlayEtcDir string) error {
	passwdFile := filepath.Join(overlayEtcDir, "passwd")
	if _, err := os.Stat(passwdFile); errors.Is(err, fs.ErrNotExist) {
		return h.MergeFromRoot(rootPath, overlayEtcDir)
	} else if err != nil {
		return err
	}
	if h.userEntryUpToDate(passwdFile) {
		return nil
	}
	return h.merge(overlayEtcDir, overlayEtcDir)
}

// merge writes srcDir's passwd/group/shadow into dstDir with the host user's entries replacing clashing ones.
func (h *HostIdentity) merge(srcDir, dstDir string) error {
	if err := h.mergePasswd(filepath.Join(srcDir, "passwd"), filepath.Join(dstDir, "passwd")); err != nil {
		return err
	}
	if err := h.mergeGroup(filepath.Join(srcDir, "group"), filepath.Join(dstDir, "group")); err != nil {
		return err
	}
	return h.mergeShadow(filepath.Join(srcDir, "shadow"), filepath.Join(dstDir, "shadow"))
}

func (h *HostIdentity) mergePasswd(src, dst string) error {
	lines, err := readLines(src)
	if err != nil {
		return err
	}
	out := dropEntries(lines, h.User, strconv.Itoa(h.UID))
	out = append(out, fmt.Sprintf("%s:x:%d:%d:%s:%s:%s",
		h.User, h.UID, h.GID, h.Gecos, h.Home, h.Shell))
	return writeLines(dst, out, 0o644)
}

func (h *HostIdentity) mergeGroup(src, dst string) error {
	lines, err := readLines(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	out := dropEntries(lines, h.Group, strconv.Itoa(h.GID))
	out = append(out, fmt.Sprintf("%s:x:%d:%s", h.Group, h.GID, h.User))
	out = appendUserToGroup(out, "wheel", h.User, h.GID)
	out = appendUserToGroup(out, "sudo", h.User, h.GID)
	return writeLines(dst, out, 0o644)
}

func (h *HostIdentity) mergeShadow(src, dst string) error {
	lines, err := readLines(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	out := dropEntries(lines, h.User, "")
	out = append(out, fmt.Sprintf("%s::19000:0:99999:7:::", h.User))
	return writeLines(dst, out, 0o600)
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
