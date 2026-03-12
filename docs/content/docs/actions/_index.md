---
title: "Actions Reference"
linkTitle: "Actions"
weight: 6
---


Actions are the building blocks of StarForge layers. Each step in a `layer.yaml` specifies an action and its configuration. This page covers step fields, all available actions, override semantics, custom YAML tags, and build phase ordering.

For YAML quoting rules, systemd field naming, and common patterns, see the [YAML Reference](../yaml-reference/).

## Step Fields

Every step supports these common fields in addition to the action-specific fields:

| Field | Type | Description |
|-------|------|-------------|
| `action` | string | **Required.** The action name (e.g., `pacman-add`, `file-create`). |
| `label` | string | Optional. Human-readable label shown in build output and `starforge inspect`. |
| `layer_source` | string | Optional. Git repo URL (`*.git#ref`) or archive URL to resolve before action dispatch. |
| `layer_script` | string | Optional. Inline shell script to run in the resolved `layer_source` directory on the host. Requires `layer_source`. |
| `layer_script_path` | string | Optional. Path to a shell script (relative to layer dir) to run in the resolved `layer_source` directory. Requires `layer_source`. Mutually exclusive with `layer_script`. |

### `layer_source`

Fetches external files for a step. The resolved directory replaces the layer directory for that step, so `layer_path` references are resolved against the source.

```yaml
# Git source with tag
- action: file-create
  layer_source: https://github.com/org/configs.git#v2.0
  layer_path: ./etc/myapp.conf
  path: /etc/myapp.conf

# Archive source
- action: file-create
  layer_source: https://example.com/resources-v2.tar.gz
  layer_path: ./etc/config
  path: /etc/myapp
```

Sources are cached in `.starforge/cache/sources/{sha256(url)}/` with a `.resolved` marker to avoid re-fetching.

### `layer_script`

Runs a host-side build script in the resolved source directory before the action is dispatched. Useful for compiling or transforming source files.

```yaml
- action: file-create
  layer_source: https://github.com/org/app.git#v1.0
  layer_script: |
    make build
    strip ./output/myapp
  layer_path: ./output/myapp
  path: /usr/local/bin/myapp

# Or reference a script file
- action: file-create
  layer_source: https://github.com/org/app.git#v1.0
  layer_script_path: ./build-app.sh
  layer_path: ./output/myapp
  path: /usr/local/bin/myapp
```

## Actions by Category

### Package Management

| Action | Description | Page |
|--------|-------------|------|
| [pacman-add](pacman-add/) | Add pacman packages | Accumulate |
| [pacman-remove](pacman-remove/) | Remove packages added by earlier layers | Remove |

### Partition Management

| Action | Description | Page |
|--------|-------------|------|
| [partition-add](partition-add/) | Define disk partitions | Accumulate |
| [partition-remove](partition-remove/) | Remove a partition by name | -- |
| [partition-change](partition-change/) | Modify partition fields by name | -- |

### File Operations

| Action | Description | Page |
|--------|-------------|------|
| [file-create](file-create/) | Create files from inline content or layer files | Replace-on-path |
| [file-edit](file-edit/) | Modify existing file content | Accumulate |
| [file-copy](file-copy/) | Copy files within the target filesystem | Accumulate |
| [file-move](file-move/) | Move/rename files within the target | Accumulate |
| [file-delete](file-delete/) | Remove files or directories | Accumulate |
| [file-link](file-link/) | Create symbolic or hard links | Accumulate |
| [file-mkdir](file-mkdir/) | Create directories | Accumulate |
| [file-permissions](file-permissions/) | Set file mode (chmod) | Accumulate |
| [file-ownership](file-ownership/) | Set file ownership (chown) | Accumulate |

### System Configuration

| Action | Description | Page |
|--------|-------------|------|
| [system-hostname](system-hostname/) | Set the system hostname | Replace |
| [system-locale](system-locale/) | Set the system locale | Replace / Accumulate |
| [system-timezone](system-timezone/) | Set the system timezone | Replace |
| [system-keymap](system-keymap/) | Set the keyboard map | Replace |
| [system-user](system-user/) | Create or modify a user account | Merge-on-name |
| [system-group](system-group/) | Create an explicit group | Replace-on-name |

### Systemd

| Action | Description | Page |
|--------|-------------|------|
| [systemd-service](systemd-service/) | Manage systemd service units | Accumulate |
| [systemd-mount](systemd-mount/) | Manage systemd mount units | Accumulate |
| [systemd-timer](systemd-timer/) | Manage systemd timer units | Accumulate |
| [systemd-socket](systemd-socket/) | Manage systemd socket units | Accumulate |
| [systemd-slice](systemd-slice/) | Manage systemd slice units | Accumulate |
| [systemd-target](systemd-target/) | Set default target or manage target units | Replace / Accumulate |
| [systemd-boot-install](systemd-boot-install/) | Configure systemd-boot | Mixed (loader replaces, entries accumulate) |

### Scripts

| Action | Description | Page |
|--------|-------------|------|
| [run](run/) | Execute a script in chroot during build | Accumulate |
| [layer-run](layer-run/) | Execute a host-side script during Collect | Accumulate |

### Installer

| Action | Description | Page |
|--------|-------------|------|
| [install-payload](install-payload/) | Bundle a built target as a compressed payload | -- |
| [install-server](install-server/) | Configure the installer REST API server | -- |
| [install-client](install-client/) | Configure the installer TUI client | -- |

## `!include` -- File Inclusion

The `!include` tag lets you split large layers into multiple YAML files. It can appear anywhere in a layer's YAML.

### Scalar form

Include an entire file:

```yaml
steps:
  - !include ./packages.yaml
  - !include ./services.yaml
  - action: system-hostname
    hostname: my-device
```

### Mapping form

Include a specific portion of a file using a dot-separated path:

```yaml
steps:
  - !include
    layer_path: ./shared.yaml
    yaml_path: common.steps
```

The `yaml_path` navigates into the loaded YAML structure using mapping keys and sequence indices.

### List splicing

When an `!include` in a sequence resolves to another sequence, items are spliced (flattened) into the parent list:

```yaml
# packages.yaml (a sequence)
- action: pacman-add
  packages: [base, linux]
- action: pacman-add
  packages: [sudo, openssh]

# layer.yaml -- both steps end up in the steps list, not nested
steps:
  - !include ./packages.yaml
  - action: system-hostname
    hostname: my-device
```

### URL includes

```yaml
steps:
  - !include https://example.com/shared-steps.yaml
```

### Limits

Includes nest up to 10 levels deep. Each nested include resolves paths relative to the included file's directory, not the root file. Circular includes hit the depth limit and produce an error.

## Override Semantics

When the same action appears in multiple layers, StarForge uses these rules to combine them:

| Semantics | Actions | Behavior |
|-----------|---------|----------|
| **Replace** | `systemd-target` (set-default), `system-hostname`, `system-locale` (locale), `system-timezone`, `system-keymap` | Last layer wins entirely. |
| **Replace-on-path** | `file-create` (single files) | Later layer replaces earlier file at the same path. Directory copies always accumulate. |
| **Replace-on-name** | `system-group` | Later layer redefines a group with the same name. |
| **Mixed** | `systemd-boot-install` | Loader config replaces; boot entries accumulate. |
| **Remove** | `pacman-remove` | Removes matching items from the accumulated list. Unmatched items are silently ignored. |
| **Merge-on-name** | `system-user` | Later layer referencing the same user modifies the existing user. Supports `!add`/`!remove` on `groups`. |
| **Accumulate** | Everything else (including `partition-add`) | Values from all layers are combined in order. |

## Custom YAML Tags

StarForge defines 10 custom YAML tags for merge control, file editing, and systemd overrides. See the [YAML Reference](../yaml-reference/) for the complete tag documentation.

## Build Phase Order

Actions are collected during the Collect stage and then executed in these phases:

| Phase | Name | Actions Executed |
|-------|------|-----------------|
| 0 | `preinstall` | `system-keymap` (writes vconsole.conf before pacstrap) |
| 1 | `packages` | `pacman-add` (runs pacstrap with deduplicated package list) |
| 2 | `sysconfig` | `system-hostname`, `system-locale`, `system-timezone`, `system-keymap` (summary only) |
| 3 | `users` | `system-group`, `system-user` |
| 4 | `files` | `file-mkdir`, `file-create` (layer copies and inline), `file-edit`, `file-copy`, `file-move`, `file-link`, `file-delete`, all systemd unit file creation |
| 5 | `permissions` | `file-ownership`, `file-permissions` |
| 6 | `services` | `systemd-service`, `systemd-mount`, `systemd-timer`, `systemd-socket`, `systemd-slice`, `systemd-target` (enable/disable/mask/set-default) |
| 7 | `boot` | `systemd-boot-install` |
| 8 | `scripts` | `run` |

Within phase 4 (files), operations execute in this order:

1. `file-mkdir` -- create directories
2. Layer copies -- `file-create` with directory `layer_path`, systemd unit files from inline definitions
3. File creates -- `file-create` with content or single-file `layer_path`
4. File edits -- `file-edit`
5. Internal copies -- `file-copy` (within target filesystem)
6. Moves -- `file-move`
7. Links -- `file-link`
8. Deletes -- `file-delete`

Within phase 5 (permissions), ownership changes (`file-ownership`) run before permission changes (`file-permissions`).

## URL Support

Several fields support HTTP(S) URLs in addition to local paths:

- **Target `layers:`** -- git repos, archives, and remote layer URLs
- **Step `layer_source`** -- git repos and archive URLs
- **`!include`** -- remote YAML files
- **`run` `script_path`** -- remote script files
- **`file-create` `layer_path`** -- remote files (single files only, not directories)

URLs are downloaded once and cached locally. See the [YAML Reference](../yaml-reference/) for details on URL-based includes.
