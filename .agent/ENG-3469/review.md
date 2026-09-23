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

<pending>