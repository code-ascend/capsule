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
	"fmt"
	"io"
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

// list prints installed capsules as a table or, with asJSON, as a JSON array.
func list(w io.Writer, asJSON bool, extraRoots []string) error {
	m := manager.NewManager(extraRoots...)
	capsules := m.Scan()
	if asJSON {
		return listJSON(w, capsules)
	}
	if len(capsules) == 0 {
		exitcode.Notice(gotext.Get("No capsules found in: %s", strings.Join(m.RootPaths(), ", ")))
		return nil
	}
	tbl := table.New(w,
		gotext.Get("NAME"),
		gotext.Get("STATUS"),
		gotext.Get("RUNNING"),
		gotext.Get("SOURCE"),
		gotext.Get("SIZE"),
		gotext.Get("SHA"),
		gotext.Get("BUILT"),
	)
	for _, c := range capsules {
		running := "-"
		if isRunning(c.Path) {
			running = gotext.Get("yes")
		}
		tbl.Row(
			filepath.Base(c.Path),
			c.Kind.String(),
			running,
			c.Cfg.SourceRef,
			units.Bytes(c.Size),
			c.Cfg.SourceSHA[:min(len(c.Cfg.SourceSHA), 12)],
			c.Cfg.BuiltAt,
		)
	}
	return tbl.Flush()
}

func listJSON(w io.Writer, capsules []manager.Capsule) error {
	views := make([]capsuleView, 0, len(capsules))
	for _, c := range capsules {
		views = append(views, capsuleView{
			Name:      filepath.Base(c.Path),
			Path:      c.Path,
			Status:    c.Kind.Slug(),
			Running:   isRunning(c.Path),
			Source:    c.Cfg.SourceRef,
			SizeBytes: c.Size,
			SourceSHA: c.Cfg.SourceSHA,
			BuiltAt:   c.Cfg.BuiltAt,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(views); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
