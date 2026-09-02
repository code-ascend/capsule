package mksquashfs

// Args returns mksquashfs arguments packing src into dst with the given compression.
func Args(src, dst, compression string) []string {
	args := []string{src, dst, "-comp", compression, "-noappend", "-no-xattrs"}
	switch compression {
	case "zstd":
		args = append(args, "-b", "1M", "-Xcompression-level", "19")
	case "xz":
		args = append(args, "-b", "1M", "-Xbcj", "x86")
	case "lz4":
		args = append(args, "-b", "256K", "-Xhc")
	case "gzip", "":
		args = append(args, "-b", "1M")
	}
	return args
}
