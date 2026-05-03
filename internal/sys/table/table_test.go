package table

import (
	"bytes"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	var buf bytes.Buffer
	tbl := New(&buf, "NAME", "STATUS", "SIZE")
	tbl.Row("alpha", "ok", "10 MB")
	tbl.Row("beta-long", "stale", "1.2 GB")
	tbl.Row("gamma") // missing cells fill with "-"
	if err := tbl.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := buf.String()
	for _, want := range []string{"NAME", "alpha", "beta-long", "gamma", "1.2 GB", "-"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	starts := columnStarts(lines[0])
	for i := 1; i < len(lines); i++ {
		row := columnStarts(lines[i])
		if len(row) < len(starts) || !equalPrefix(row, starts) {
			t.Errorf("line %d columns %v, want prefix %v:\n%q", i, row, starts, lines[i])
		}
	}
}

// columnStarts returns byte offsets where each whitespace-separated cell starts.
func columnStarts(line string) []int {
	var out []int
	in := false
	for i, r := range line {
		if r != ' ' && !in {
			out = append(out, i)
			in = true
		} else if r == ' ' {
			in = false
		}
	}
	return out
}

func equalPrefix(a, b []int) bool {
	for i := range b {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
