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

[ -f "${out_dir}/ld-linux-x86-64.so.2" ] || cp -L /lib64/ld-linux-x86-64.so.2 "${out_dir}/"
[ -f "${out_dir}/libgcc_s.so.1" ]        || cp -L /usr/lib64/libgcc_s.so.1   "${out_dir}/" 2>/dev/null || \
                                            cp -L /usr/lib/libgcc_s.so.1     "${out_dir}/"

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

rm -rf "${dest_dir}/utils"
cp -a "${out_dir}" "${dest_dir}/utils"

echo
echo "Done: ${dest_dir}/utils.tar.gz"
ls -la "${dest_dir}/utils.tar.gz"
