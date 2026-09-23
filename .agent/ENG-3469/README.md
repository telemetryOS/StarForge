# ENG-3469 - Abstract package manager configuration into an interface

StarForge hardcoded x86_64 Arch Linux package configuration across the packages
phase and dependency vendoring. This change extracts it behind a `PackageSource`
interface with two implementations — x86_64 Arch Linux and aarch64 Arch Linux
ARM — selected from a new `arch` field on the build target. x86_64 behavior is
unchanged; aarch64 targets get ALARM mirrors, repo layout, keyring, and a
generated pacman.conf with `Architecture = aarch64`.

**Status:** In Progress (implementation complete, review in flight)
**PR:** none yet — standalone `$fix` stops at a local commit

| Doc | What |
| --- | --- |
| [design.md](design.md) | Investigation, evidence, implementation plan |
| [implementation.md](implementation.md) | What changed, decisions, verification |
| [review.md](review.md) | Adversary review verdict and findings |

## Next Agent Prompt

Implementation is complete and verified on branch `mucahit/eng-3469` (worktree
`~/.codex/worktrees/StarForge-ENG-3469`, integration base `master`). The
canonical gate (`go build ./...`, `go test ./...`) is green and the adversary
review is recorded in `review.md`. Pickup point: run `$pr` to open/update the
PR and move the ticket to review, or `$review` if another adversarial pass is
wanted. Do not weaken the gate; a full aarch64 pacstrap build could not be
exercised on the dev host (see implementation.md verification notes for the
exact limitation and what was proven instead).