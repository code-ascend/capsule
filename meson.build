project('capsule', version: '0.3.5',
  meson_version: '>= 1.1.0',
)

go_root = run_command('printenv', 'GOROOT', check: false)
go_bin = go_root.returncode() != 0 ? find_program('go') : find_program(join_paths(go_root.stdout().strip(), 'bin', 'go'))

version = run_command(
  'sh',
  meson.current_source_dir() / 'scripts/get-version.sh',
  meson.project_version(),
  check: false,
).stdout().strip()

ldflags = ['-X', 'capsule/internal/version.Version=' + version]

build_utils = custom_target(
  'utils.tar.gz',
  build_by_default: true,
  build_always_stale: true,
  output: 'utils.tar.gz.stamp',
  console: true,
  command: [
    'sh', '-c',
    'sh @0@ && touch @OUTPUT@'.format(
      meson.current_source_dir() / 'scripts/create-utils.sh',
    ),
  ],
)

build_runtime = custom_target(
  'capsule-runtime',
  build_by_default: true,
  build_always_stale: true,
  output: 'capsule-runtime.stamp',
  console: true,
  depends: [build_utils],
  env: ['CAPSULE_LDFLAGS=' + ' '.join(ldflags)],
  command: [
    'sh', '-c',
    'sh @0@ && touch @OUTPUT@'.format(
      meson.current_source_dir() / 'scripts/build-runtime.sh',
    ),
  ],
)

custom_target(
  'capsule',
  build_by_default: true,
  build_always_stale: true,
  output: meson.project_name(),
  console: true,
  install: true,
  install_dir: get_option('bindir'),
  depends: [build_runtime],
  command: [
    go_bin,
    'build', '-v',
    '-ldflags', ' '.join(ldflags),
    '-trimpath',
    '-o', '@OUTPUT@',
    meson.current_source_dir() / 'cmd' / 'capsule',
  ],
  env: ['CGO_ENABLED=1'],
)

subdir('po')
