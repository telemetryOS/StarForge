# ENG-3469 review

Full lane (lite-lane classifier: `LANE=full LINES=1021 BLOCKED=3
REASON=blocked-surface PATHS=engine/cache.go,go.mod,go.sum` — cache-hash logic
and a new module dependency are production changes).

## Adversary pass — iteration 1 (pre-commit)

Reviewed head: working tree of branch `mucahit/eng-3469` (uncommitted; identical
to commit `mucahit/eng-3469` first production commit)
Integration base: `origin/master` @ 1b60fca5599d870758466345800de92f31cd3468
Reviewer lane: cursor-agent via `adversary-review.sh run --lane auto`
(probe: host=other, cursor present+authed, recommended=cursor); the ladder
fell through to the Codex lane after the cursor channel declined; run was
read-only. Bundle: self-contained prompt + complete diff vs master.
Note: the runner reported `SWEEP=dirty` because the author ran `go mod tidy`
in the worktree while the reviewer was running; the verdict content predates
and is unaffected by that mutation, and the go.mod change was already in the
reviewed bundle.

```json
{
  "verdict": "changes_requested",
  "iteration": 1,
  "adversary": "cursor-agent",
  "coverage_note": "Statically reviewed the complete diff plus relevant surrounding builder, chroot, cache, packaging, and dependency-vendoring call sites. Verified x86_64 pacman.conf compatibility, source selection, cache-hash wiring, pinned-package rejection logic, and host-tool package selection. Tests and live-network behavior were not executed under the read-only constraint.",
  "findings": [
    {
      "severity": "critical",
      "file": "engine/packagesource.go",
      "line": 193,
      "claim": "ALARM signing keyring bootstrapped from unauthenticated HTTP and extracted without verification; a malicious archive could overwrite vendored executables or substitute the trust root.",
      "confidence": "high"
    },
    {
      "severity": "high",
      "file": "engine/phase_packages.go",
      "line": 110,
      "claim": "aarch64 build executes aarch64 binaries in chroot without binfmt emulation preflight.",
      "confidence": "high"
    },
    {
      "severity": "medium",
      "file": "engine/deps.go",
      "line": 456,
      "claim": "Compression sniffing panics on a truncated 4- or 5-byte package.",
      "confidence": "high"
    },
    {
      "severity": "low",
      "file": "engine/packagesource_test.go",
      "line": 171,
      "claim": "Tests do not exercise rejection paths through Builder.Collect nor prove aarch64 vendoring keeps non-keyring tooling on the x86_64 source.",
      "confidence": "high"
    }
  ]
}
```

Author sanity-check and dispositions:

- **F1 (critical) — accepted, fixed.** `KeyringPackageSHA256()` added to the
  interface: the ALARM keyring package is digest-pinned
  (sha256 of `archlinuxarm-keyring-20240419-2-any.pkg.tar.xz`, taken from a
  live mirror download) and verified after download (`verifyPkgDigest`,
  fail-loud with guidance when ALARM publishes a newer keyring). Keyring
  extraction is confined: `extractKeyringPkg` extracts into an isolated temp
  dir and copies only regular files under `usr/share/pacman/keyrings/` into
  the vendor tree, so a tampered archive cannot overwrite vendored
  executables. Tests: digest match/mismatch, executable-overwrite attempt,
  routing. **Declined portion:** generally hardening `extractPkg` path/link
  handling for all vendored x86 packages — pre-existing behavior over TLS,
  out of ticket scope; recorded as follow-up, not absorbed here.
- **F2 (high) — accepted, fixed.** `RequireBinfmt(ctx.Arch)` preflight in
  `Builder.execute` fails loudly before any filesystem work when the target
  arch differs from the host and no `qemu-<arch>` binfmt handler is
  registered; prerequisite documented in the project-structure guide. Note:
  phases that chroot into locale-gen/systemctl/bootctl were already arch-
  sensitive before this ticket; the preflight makes the failure actionable.
- **F3 (medium) — accepted, fixed.** Sniff comparisons are length-guarded;
  truncated/empty archives return an error. Test covers inputs of 0–5 bytes.
- **F4 (low) — accepted, fixed.** Added Collect-level tests: unsupported arch
  rejection, aarch64 pinned-package rejection, normalized `ctx.Arch` for
  default and explicit arch; EnsureDeps routing test asserting all static
  vendor packages resolve through x86_64 Arch Linux and only the ALARM
  keyring through `alarmSource` (digest-pinned).

Verification after fixes: `go build ./...` clean; `go test ./...` all green;
CLI smoke re-run unchanged for x86_64/aarch64 inspect paths.

## Adversary pass — iteration 2

Reviewed head: `0154cb7` (branch `mucahit/eng-3469`, integration base
`origin/master` @ 1b60fca). Lane: Codex (`--lane codex`; the auto ladder's
Cursor lane was rejected by team policy: "Your team restricts model selection
to Auto"), read-only, `SWEEP=clean`, ELAPSED=383s.

Verdict: `changes_requested` — 2 high, 1 medium.

- **F2-1 (high) — accepted, fixed.** `alarmSource.Repos()` omitted ALARM's
  `alarm` (ARM enablement/board packages) and `aur` repositories. Verified
  live: `aarch64/alarm/alarm.db` and `aarch64/aur/aur.db` both return HTTP 200.
  `Repos()` now returns `core, extra, alarm, aur`, matching the stock Arch
  Linux ARM pacman.conf; `TestPacmanConf_ArchARM` asserts all four sections.
- **F2-2 (high) — accepted, fixed.** The preflight assumed a same-arch host
  needs no emulation, but all vendored host tooling is x86_64 Arch Linux, so
  an arm64 host cannot run it. Added `RequireHostToolchain()` (called in
  `Builder.execute` before `RequireBinfmt`) which fails loudly on non-amd64
  hosts, and corrected the binfmt error text, which previously suggested
  building on an aarch64 host. Test: `TestRequireHostToolchain`.
- **F2-3 (medium) — accepted, fixed.** `binfmtHandlerRegistered` was a
  filename-presence check. It now reads each registration, requires the first
  line to be exactly `enabled`, requires the `F` (fix_binary) flag that makes
  emulated exec work inside a chroot, and matches the handler by name or
  registration content (so WSL's `aarch64` entry is recognized alongside
  `qemu-aarch64`). Injectable via `binfmtHandlerRegisteredIn` with fixture
  tests for enabled, disabled, missing-F, unrelated-arch, and
  missing-directory cases.

Verification after fixes: `go build ./...` clean; `go test ./...` all green;
CLI smoke re-run unchanged.

## Adversary pass — iteration 3

Reviewed head: `bc0833c` (integration base `origin/master` @ 1b60fca). Lane:
Codex, read-only, `SWEEP=clean`, ELAPSED=514s.

Verdict: `changes_requested` — 1 high, 2 medium.

- **F3-1 (high) — accepted, fixed after the reviewed head.** Unbounded
  `io.ReadAll` on the plain-HTTP ALARM listing and unbounded `downloadFile`
  streaming, plus a digest-mismatching cache entry that stayed on disk and
  blocked every later build. Fixes: `maxListingBytes` caps both listing
  readers; `maxPackageDownloadBytes` caps downloads; `downloadFile` stages to
  `<dest>.part` and renames only after a complete, size-checked response;
  `fetchVendorPackage` verifies the pinned digest and evicts a mismatching
  cache entry so the next run re-downloads; all package/listing requests use
  a shared client with a 10-minute timeout. Tests: poisoned-entry eviction and
  re-download, oversized response rejected with no cached file, stage-then-
  rename.
- **F3-2 (medium) — accepted, fixed after the reviewed head.** `BuildResult`
  lost `Arch`, so `EnsurePackaged` returned a reconstructed context claiming
  x86_64. Fix: `BuildResult.Arch` added and mapped in both
  `contextToBuildResult` and `buildResultToContext` (the cache cross-check
  test covers the pair). Test: mapping and save/load round trip.
- **F3-3 (medium) — accepted, fixed after the reviewed head.**
  `RequireHostToolchain`/`RequireBinfmt` ran after `EnsureDeps`, so download
  or network errors could mask the unsupported-host error. Both preflights
  now run before any dependency download.

Verification after these fixes: `go build ./...` clean; `go test ./...` all
green; CLI smoke unchanged.

### Closure status: UNRESOLVED — stopped for a person

The iteration-3 fixes are production changes that landed after the reviewed
head (`bc0833c`), and the three-iteration cap forbids a fourth adversary pass,
so no verdict covers them. Readiness is invalidated: the branch must not be
treated as review-closed. The fixes are symbolically committed on top of the
reviewed head; the repository's review-closure helper is not invoked for a
human decision.

On 2026-09-23 the user directed the PR handoff (`$pr`), superseding the
"do not open a PR" hold recorded earlier the same day. The PR therefore opens
with this gap disclosed rather than resolved: the post-`bc0833c` fixes are
unreviewed by the adversary, and the PR review itself (human reviewer) is the
closure path. Do not merge until a person accepts the post-`bc0833c` fixes as
review-closed — recorded as
`Human acceptance: <who> accepted the post-bc0833c fixes at <fix-SHA> as
review-closed on <date>` in this file — or a review of the current head
returns a pass verdict recorded here with its reviewed head.