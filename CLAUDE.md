# CLAUDE.md -- StarForge Recovery

## What Is This?

StarForge is a declarative Arch Linux OS image builder. It reads a `starforge.yaml` project file, composes ordered layers of YAML configuration, and produces bootable disk images. The original repository was completely lost -- **nothing was in git.** The entire codebase has been reconstructed from AI conversation transcripts where the code was previously discussed and developed. The single commit `c0f2b09` ("the dark ages...") is the initial dump of whatever could be extracted from those transcripts. We are incrementally restoring function by cross-referencing against the `../Edge-OS` project (a surviving real-world StarForge project with 7 layers), restored documentation in `docs/`, and internal consistency between the engine, config, and action code.

## Recovery Ground Rules

- **Cross-reference everything.** Before writing or modifying code, check `docs/actions/`, `../Edge-OS` layer files, `engine/builder.go` (which consumes BuildContext), and `config/layer.go` (which defines all YAML step types). These are the sources of truth.
- **Don't guess field names.** The config types in `config/layer.go` define the exact YAML schema. Action implementations must match these types exactly.
- **Centralized registration.** All action `Register()` calls live in `actions/actions.go` in a single `init()` alongside the `Action` interface and registry. Never add `init()` to individual action files.
- **Declarative actions.** Actions populate `BuildContext` during the Collect phase. They never perform side effects (no file I/O, no exec). The engine reads BuildContext during the Build phase.

## Module & Toolchain

- **Module:** `github.com/telemetryos/starforge`
- **Go version:** 1.25.5
- **Build:** `go build ./cmd/starforge`
- **Test:** `go test ./...`
- **Verify compilation:** `go build ./...` (must produce zero errors)

## Directory Layout

```
cmd/starforge/       Entry point
commands/            Cobra CLI commands (build, run, clean, export, write, init, list, status, chroot, inspect)
config/              YAML parsing: Project, Target, Layer, Step types, variable substitution, remote fetching
actions/             Declarative action implementations (32 registered actions + layer-run (handled by builder) + registry + helpers)
engine/              Build orchestration: overlay management, phase execution, packaging, QEMU, deps
installer/           Installer server/client for writing images to devices
docs/                Restored documentation (actions reference, YAML reference)
```

## Architecture

### Build Pipeline

```
starforge build <target>
  └─ engine.NewBuilder(project)
       ├─ builder.Collect(target, verbose)    # parse layers, run actions → populate BuildContext
       │    └─ for each layer:
       │         config.LoadLayerRaw → SubstituteVars → DecodeStep → action.Execute(step, layerDir, ctx)
       └─ builder.Build(name, target, clean)  # execute 9 phases using BuildContext
            └─ phase 0..8 in order, overlayfs snapshots between phases
```

### The 9 Build Phases

| # | Phase | What Runs |
|---|-------|-----------|
| 0 | preinstall | vconsole.conf (keymap before pacstrap) |
| 1 | packages | pacstrap with deduplicated package list |
| 2 | sysconfig | hostname, locale, timezone, keymap |
| 3 | users | groups then users |
| 4 | files | mkdir → layer copies → creates → edits → internal copies → moves → links → deletes |
| 5 | permissions | ownership then chmod |
| 6 | services | mask → enable → disable → user-enable → user-disable → default-target |
| 7 | boot | systemd-boot loader.conf + entries |
| 8 | scripts | run actions via arch-chroot |

### Key Types

- `config.Step` -- has one typed sub-struct per action (only one populated per step)
- `config.Layer` -- fully-decoded `[]Step`; `config.RawLayer` has raw `[]*yaml.Node` for variable substitution
- `config.Target` -- layers list, args, env, optional QEMU config
- `actions.BuildContext` -- central accumulator struct with fields for every phase
- `actions.Action` -- interface: `Name() string` + `Execute(step, layerDir, ctx) error`

### Custom YAML Tags

| Tag | Where | Purpose |
|-----|-------|---------|
| `!include` | Anywhere in layer YAML | Inline external YAML files |
| `!add` / `!remove` / `!replace` | `Mergeable[T]` fields (e.g. user groups) | Cross-layer list merging |
| `!append` / `!prepend` | `file-edit` content | Insert at start/end of file |
| `!before` / `!after` | `file-edit` content | Insert relative to regex match |
| `!truncate_before` / `!truncate_after` | `file-edit` content | Remove content around match |
| `!replace` | Systemd unit section values | Clear-then-set for drop-in overrides |

## Actions (32 registered + layer-run)

All registered in `actions/actions.go`. Each maps to a `config.*Step` type in `config/layer.go`.

| Category | Actions |
|----------|---------|
| Packages | `pacman-add`, `pacman-remove` |
| Files | `file-create`, `file-edit`, `file-copy`, `file-move`, `file-delete`, `file-link`, `file-mkdir`, `file-permissions`, `file-ownership` |
| System | `system-hostname`, `system-locale`, `system-timezone`, `system-keymap`, `system-user`, `system-group` |
| Systemd | `systemd-service`, `systemd-mount`, `systemd-timer`, `systemd-socket`, `systemd-slice`, `systemd-target`, `systemd-boot-install` |
| Partitions | `partition-add`, `partition-remove`, `partition-change` |
| Scripts | `run`, `layer-run` (builder-handled) |
| Installer | `install-server`, `install-client`, `install-payload` |
| Multi-target | `install-embed` |

### Shared Helpers

- `actions/systemd_unit_exec.go` -- `executeSystemdUnit()` shared by all 5 systemd unit types
- `actions/render_unit.go` -- `RenderUnit()` renders INI-format unit files from `map[string]map[string]any`
- `actions/resolve.go` -- `ParseSize()`, `ReadLayerFile()`, `copyPartitions()`, `isValidPartitionType()`

## Reference Materials

- **`docs/actions/`** -- 32 markdown files, one per action + README overview
- **`../Edge-OS`** -- surviving real-world StarForge project (7 layers, 18 action types)
  - Uses growable partition syntax (`256M+`, `7G+`, `100%`)
  - Uses `no_password: true` and `groups: !add [...]` merge syntax
  - Systemd unit fields use CamelCase directly (no snake_case conversion)

## Recovery Status

### Restored (compiles clean)
- All 34 action implementations
- Centralized `actions/register.go`
- All 10 commands fully implemented: build, run, clean, export, write, init, list, status, chroot, inspect
- Config fixes: `SystemUserStep.NoPassword`, `RunStep.Env`
- `ParseSize` `+` suffix for growable partitions

### Extracted from AI Transcripts (initial commit)
Everything was reconstructed from AI conversation transcripts. The initial commit contained:
- `engine/` -- builder, overlay, cache, cleanup, deps, installer, mount, package, partitions, qemu
- `config/` -- layer.go, project.go, include.go, source.go, vars.go (minor field additions needed)
- `commands/` -- build, run, clean, export, write, root
- `cmd/starforge/main.go`
- `installer/client/`, `installer/manifest.go`, `installer/server/routes/installations/`
- 5 action files: `file_create.go`, `install_server.go`, `partition_change.go`, `systemd_target.go`, `systemd_unit_exec.go`

Subsequent recovery sessions restored the remaining 29 action files, stub commands, docs, and config fixes.

### Potentially Still Missing
- Tests: 19 test files exist across `actions/`, `config/`, `engine/`; gaps remain (no command tests, no packaging/overlay/installer tests)
- CI/CD configuration
- Any tooling or scripts that lived outside the Go source tree
