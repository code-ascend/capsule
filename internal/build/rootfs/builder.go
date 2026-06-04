package rootfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"capsule/internal/sys/log"

	"github.com/containers/buildah"
	"github.com/containers/buildah/define"
	"github.com/sirupsen/logrus"
	"go.podman.io/storage"
)

type Builder struct {
	store     storage.Store
	builder   *buildah.Builder
	rootfsMnt string
}

// Full Linux capability for build
var allCapabilities = []string{
	"CAP_AUDIT_CONTROL", "CAP_AUDIT_READ", "CAP_AUDIT_WRITE",
	"CAP_BLOCK_SUSPEND", "CAP_BPF", "CAP_CHECKPOINT_RESTORE",
	"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_DAC_READ_SEARCH",
	"CAP_FOWNER", "CAP_FSETID", "CAP_IPC_LOCK", "CAP_IPC_OWNER",
	"CAP_KILL", "CAP_LEASE", "CAP_LINUX_IMMUTABLE", "CAP_MAC_ADMIN",
	"CAP_MAC_OVERRIDE", "CAP_MKNOD", "CAP_NET_ADMIN", "CAP_NET_BIND_SERVICE",
	"CAP_NET_BROADCAST", "CAP_NET_RAW", "CAP_PERFMON", "CAP_SETFCAP",
	"CAP_SETGID", "CAP_SETPCAP", "CAP_SETUID", "CAP_SYS_ADMIN",
	"CAP_SYS_BOOT", "CAP_SYS_CHROOT", "CAP_SYS_MODULE", "CAP_SYS_NICE",
	"CAP_SYS_PACCT", "CAP_SYS_PTRACE", "CAP_SYS_RAWIO", "CAP_SYS_RESOURCE",
	"CAP_SYS_TIME", "CAP_SYS_TTY_CONFIG", "CAP_SYSLOG", "CAP_WAKE_ALARM",
}

var bindPlaceholders = []struct {
	path    string
	content string
	perm    os.FileMode
}{
	{"machine-id", "", 0o444},
	{"localtime", "", 0o644},
	{"hosts", "127.0.0.1 localhost\n", 0o644},
	{"nsswitch.conf", "hosts: files dns\n", 0o644},
	{"resolv.conf", "", 0o644},
}

var runEnv = []string{
	"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	"HOME=/root",
	"TMPDIR=/tmp",
	"LC_ALL=C",
}

func NewBuilder(ctx context.Context, image string) (*Builder, error) {
	storeOpts, err := storage.DefaultStoreOptions()
	if err != nil {
		return nil, fmt.Errorf("load storage options: %w", err)
	}
	storeOpts.GraphDriverName = "overlay"
	storeOpts.GraphDriverOptions = []string{
		"overlay.mount_program=/usr/bin/fuse-overlayfs",
	}

	store, err := storage.GetStore(storeOpts)
	if err != nil {
		return nil, fmt.Errorf("init containers storage at %s: %w (hint: ensure /etc/subuid and /etc/subgid have an entry for $USER)", storeOpts.GraphRoot, err)
	}

	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	if log.IsDebug() {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.WarnLevel)
	}

	bb, err := buildah.NewBuilder(ctx, store, buildah.BuilderOptions{
		FromImage:    image,
		Isolation:    define.IsolationChroot,
		PullPolicy:   define.PullIfNewer,
		Capabilities: allCapabilities,
		IDMappingOptions: &define.IDMappingOptions{
			HostUIDMapping: true,
			HostGIDMapping: true,
		},
		Logger:       logger,
		ReportWriter: os.Stderr,
	})
	if err != nil {
		_, _ = store.Shutdown(false)
		return nil, fmt.Errorf("create buildah container from %s: %w", image, err)
	}

	mnt, err := bb.Mount(bb.MountLabel)
	if err != nil {
		_ = bb.Delete()
		_, _ = store.Shutdown(false)
		return nil, fmt.Errorf("mount container rootfs: %w", err)
	}

	log.Debug("Buildah container ready", "image", image, "rootfs", mnt)
	return &Builder{store: store, builder: bb, rootfsMnt: mnt}, nil
}

func (b *Builder) RunScript(_ context.Context, script string) error {
	if script == "" {
		return nil
	}
	log.Debug("Running script in container", "script_length", len(script))

	err := b.builder.Run([]string{"/bin/sh", "-c", "set -e\n" + script}, buildah.RunOptions{
		Isolation:       define.IsolationChroot,
		AddCapabilities: allCapabilities,
		Env:             runEnv,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		Quiet:           !log.IsDebug(),
	})
	if err != nil {
		return fmt.Errorf("script failed: %w", err)
	}
	return nil
}

func (b *Builder) PrepareBindTargets() error {
	// var/home is a tmpfs mount point for binding an ostree/atomic home
	for _, d := range []string{"media", "var/home"} {
		if err := os.MkdirAll(filepath.Join(b.rootfsMnt, d), 0o755); err != nil {
			log.Debug("Failed to create bind-target dir", "dir", d, "error", err)
		}
	}

	etcDir := filepath.Join(b.rootfsMnt, "etc")
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		return err
	}

	for _, f := range bindPlaceholders {
		p := filepath.Join(etcDir, f.path)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		// A dangling symlink is not: drop it so the placeholder lands a real file.
		if _, err := os.Lstat(p); err == nil {
			_ = os.Remove(p)
		}
		if err := os.WriteFile(p, []byte(f.content), f.perm); err != nil {
			log.Debug("Failed to create placeholder", "file", f.path, "error", err)
		}
	}
	return nil
}

func (b *Builder) RootfsPath() string {
	return b.rootfsMnt
}

func (b *Builder) Cleanup() {
	if b.builder != nil {
		if err := b.builder.Unmount(); err != nil {
			log.Debug("Failed to unmount container", "error", err)
		}
		if err := b.builder.Delete(); err != nil {
			log.Debug("Failed to delete container", "error", err)
		}
		b.builder = nil
	}
	if b.store != nil {
		if _, err := b.store.Shutdown(true); err != nil {
			log.Debug("Failed to shutdown storage", "error", err)
		}
		b.store = nil
	}
}
