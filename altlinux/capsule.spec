%define _unpackaged_files_terminate_build 1

Name: capsule
Version: 0.3.3
Release: alt1

Summary: Tool for creating portable Linux containers from OCI images
Summary(ru_RU.UTF-8): Инструмент для создания портативных Linux-контейнеров из OCI-образов
License: GPL-3.0-or-later
Group: System/Configuration/Other
Url: https://altlinux.space/dmitry/capsule
Vcs: https://altlinux.space/dmitry/capsule.git

ExclusiveArch: %go_arches

Source: %name-%version.tar
Source1: vendor.tar

BuildRequires(pre): rpm-macros-golang
BuildRequires(pre): rpm-macros-meson
BuildRequires: rpm-build-golang
BuildRequires: golang
BuildRequires: meson
BuildRequires: libgpgme-devel
BuildRequires: libbtrfs-devel
BuildRequires: libdevmapper-devel

Requires: bubblewrap
Requires: squashfuse
Requires: fuse-overlayfs
Requires: squashfs-tools

%description
capsule is a tool for creating portable Linux containers from OCI images,
producing a single self-contained ELF binary that bundles a SquashFS rootfs
and a Go runtime which mounts and runs it via bubblewrap.

%description -l ru_RU.UTF-8
capsule — инструмент для создания портативных Linux-контейнеров из OCI-образов.
Результат сборки — единый самодостаточный ELF-файл, содержащий SquashFS-корень
и Go-рантайм, который монтирует и запускает его через bubblewrap.

%prep
%setup -a1

# Fix go vendoring build: rename "[generated]" files
for file in $(find -name "*\[generated\]*"); do
  mv -v "$file" "${file//\[generated\]/}"
done

%build
export GOFLAGS="-mod=vendor"
%meson
%meson_build

%install
%meson_install

%find_lang %name

%files -f %name.lang
%_bindir/%name
%doc README.md
%doc README.en.md
%doc README.ru.md
%doc LICENSE

%changelog
* Sat May 17 2026 Dmitry Udalov <udalov@altlinux.org> 0.3.3-alt1
- Initial build for Sisyphus.
