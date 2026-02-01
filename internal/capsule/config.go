package capsule

// BinaryConfig holds configuration for the capsule binary layout.
type BinaryConfig struct {
	InitSize   int64  // Size of the init binary (padded)
	BashSize   int64  // Size of embedded bash
	ScriptSize int64  // Size of runtime.sh (padded)
	UtilsSize  int64  // Size of utils.tar.gz
	Launch     string // Default command to run when no arguments provided
}
