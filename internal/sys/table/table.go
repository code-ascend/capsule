// Package table renders aligned text tables to an io.Writer.
package table

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type Table struct {
	w       *tabwriter.Writer
	cols    int
	headers []string
}

// New starts a table with the given column headers.
func New(out io.Writer, headers ...string) *Table {
	t := &Table{
		w:       tabwriter.NewWriter(out, 0, 0, 2, ' ', 0),
		cols:    len(headers),
		headers: headers,
	}
	fmt.Fprintln(t.w, strings.Join(headers, "\t"))
	return t
}

// Row appends one row; extra cells are dropped, missing cells become "-".
func (t *Table) Row(cells ...string) {
	out := make([]string, t.cols)
	for i := range t.cols {
		if i < len(cells) && cells[i] != "" {
			out[i] = cells[i]
		} else {
			out[i] = "-"
		}
	}
	fmt.Fprintln(t.w, strings.Join(out, "\t"))
}

// Flush writes the buffered output.
func (t *Table) Flush() error { return t.w.Flush() }
