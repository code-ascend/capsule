package capsule

// BinaryConfig holds configuration for the capsule binary layout.
type BinaryConfig struct {
	InitSize           int64  // Size of the init binary (padded)
	BashSize           int64  // Size of embedded bash
	ScriptSize         int64  // Size of runtime.sh (padded)
	UtilsSize          int64  // Size of utils.tar.gz
	Launch             string // Default command to run when no arguments provided
	ExportAppsLines    string // Apps export config: desktop|icon|suffix per line
	ExportBinariesBash string // Bash array for binaries export: ("ffmpeg" "yt-dlp")
	Compression        string // Compression type (zstd, xz, lz4, gzip) for mksquashfs at runtime
	UpdateScript       string // Combined bash script from update steps
	EnvUnsetBash       string // Bash array of env vars to unset: ("VAR1" "VAR2")
	EnvSetBash         string // Bash array of env vars to set: ("KEY=value" "KEY2=value2")
}
