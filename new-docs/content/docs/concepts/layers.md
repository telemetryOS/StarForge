---
title: "Layers"
weight: 2
---


A layer is a directory containing a `layer.yaml` file. The directory can also contain additional files and subdirectories referenced by actions -- files to copy into the target, scripts to run, systemd units, and so on.

Layers are the primary way to organize and compose an OS image. Each layer defines a list of steps, and layers are processed in order during the [Collect](build-pipeline/) stage.

## Layer File

```yaml
vars:
  app_name: myapp

imports:
  - version

exports:
  - app_name

steps:
  - action: pacman-add
    packages: [base, linux, linux-firmware]

  - action: system-hostname
    hostname: ${{ app_name }}-device

  - action: file-create
    path: /etc/app-version
    content: ${{ version }}
```

| Field | Type | Description |
|-------|------|-------------|
| `steps` | list | Ordered list of action steps. |
| `vars` | map | Default variable values. See [Variables](variables/). |
| `imports` | list | Required variables -- error if missing at layer start. |
| `exports` | list | Variables to propagate to subsequent layers. If omitted, all variables propagate. |

For full details on the variable system, see [Variables](variables/).

## Step Fields

Every step supports these common fields in addition to the action-specific fields documented in the [Actions Reference](../actions/).

| Field | Type | Description |
|-------|------|-------------|
| `action` | string | **Required.** The action name (e.g., `pacman-add`, `file-create`). |
| `label` | string | Optional. Human-readable label shown in build output and `starforge inspect`. |
| `layer_source` | string | Optional. Git repo URL (`*.git#ref`) or archive URL to resolve before action dispatch. |
| `layer_script` | string | Optional. Inline shell script to run in the resolved `layer_source` directory on the host. Requires `layer_source`. |
| `layer_script_path` | string | Optional. Path to a shell script (relative to layer dir) to run in the resolved `layer_source` directory. Requires `layer_source`. Mutually exclusive with `layer_script`. |

### `layer_source`

Fetches external files for a single step. The resolved directory replaces the layer directory for that step, so `layer_path` references resolve against the source.

```yaml
- action: file-create
  layer_source: https://github.com/org/configs.git#v2.0
  layer_path: ./etc/myapp.conf
  path: /etc/myapp.conf
```

Sources are cached in `.starforge/cache/sources/{sha256(url)}/` with a `.resolved` marker to avoid re-fetching.

### `layer_script` / `layer_script_path`

Runs a host-side build script in the resolved source directory before the action executes. Useful for compiling or transforming files from the source.

```yaml
- action: file-create
  layer_source: https://github.com/org/app.git#v1.0
  layer_script: |
    make build
    strip ./output/myapp
  layer_path: ./output/myapp
  path: /usr/local/bin/myapp
```

`layer_script` (inline content) and `layer_script_path` (path to a script file relative to the layer directory) are mutually exclusive. Both require `layer_source`.

## Override Semantics

When the same action appears in multiple layers, StarForge uses these rules to combine them:

| Semantics | Actions | Behavior |
|-----------|---------|----------|
| **Replace** | `systemd-boot-install`, `systemd-target` (set-default), `system-hostname`, `system-locale` (locale), `system-timezone`, `system-keymap` | Last layer wins entirely. |
| **Replace-on-path** | `file-create` (single files) | Later layer replaces earlier file at the same path. Directory copies always accumulate. |
| **Accumulate + replace-on-name** | `partition-add` | Partitions accumulate; a later partition with the same name replaces the earlier definition in place. |
| **Remove** | `pacman-remove`, `partition-remove` | Removes matching items accumulated by earlier layers. |
| **Merge-on-name** | `system-user` | Later layer referencing the same user modifies the existing user. Supports `!add`/`!remove` on `groups`. |
| **Accumulate** | Everything else | Values from all layers are combined in order. |

This means a "desktop" layer can override the hostname set by a "base" layer, while packages from both layers are combined into a single list.

## `!include` -- File Inclusion

The `!include` tag lets you split large layers into multiple YAML files. It can appear anywhere in the layer YAML.

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

The `yaml_path` navigates into the loaded YAML structure using mapping keys and sequence indices (e.g., `steps.0` for the first item in a `steps` sequence).

### List splicing

When an `!include` in a sequence resolves to another sequence, items are spliced (flattened) into the parent list:

```yaml
# packages.yaml
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

URL includes are fetched and cached locally. They require a cache directory (provided automatically during builds).

### Nesting limits

Includes nest up to 10 levels deep. Each nested include resolves paths relative to the included file's directory, not the root file. Circular includes hit the depth limit and produce an error.

## Layering Strategy

A typical project uses layers to separate concerns:

```
layers/
  base/           # Partitions, packages, locale, timezone, keymap
  desktop/        # Desktop environment, display manager, user accounts
  app/            # Application-specific files, services, scripts
  dev-tools/      # Extra packages and config for development targets
```

Best practices:

- **Base layer first.** Define partitions, core packages, and system configuration in the base layer.
- **One concern per layer.** Keep layers focused -- a "networking" layer, a "desktop" layer, a "player" layer.
- **Use `!include` for large layers.** Split package lists, service definitions, and file blocks into separate YAML files.
- **Use variables for differences.** Pass target-specific values through `args` rather than duplicating layers. See [Variables](variables/).
- **Use `imports`/`exports` for reusable layers.** Document what a layer needs and what it provides so it can be shared across projects.
