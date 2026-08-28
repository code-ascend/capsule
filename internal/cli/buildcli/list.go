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
	"path/filepath"
	"strings"

	"capsule/internal/build/manager"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/table"

	"github.com/leonelquinteros/gotext"
)

// list scans for installed capsules and returns a renderable result.
func list(extraRoots []string) result {
	m := manager.NewManager(extraRoots...)
	return listResult{capsules: m.Scan(), roots: m.RootPaths()}
}

type listResult struct {
	capsules []manager.Capsule
	roots    []string
}

// capsuleView is the machine-readable projection of one installed capsule.
type capsuleView struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	Source    string `json:"source,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	SourceSHA string `json:"source_sha,omitempty"`
	BuiltAt   string `json:"built_at,omitempty"`
}

// MarshalJSON renders the capsule list as a plain JSON array.
func (l listResult) MarshalJSON() ([]byte, error) {
	views := make([]capsuleView, 0, len(l.capsules))
	for _, c := range l.capsules {
		views = append(views, capsuleView{
			Name:      filepath.Base(c.Path),
			Path:      c.Path,
			Status:    c.Kind.Slug(),
			Source:    c.Cfg.SourceRef,
			SizeBytes: c.Size,
			SourceSHA: c.Cfg.SourceSHA,
			BuiltAt:   c.Cfg.BuiltAt,
		})
	}
	return json.Marshal(views)
}

func (l listResult) text(p printer) error {
	if len(l.capsules) == 0 {
		exitcode.Notice(gotext.Get("No capsules found in: %s", strings.Join(l.roots, ", ")))
		return nil
	}
	tbl := table.New(p.writer,
		gotext.Get("NAME"),
		gotext.Get("STATUS"),
		gotext.Get("SOURCE"),
		gotext.Get("SIZE"),
		gotext.Get("SHA"),
		gotext.Get("BUILT"),
	)
	for _, c := range l.capsules {
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
