package manager

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"capsule/internal/format/binconfig"
	"capsule/internal/format/selfread"
	"capsule/internal/sys/log"
	"capsule/internal/sys/srcref"
	"capsule/internal/sys/table"

	"github.com/leonelquinteros/gotext"
)

// Rebuilder rebuilds one capsule from its recorded source.
type Rebuilder func(ctx context.Context, c Capsule) error

type SourceKind int

const (
	SourceUnknown SourceKind = iota
	SourceExternal
	SourceLocalPresent
	SourceLocalMissing
)

func (k SourceKind) String() string {
	switch k {
	case SourceExternal:
		return gotext.Get("external")
	case SourceLocalPresent:
		return gotext.Get("local")
	case SourceLocalMissing:
		return gotext.Get("local-missing")
	default:
		return gotext.Get("no source")
	}
}

type Capsule struct {
	Path string
	Cfg  *binconfig.Config
	Size int64
	Kind SourceKind
}

// Updatable reports whether the capsule has a usable source.
func (c Capsule) Updatable() bool {
	return c.Kind == SourceExternal || c.Kind == SourceLocalPresent
}

type UpdateOpts struct {
	DryRun    bool
	KeepGoing bool
}

// scanRoot is a directory to scan, paired with whether the user supplied it.
type scanRoot struct {
	path string
	user bool
}

type Manager struct {
	roots []scanRoot
}

func NewManager(extraRoots ...string) *Manager {
	var roots []scanRoot
	for _, r := range defaultScanRoots() {
		roots = append(roots, scanRoot{path: r, user: false})
	}
	for _, r := range extraRoots {
		if r != "" {
			roots = append(roots, scanRoot{path: r, user: true})
		}
	}
	return &Manager{roots: roots}
}

func defaultScanRoots() []string {
	home, _ := os.UserHomeDir()
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			home = u.HomeDir
		}
	}
	if home == "" {
		return nil
	}
	return []string{filepath.Join(home, ".local/bin")}
}

// Scan returns every capsule found under the configured roots.
func (m *Manager) Scan() []Capsule {
	seen := map[string]bool{}
	var out []Capsule
	for _, root := range m.roots {
		entries, err := os.ReadDir(root.path)
		if err != nil {
			if root.user {
				log.Warn(gotext.Get("scan path unreadable"), "dir", root.path, "error", err)
			} else {
				log.Debug("scan: readdir failed", "dir", root.path, "error", err)
			}
			continue
		}
		for _, e := range entries {
			if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
				continue
			}
			abs, err := filepath.Abs(filepath.Join(root.path, e.Name()))
			if err != nil || seen[abs] {
				continue
			}
			seen[abs] = true
			if !selfread.IsCapsule(abs) {
				continue
			}
			c, ok := loadCapsule(abs)
			if !ok {
				continue
			}
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Capsule) int {
		return cmp.Compare(filepath.Base(a.Path), filepath.Base(b.Path))
	})
	return out
}

func loadCapsule(path string) (Capsule, bool) {
	fail := func(stage string, err error) (Capsule, bool) {
		log.Debug("scan: "+stage+" failed", "path", path, "error", err)
		return Capsule{}, false
	}
	layout, err := selfread.ReadLayout(path)
	if err != nil {
		return fail("ReadLayout", err)
	}
	raw, err := selfread.ReadBinConfig(path, layout)
	if err != nil {
		return fail("ReadBinConfig", err)
	}
	cfg := &binconfig.Config{}
	if len(raw) > 0 {
		cfg, err = binconfig.Unmarshal(raw)
		if err != nil {
			return fail("Unmarshal", err)
		}
	}
	var size int64
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	return Capsule{
		Path: path,
		Cfg:  cfg,
		Size: size,
		Kind: classify(cfg.SourceRef),
	}, true
}

func classify(ref string) SourceKind {
	if ref == "" {
		return SourceUnknown
	}
	if srcref.IsRemote(ref) {
		return SourceExternal
	}
	if _, err := os.Stat(ref); err == nil {
		return SourceLocalPresent
	}
	return SourceLocalMissing
}

// rootPaths returns just the directory strings, for diagnostics.
func (m *Manager) rootPaths() []string {
	out := make([]string, len(m.roots))
	for i, r := range m.roots {
		out[i] = r.path
	}
	return out
}

// List prints all installed capsules as a table.
func (m *Manager) List() error {
	caps := m.Scan()
	if len(caps) == 0 {
		fmt.Fprintln(os.Stderr, gotext.Get("No capsules found in: %s", strings.Join(m.rootPaths(), ", ")))
		return nil
	}
	tbl := table.New(os.Stdout,
		gotext.Get("NAME"),
		gotext.Get("STATUS"),
		gotext.Get("SOURCE"),
		gotext.Get("SIZE"),
		gotext.Get("SHA"),
		gotext.Get("BUILT"),
	)
	for _, c := range caps {
		tbl.Row(
			filepath.Base(c.Path),
			c.Kind.String(),
			c.Cfg.SourceRef,
			fmt.Sprintf("%.1f MB", float64(c.Size)/(1024*1024)),
			shortSHA(c.Cfg.SourceSHA),
			c.Cfg.BuiltAt,
		)
	}
	return tbl.Flush()
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// Update rebuilds the named capsules, or all when names is empty.
func (m *Manager) Update(ctx context.Context, names []string, opts UpdateOpts, rebuild Rebuilder) error {
	caps := m.Scan()
	selected, err := selectCapsules(caps, names)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Fprintln(os.Stderr, gotext.Get("No capsules to update"))
		return nil
	}

	var updated, skipped, failed int
	for _, c := range selected {
		name := filepath.Base(c.Path)
		if !c.Updatable() {
			log.Warn(gotext.Get("Skipping capsule"), "name", name, "reason", skipReason(c.Kind))
			skipped++
			continue
		}
		if opts.DryRun {
			log.Info(gotext.Get("Would rebuild capsule"), "name", name, "from", c.Cfg.SourceRef)
			updated++
			continue
		}
		log.Info(gotext.Get("Updating capsule"), "name", name, "from", c.Cfg.SourceRef)
		if err := rebuild(ctx, c); err != nil {
			failed++
			log.Error(gotext.Get("Update failed"), "name", name, "error", err)
			if !opts.KeepGoing {
				return err
			}
			continue
		}
		updated++
	}
	if opts.DryRun {
		log.Info(gotext.Get("Update summary (dry-run)"),
			gotext.Get("Pending"), updated,
			gotext.Get("Skipped"), skipped)
	} else {
		log.Info(gotext.Get("Update summary"),
			gotext.Get("Updated"), updated,
			gotext.Get("Skipped"), skipped,
			gotext.Get("Failed"), failed)
	}
	if failed > 0 {
		return errors.New(gotext.Get("%d capsule(s) failed to update", failed))
	}
	return nil
}

func selectCapsules(caps []Capsule, names []string) ([]Capsule, error) {
	if len(names) == 0 {
		return caps, nil
	}
	byBase := map[string]Capsule{}
	byPath := map[string]Capsule{}
	for _, c := range caps {
		byBase[filepath.Base(c.Path)] = c
		byPath[c.Path] = c
	}
	picked := map[string]bool{}
	var out []Capsule
	for _, n := range names {
		c, ok := lookupCapsule(byPath, byBase, n)
		if !ok {
			return nil, errors.New(gotext.Get("capsule %q not found", n))
		}
		if picked[c.Path] {
			continue
		}
		picked[c.Path] = true
		out = append(out, c)
	}
	return out, nil
}

func lookupCapsule(byPath, byBase map[string]Capsule, n string) (Capsule, bool) {
	if c, ok := byPath[n]; ok {
		return c, true
	}
	if abs, err := filepath.Abs(n); err == nil {
		if c, ok := byPath[abs]; ok {
			return c, true
		}
	}
	if c, ok := byBase[n]; ok {
		return c, true
	}
	return Capsule{}, false
}

// skipReason describes why a non-updatable capsule was skipped.
func skipReason(k SourceKind) string {
	switch k {
	case SourceLocalMissing:
		return gotext.Get("source file missing")
	case SourceUnknown:
		return gotext.Get("no source recorded")
	default:
		return gotext.Get("not updatable")
	}
}
