package config

const (
	InitPaddedSize   = 786432 // 768KB - space for init/launcher binary
	ScriptPaddedSize = 49152  // 48KB - space for runtime.sh script

	ImageSquashfs = "image.squashfs"

	TempDir            = "/var/tmp"
	TempPrefixCompile  = "capsule-compile-"
	TempPrefixAssemble = "capsule-assemble-"
)
