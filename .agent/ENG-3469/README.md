# ENG-3469 - Abstract package manager configuration into an interface

StarForge hardcoded x86_64 Arch Linux package configuration across the packages
phase and dependency vendoring. This change extracts it behind a `PackageSource`
interface with two implementations — x86_64 Arch Linux and aarch64 Arch Linux
ARM — selected from a new `arch` field on the build target. x86_64 behavior is
unchanged; aarch64 targets get ALARM mirrors, repo layout, keyring, and a
generated pacman.conf with `Architecture = aarch64`.

**Status:** In Progress — implementation complete and verified locally, but
**review closure is UNRESOLVED and stopped for a person** (iteration-3 fixes
landed after the last reviewed head; the three-iteration cap forbids a fourth
adversary pass). See `review.md` → "Closure status".
**PR:** none — do not open one until a person closes the review gap.

| Doc | What |
| --- | --- |
| [design.md](design.md) | Investigation, evidence, implementation plan |
| [implementation.md](implementation.md) | What changed, decisions, verification |
| [review.md](review.md) | Three adversary passes, findings, fixes, closure status |

## Next Agent Prompt

Read `review.md` first: three adversary iterations ran (all `changes_requested`);
every accepted finding is fixed in commits `0154cb7`, `bc0833c`, `762b2b6`, and
the canonical gate (`go build ./...`, `go test ./...`) is green on the current
tree with 40+ tests covering the new behavior.

The branch `mucahit/eng-3469` (worktree `~/.codex/worktrees/StarForge-ENG-3469`,
integration base `master` @ 1b60fca) is **not** review-closed: the fixes for
iteration 3's findings (`762b2b6`) are production changes after the last
reviewed head (`bc0833c`) and no fourth adversary pass is permitted. Do not
claim pass, do not open a PR, and do not merge until a person either accepts
those fixes as review-closed (record the exact `Human acceptance:` line in
`review.md`) or directs a further review cycle, recorded there with the reason.

Blockers/limits to carry forward: a full aarch64 pacstrap build was not run on
this host (vendored x86_64 Arch binaries need GLIBC ≥ 2.38, which the dev host
lacks) — see `implementation.md` for what was verified instead; ALARM mirrors
are plain HTTP, which is why the keyring package is digest-pinned.