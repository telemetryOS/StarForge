Useful unrelated parent work: None identified

# ENG-3469 design — Abstract package manager configuration into an interface

## Summary

StarForge hardcodes x86_64 Arch Linux assumptions across the packages phase and
dependency vendoring. This ticket extracts those behind a `PackageSource`
interface so aarch64 Arch Linux ARM (ALARM) builds use their own mirrors, repo
layout, keyring, and pacman.conf, selected from the target `arch`.

## Evidence inspected

- `engine/phase_packages.go` — `archiveURL` const, `pacmanConf()` (`Architecture
  = auto`, `[core]`/`[extra]` with `Server = geo.mirror.pkgbuild.com/$repo/os/$arch`),
  mirrorlist write (`$repo/os/$arch` layout), `--populate archlinux` (host, line
  279, and chroot, line 109), `installFromArchive` (x86_64/any fallback,
  `archive.archlinux.org`), `resolveLatestPkgrel` (archive listing regex hardcodes
  `x86_64|any` and `.tar.zst`).
- `engine/deps.go` — `vendorPackages` includes `archlinux-keyring` (core/any);
  `vendorChecks` pins `usr/share/pacman/keyrings/archlinux.gpg`; `archMirror`
  const; `resolvePackageURL` uses the archlinux.org JSON API and
  `{mirror}/{repo}/os/x86_64/{filename}` layout; `extractPkgTarZst` only
  decompresses zstd.
- `engine/builder.go` — `Collect(target)` creates the `BuildContext` (no arch
  field today); `EnsureDeps("build")` called from `EnsurePackaged` (line 247,
  ctx from build result) and `execute` (line 688, ctx in scope); `resolvePkgRels`
  resolves pinned pkgrels against the x86 archive.
- `config/project.go` — `config.Target` has no `arch` field.
- `engine/cache.go` — phase-1 hash covers package names/versions only, so an
  arch flip would not invalidate the cached rootfs.
- Live network probes (2026-09-23): `http://mirror.archlinuxarm.org/aarch64/core/`
  serves a directory listing (302 → regional mirror; HTTPS on the geo host has a
  cert mismatch, so ALARM access must use plain HTTP); all ALARM packages are
  `.pkg.tar.xz` (e.g. `archlinuxarm-keyring-20240419-2-any.pkg.tar.xz`); ALARM
  repo layout is `{mirror}/{arch}/{repo}` (no `os` segment); ALARM has no
  archlinux.org-style JSON API and no versioned package archive.

## Hypothesis / current behavior

Current behavior (fact, not hypothesis): every package-phase input is fixed to
x86_64 Arch Linux. ARM builds are impossible; there is no target-arch concept in
config, BuildContext, or the cache.

## Implementation plan

1. **`engine/packagesource.go` (new)** — `PackageSource` interface:
   `Arch()`, `PacmanArchitecture()`, `MirrorURL()`, `ServerTemplate()`
   (`.../$repo/os/$arch` vs `.../$arch/$repo`), `Repos()`, `KeyringPackage()`,
   `KeyringName()`, `ArchiveBaseURL()` (empty = no archive),
   `ArchiveArches()`, `PacmanConf(cacheDir, gpgDir)` (shared writer).
   `PackageSourceFor(arch)` returns `archLinuxSource` for `x86_64`/empty,
   `alarmSource` for `aarch64`, error otherwise.
2. **`config.Target.Arch`** — new `arch` yaml field; `Collect` resolves the
   source (unsupported arch fails at Collect), stores `ctx.Arch`, and rejects
   pinned packages on distros without an archive (fail-loud before any network).
3. **`actions.BuildContext.Arch`** — new field; `HashPhase` phase 1 includes
   `arch=` so flipping arch invalidates phase 1..8 (`InvalidateFrom` already
   cascades).
4. **`engine/phase_packages.go`** — select source from `ctx.Arch`; pacman.conf,
   mirrorlist, host `--populate`, and chroot `--populate` all read the source;
   `installFromArchive`/`resolveLatestPkgrel` take the archive base URL and
   arch list from the source.
5. **`engine/deps.go`** — `EnsureDeps(arch, groups...)`; keyring vendor package
   and `vendorChecks` keyring path derived from the source; ALARM keyring
   resolved via mirror directory listing (latest version, redirect-following,
   plain http); package extraction sniffs zstd vs xz magic (ALARM ships
   `.pkg.tar.xz`) using `github.com/ulikunitz/xz`.
6. **Callers** — builder `execute`/`EnsurePackaged` pass ctx/target arch;
   `qemu.go` run-group vendoring passes empty arch (host tooling, no keyring).
7. **Docs** — document the `arch` target field in the YAML reference.

Out of scope (state explicitly): running ARM images under QEMU (different
firmware), `scripts` phase cross-arch chroot (needs qemu-user), and multi-arch
vendored host tooling — host tooling stays x86_64 Arch packages.

## Verification plan

- `go build ./...` (CLAUDE.md compile gate) and `go test ./...` (canonical gate).
- New unit tests: `PackageSourceFor` mapping/errors; pacman.conf content for
  both sources (x86 byte-identical semantics: `Architecture = auto`, pkgbuild
  servers; ALARM: `Architecture = aarch64`, `$arch/$repo` layout); phase-1 hash
  changes with arch; ALARM listing URL resolution against an httptest mirror;
  xz extraction; pinned-package rejection on ALARM at Collect level.
- Local Fleet Gate: `$local-fleet-test` substitution — StarForge is a standalone
  CLI with no TelemetryOS service surface; the canonical gate substitutes,
  stated explicitly in the trail. A real aarch64 build requires network pacstrap
  of ALARM packages; if feasible, smoke it; otherwise record why not.
- Visual Evidence Gate: not applicable — no browser/UI surface.

## Risks / unknowns

- pacstrap on an x86_64 host for an aarch64 rootfs is untested end-to-end here
  (no ARM target project available); the change is config-plumbing and fails
  loudly at well-defined points if a mirror/keyring step is wrong.
- ALARM mirrors are plain HTTP (geo host HTTPS cert mismatch observed).
- New module dependency `github.com/ulikunitz/xz` (pure Go, no transitive deps).

## Complexity and agent suitability

Refactor with clear seams, confined to `engine/` + small config/context touch +
tests. Agent-suitable.

## Priority

No priority set on the ticket; feature-blocking work for ARM image builds.

## Recommended next step

Implement per plan in `~/.codex/worktrees/StarForge-ENG-3469` (branch
`mucahit/eng-3469`), full lane.