// capsule
// Copyright (C) 2026 Дмитрий Удалов dmitry@udalov.online
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package buildcli

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"capsule/internal/build/manager"
	"capsule/internal/runtime/workspace"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"
	"capsule/internal/sys/table"
	"capsule/internal/sys/units"

	"github.com/leonelquinteros/gotext"
)

// isRunning probes capsule activity, treating probe failures as inactive.
func isRunning(path string) bool {
	active, err := workspace.Active(path)
	if err != nil {
		log.Debug("activity probe failed", "path", path, "error", err)
		return false
	}
	return active
}

type listEntry struct {
	manager.Capsule
	running bool
}

// list scans for installed capsules and returns a renderable result.
func list(extraRoots []string) result {
	m := manager.NewManager(extraRoots...)
	capsules := m.Scan()

	entries := make([]listEntry, 0, len(capsules))
	for _, c := range capsules {
		entries = append(entries, listEntry{Capsule: c, running: isRunning(c.Path)})
	}
	return listResult{entries: entries, roots: m.RootPaths()}
}

type listResult struct {
	entries []listEntry
	roots   []string
}

// capsuleView is the machine-readable projection of one installed capsule.
type capsuleView struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	Running   bool   `json:"running"`
	Source    string `json:"source,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	SourceSHA string `json:"source_sha,omitempty"`
	BuiltAt   string `json:"built_at,omitempty"`
}

// MarshalJSON renders the capsule list as a plain JSON array.
func (l listResult) MarshalJSON() ([]byte, error) {
	views := make([]capsuleView, 0, len(l.entries))
	for _, e := range l.entries {
		views = append(views, capsuleView{
			Name:      filepath.Base(e.Path),
			Path:      e.Path,
			Status:    e.Kind.Slug(),
			Running:   e.running,
			Source:    e.Cfg.SourceRef,
			SizeBytes: e.Size,
			SourceSHA: e.Cfg.SourceSHA,
			BuiltAt:   e.Cfg.BuiltAt,
		})
	}
	return json.Marshal(views)
}

func (l listResult) text(p printer) error {
	if len(l.entries) == 0 {
		exitcode.Notice(gotext.Get("No capsules found in: %s", strings.Join(l.roots, ", ")))
		return nil
	}
	tbl := table.New(p.writer,
		gotext.Get("NAME"),
		gotext.Get("STATUS"),
		gotext.Get("RUNNING"),
		gotext.Get("SOURCE"),
		gotext.Get("SIZE"),
		gotext.Get("SHA"),
		gotext.Get("BUILT"),
	)
	for _, e := range l.entries {
		running := "-"
		if e.running {
			running = gotext.Get("yes")
		}
		tbl.Row(
			filepath.Base(e.Path),
			e.Kind.String(),
			running,
			e.Cfg.SourceRef,
			units.Bytes(e.Size),
			shortSHA(e.Cfg.SourceSHA),
			e.Cfg.BuiltAt,
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
