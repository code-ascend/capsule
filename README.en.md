# capsule

A tool for building portable Linux containers from OCI images. The result is a single ELF binary with a statically
compiled Go runtime inside.

## Features

- **Single file** — download and run, no dependencies
- **Portable** — drop it anywhere, including a USB stick
- **No root required** — running a capsule does not need superuser privileges
- **Seamless experience** — applications read and store configs in your `$HOME`
- **Clean host** — the application runs in its own environment with its own libraries, without polluting the host
- **Bundle of apps** — one capsule can ship a whole set of applications and utilities
- **Static Go runtime** — no dependency on the host glibc/bash
- **Isolated runtime** — utilities run via the bundled ld-linux and libc

## Installation

### Install from ALS

> The capsules themselves are highly self-contained and portable, but building them requires this project.

```bash
sudo apm repo add rpm https://altlinux.space/api/packages/dmitry/alt/group/capsule-nightly/sisyphus.repo _arch_ classic
sudo apm s update
sudo apm s install capsule
```

### Manual install

```bash
sudo apt-get install squashfs-tools go meson ninja-build gettext \
                     libgpgme-devel libbtrfs-devel libdevmapper-devel \
                     shadow-submap fuse-overlayfs containers-common netavark
meson setup build --prefix=/usr
meson compile -C build
meson install -C build
```

### Updating translations

```bash
meson compile -C build pot-update
```

## Building a portable capsule

> Note: this is the `max` build example. The `examples` directory has other recipes with different runtime/package
> sets. The point of this example is to get familiar with capsule builds — the next step is to author your own
> capsule with the package set you need.

```bash
# Build max (path to a local file or a URL with the build recipe)
capsule build https://altlinux.space/dmitry/capsule/raw/branch/main/examples/max.yaml -v
# Help
max -h
# Run
max
# Enter interactively, changes persist between runs
max shell
# Export desktop entries
max export
# Remove desktop entries from the host
max unexport
```

## Working with capsules

All changes made inside a capsule (installed packages, edited configs) persist across runs and can be committed back
into the image.

```bash
# Enter a shell, install new packages, edit files
max shell
# Commit the changes
max commit
# Update the capsule
sudo max update
# Discard all uncommitted changes
max clean
```

## Running host commands from a capsule (host-exec)

A capsule is isolated from the host, but sometimes you need to run a command **on the host itself** — for example, to
let a browser inside the capsule open a link in the host's default viewer instead of trying to handle it in-capsule.
The `capsule-host-exec` bridge is built in for exactly this.

Enabled in `config.yaml`:

```yaml
host_exec: true
```

At runtime the capsule binds an abstract UNIX socket on the host and forwards a `capsule-host-exec` client into the
capsule. The client sends the command name and arguments through the socket to the host side, where they execute as
a regular process under your UID.

The same client is also bound under **several aliases** to transparently intercept common in-capsule calls reaching
out to the host:

| alias inside capsule | behaviour                                                        |
|----------------------|------------------------------------------------------------------|
| `capsule-host-exec`  | direct call: `capsule-host-exec CMD [ARGS...]`                   |
| `xdg-open`           | links/files open in the host environment (browser, file manager) |
| `gio`                | gio operations (mount/open/...) are forwarded to the host        |

`CAPSULE_HOST_SOCKET` (env with the socket name) and the standard stdin/stdout/stderr streams are also forwarded.

Notes:

- The bridge is transparent in both directions: inside the capsule you write `xdg-open https://...`, and the host's
  `xdg-open` runs.
- In `--no-overlay` mode (read-only rootfs) `/usr/local/bin` is overlaid with a tmpfs so the aliases have a place to
  be bound — host-exec keeps working.

## In-rootfs config overrides

Install scripts (or third-party packages) can drop a `.capsule.overrides.yml` at the rootfs root during build to
amend the capsule manifest from inside. After install steps finish and before the squashfs is sealed, capsule merges
this file into the build config (validating the result) and removes it so it never ships in the image. Useful when
an installed package wants to declare its own `launch`, `export`, `env` or other top-level fields without editing the
outer YAML.

```yaml
# /.capsule.overrides.yml inside the rootfs
launch: /usr/bin/myapp
export:
  apps:
    - desktop: /usr/share/applications/myapp.desktop
```

## How it works

Capsule is a single executable with the following layout:

| Go runtime + utils.tar.gz | binconfig (JSON)  | SquashFS | Footer   |
|---------------------------|-------------------|----------|----------|
| ~14 MB                    | hundreds of bytes | image    | 32 bytes |

## Step by step

1. On startup the Go runtime reads `/proc/self/exe`, parses the 32-byte footer and learns the offsets of the embedded
   payloads (binconfig + squashfs).
2. It then unpacks `utils.tar.gz` (bwrap, squashfuse, unionfs, mksquashfs, ld-linux, libs) into a temporary workspace
   directly out of itself.
3. Squashfuse mounts the SquashFS image straight from the binary at the given offset (`-o offset=N`) over FUSE — no
   root, in userspace.
4. Optionally a writable unionfs overlay is layered on top, where session changes and host-imported NVIDIA libraries
   are merged.
5. Bubblewrap creates an isolated environment via user namespaces: root = squashfs (or overlay), forwards `$HOME`,
   `/dev`, `/tmp` (including X11 sockets at `/tmp/.X11-unix`), `/run` (including the Wayland socket
   `/run/user/UID/wayland-*` and other per-user unix sockets), and overlays `/etc/resolv.conf`, `/etc/hosts`,
   `/etc/localtime`, `/etc/machine-id` from the host.
6. If the manifest has `host_exec: true`, the runtime opens an abstract UNIX socket and binds the
   `capsule-host-exec` client (plus `xdg-open` / `gio` aliases) inside the capsule to forward selected commands back
   to the host.

## Roadmap

- [x] Self-contained capsule updates
- [x] NVIDIA support (under testing)
- [x] Building capsules without root privileges
- [x] Replace the capsule runtime with Go, drop bash/C inserts
- [x] Add translations
- [ ] Add capsule permissions management (filesystem/dbus)
- [ ] Build the capsule's bundled utilities and binary deps for ALT Linux on ALS

# Credit

- [Conty](https://github.com/Kron4ek/Conty) — for the idea
- [Epm](https://github.com/Etersoft/eepm) — for packages
- [Stplr](https://altlinux.space/stapler/stplr) — for packages