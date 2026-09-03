// Package units formats byte counts for humans.
package units

import "fmt"

var si = []string{"B", "kB", "MB", "GB", "TB"}

// Bytes formats n in decimal SI units with one decimal, matching GLib's g_format_size and GNOME apps;
// whole bytes carry no decimal.
func Bytes(n int64) string {
	v := float64(n)
	i := 0
	for v >= 1000 && i < len(si)-1 {
		v /= 1000
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", v, si[i])
}
