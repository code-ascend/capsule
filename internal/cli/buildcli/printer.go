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

	"github.com/urfave/cli/v3"
)

// printer renders command results to the CLI writer as text or JSON.
type printer struct {
	writer io.Writer
	json   bool
}

func newPrinter(command *cli.Command) printer {
	return printer{writer: command.Writer, json: command.Bool("json")}
}

func (p printer) render(response result) error {
	if p.json {
		encoder := json.NewEncoder(p.writer)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
		return nil
	}
	return response.text(p)
}
