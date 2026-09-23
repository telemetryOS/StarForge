# ENG-3469 implementation

## What changed

- **`engine/packagesource.go` (new)** — `PackageSource` interface (Arch,
  PacmanArchitecture, MirrorURL, ServerTemplate, Repos, KeyringPackage,
  KeyringName, ArchiveBaseURL, ArchiveArches, PackageFileURL, PacmanConf) with
  `archLinuxSource` (x86_64, `Architecture = auto`, `{mirror}/$repo/os/$arch`,
  archlinux.org JSON API, archive.archlinux.org) and `alarmSource` (aarch64,
  `Architecture = aarch64`, `{mirror}/$arch/$repo`, mirror directory-listing
  resolution, no archive). URL fields are injectable for tests.
- **`config/project.go`** — `Target.Arch` yaml field (`arch: aarch64`).
- **`actions/context.go`** — `BuildContext.Arch`; Collect normalizes it
  (empty → x86_64).
- **`engine/builder.go`** — Collect resolves the source, stores `ctx.Arch`,
  and rejects pinned packages on archive-less distros before any network work;
  EnsureDeps callers pass the target arch.
- **`engine/cache.go`** — phase-1 hash includes `arch=` so flipping arch
  invalidates phase 1..8 (InvalidateFrom cascades).
- **`engine/phase_packages.go`** — pacman.conf, mirrorlist, host
  `pacman-key --populate`, chroot `--populate`, and archive installs all read
  the source; `installFromArchive` fails loudly when the distro has no archive.
- **`engine/deps.go`** — `EnsureDeps(arch, groups...)`; keyring vendor package
  and check file derived from the source; vendorPkg gains a `source` field
  (nil = x86_64 Arch host tooling); `extractPkg` sniffs zstd/xz magic bytes
  (ALARM ships .pkg.tar.xz) via `github.com/ulikunitz/xz`.
- **`engine/qemu.go`** — run-group vendoring passes `x86_64` (host tooling,
  no target keyring).
- **`docs/content/docs/guide/project-structure.md`** — documents `arch` and
  the pinned-package limitation on aarch64.
- **`engine/packagesource_test.go` (new)** — source mapping, pacman.conf
  content for both distros, phase-1 hash arch sensitivity, per-distro keyring
  vendor pkg/check, archive-less pinned failure, ALARM listing resolution
  (httptest), version comparison, real xz extraction round-trip.

## Key decisions

- `Architecture = auto` kept for x86_64 so the generated conf is byte-identical
  to today's; ALARM pins `aarch64` because the build host is x86_64.
- ALARM package files resolve from the mirror listing (`{mirror}/{arch}/{repo}/`)
  since ALARM has no JSON API; the listing URL uses the target arch while the
  filename arch token is the package arch ("any").
- Pinned packages on aarch64 fail at Collect (before any network work) because
  ALARM has no versioned archive.
- Host build tooling always vendors from x86_64 Arch Linux; only the keyring
  follows the target distro.
- Explicitly out of scope: ARM QEMU firmware, cross-arch chroot script
  execution (qemu-user), and multi-arch host tooling.

## Verification

- `go build ./...` — zero errors (CLAUDE.md compile gate).
- `go test ./...` — all packages pass (canonical gate), including 12 new tests.
- CLI smoke (built binary + temp project): `inspect x86 packages` unchanged;
  `inspect arm packages` collects fine with `arch: aarch64`;
  `inspect bad` fails with `unsupported target arch "armv7h"`; `inspect
  armpinned` fails with `pinned packages are not supported on arch aarch64`.
- Live ALARM path (temporary probe, not committed): `EnsureDeps("aarch64",
  "build")` vendored the full toolchain plus `archlinuxarm-keyring` resolved
  from the live mirror listing
  (`http://mirror.archlinuxarm.org/aarch64/core/archlinuxarm-keyring-20240419-2-any.pkg.tar.xz`),
  downloaded and extracted it through the xz path; the check file
  `usr/share/pacman/keyrings/archlinuxarm.gpg` was present and its content
  verified with host gpg (master key 77193F152BDBE6A6; package ships
  archlinuxarm.gpg/-trusted/-revoked, confirming the populate name).
  `PackageSourceFor("aarch64").PacmanConf` output verified live; ALARM repo
  endpoints implied by the conf (aarch64/core/core.db, aarch64/extra/extra.db)
  both return HTTP 200 and packages ship .sig files, consistent with
  `SigLevel = Required DatabaseOptional`.

### Skips and limitations

- A full aarch64 pacstrap build could not run on the dev host: the vendored
  x86_64 Arch binaries require GLIBC >= 2.38 (host has older), and overlayfs/
  root are unavailable. The failing step was host-side `pacman-key --init` —
  identical failure exists pre-change on this host for x86_64 builds; not a
  regression of this change. Everything downstream of the vendored toolchain
  (mirror, listing resolution, download, xz extraction, keyring file naming,
  conf generation) was verified with live artifacts.
- Visual Evidence Gate: not applicable — no browser/UI surface.
- Local Fleet Gate: substituted by the canonical gate. StarForge is a
  standalone CLI with no TelemetryOS service surface (no Aion/Gateway/UI
  integration), so the repo's own `go build`/`go test` gate is the fleet
  substitution; the CLI smoke above exercised the changed surface directly.

## Remaining risk

- End-to-end ARM image build has not been executed on real hardware/CI; the
  change is config plumbing that fails loudly at well-defined points.
- ALARM mirrors are plain HTTP (the geo HTTPS host has a certificate mismatch);
  noted in code comments.