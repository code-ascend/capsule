#!/usr/bin/env bash
# Build the runtime utils tarball that gets embedded into the Capsule launcher.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "${script_dir}/.." && pwd)"

build_dir="${repo_dir}/build-utils"
out_dir="${build_dir}/utils"
rm -rf "${build_dir}"
mkdir -p "${out_dir}"

cp /usr/bin/bwrap         "${out_dir}/"
cp /usr/bin/mksquashfs    "${out_dir}/"
cp /usr/bin/unsquashfs    "${out_dir}/"
cp /usr/bin/squashfuse    "${out_dir}/"
cp /usr/bin/squashfuse_ll "${out_dir}/"
cp /usr/bin/unionfs       "${out_dir}/"

cp /usr/bin/squashfuse    "${out_dir}/squashfuse3"
cp /usr/bin/squashfuse_ll "${out_dir}/squashfuse3_ll"

mapfile -t libs < <(ldd "${out_dir}"/* 2>/dev/null | awk '/=> \// {print $3}' | sort -u)
for lib in "${libs[@]}"; do
    base="$(basename "${lib}")"
    [ -f "${out_dir}/${base}" ] || cp -L "${lib}" "${out_dir}/"
done

loader=$(readelf -p .interp "${out_dir}/bwrap" 2>/dev/null | awk '/^ *\[/ {print $NF}')
if [ -n "${loader}" ] && [ -f "${loader}" ]; then
    [ -f "${out_dir}/$(basename "${loader}")" ] || cp -L "${loader}" "${out_dir}/"
fi

libgcc=$(gcc -print-file-name=libgcc_s.so.1 2>/dev/null)
if [ -n "${libgcc}" ] && [ -f "${libgcc}" ]; then
    [ -f "${out_dir}/libgcc_s.so.1" ] || cp -L "${libgcc}" "${out_dir}/"
fi

find "${out_dir}" -type f -exec strip --strip-unneeded {} \; 2>/dev/null || true

bwrap_v=$(rpm -q --qf '%{VERSION}' bubblewrap)
sqfuse_v=$(rpm -q --qf '%{VERSION}' squashfuse)
sqfst_v=$(rpm -q --qf '%{VERSION}' squashfs-tools)
unionfs_v=$(rpm -q --qf '%{VERSION}' unionfs)

cat > "${out_dir}/info" <<EOF
bubblewrap ${bwrap_v} (alt)
squashfuse ${sqfuse_v} (alt, fuse3)
squashfs-tools ${sqfst_v} (alt)
unionfs ${unionfs_v} (alt, fuse2)
EOF

dest_dir="${repo_dir}/internal/runtime/bundle/files"
mkdir -p "${dest_dir}"
tar -C "${build_dir}" -zcf "${dest_dir}/utils.tar.gz" utils

echo
echo "Done: ${dest_dir}/utils.tar.gz"
ls -la "${dest_dir}/utils.tar.gz"
