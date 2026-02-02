package config

const (
	InitPaddedSize   = 786432 // 768KB - space for init/launcher binary
	ScriptPaddedSize = 32768  // 32KB - space for runtime.sh script

	RootfsDir     = "rootfs"
	ImageTar      = "image.tar"
	ImageSquashfs = "image.squashfs"

	CacheDir           = "/var/cache/capsule"
	TempDir            = "/var/tmp"
	TempPrefixImage    = "capsule-image-"
	TempPrefixCompile  = "capsule-compile-"
	TempPrefixAssemble = "capsule-assemble-"
)
