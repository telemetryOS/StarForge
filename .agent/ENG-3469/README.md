# ENG-3469 - Abstract package manager configuration into an interface

StarForge hardcoded x86_64 Arch Linux package configuration across the packages
phase and dependency vendoring. This change extracts it behind a `PackageSource`
interface with two implementations — x86_64 Arch Linux and aarch64 Arch Linux
ARM — selected from a new `arch` field on the build target. x86_64 behavior is
unchanged; aarch64 targets get ALARM mirrors, repo layout, keyring, and a
generated pacman.conf with `Architecture = aarch64`.

**Status:** In Review — implementation complete, verified locally, and
live-exercised on a real aarch64 build; review closure of the post-iteration-3
fixes is with the PR reviewer (see `review.md` → Closure status). Do not merge
without that closure.
**PR:** <pending — filled on open>

| Doc | What |
| --- | --- |
| [design.md](design.md) | Investigation, evidence, implementation plan |
| [implementation.md](implementation.md) | What changed, decisions, verification |
| [review.md](review.md) | Three adversary passes, findings, fixes, closure status |

## Next Agent Prompt

The PR is open for human review. `review.md` records three adversary
iterations (all `changes_requested`, all findings fixed in `0154cb7`,
`bc0833c`, `762b2b6`) and the closure gap: the iteration-3 fixes landed after
the last reviewed head (`bc0833c`), so no adversary verdict covers the final
tree. Merge closure needs a person — either the PR reviewer's own review of
the current head, or an explicit `Human acceptance:` line recorded in
`review.md`. Do not merge before that.

Live-build evidence: both targets built end-to-end in a privileged Arch
container (aarch64 via ALARM, x86_64 as symmetry); note that a target's layer
must declare its distro keyring package (`archlinuxarm-keyring` /
`archlinux-keyring`) for the chroot populate — the docs note this. Logs and
manifests live in `$TELEMETRYOS_ROOT/.testruns/exercise/starforge-eng3469/logs/`.

Verification: `go build ./...` and `go test ./...` green on the current tree.