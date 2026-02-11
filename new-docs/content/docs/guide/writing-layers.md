---
title: "Writing Layers"
weight: 2
---

Layers are the primary unit of composition in StarForge. Each layer is a directory containing a `layer.yaml` file that defines a list of steps. Layers are processed in order during the Collect phase, and their combined results drive the Build phase.

## Layer Anatomy

A `layer.yaml` file has four top-level fields:

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
| `steps` | list | Ordered list of action steps. This is the core of every layer. |
| `vars` | map | Default variable values for this layer. Overridden by target `args` or values exported from earlier layers. |
| `imports` | list | Required variables -- the build fails with an error if any are missing when this layer is processed. |
| `exports` | list | Variables to propagate to subsequent layers. If omitted, all variables propagate. If specified, only the listed variables are forwarded. |

For full details on the variable system, see [Variables](../variables/).

## Step Structure

Every step requires an `action` field that names the action to execute. Beyond that, each step has action-specific fields documented in the [Actions Reference](../../actions/). All steps also support these common fields:

| Field | Type | Description |
|-------|------|-------------|
| `action` | string | **Required.** The action name (e.g., `pacman-add`, `file-create`, `systemd-service`). |
| `label` | string | Optional. Human-readable label shown in build output and `starforge inspect --layers`. |
| `layer_source` | string | Optional. Git repo URL or archive URL to fetch before running this step. |
| `layer_script` | string | Optional. Inline shell script to run in the resolved `layer_source` directory on the host. Requires `layer_source`. |
| `layer_script_path` | string | Optional. Path to a shell script (relative to the layer directory) to run in the resolved `layer_source` directory. Requires `layer_source`. Mutually exclusive with `layer_script`. |

### Labels

Labels appear in build output and in `starforge inspect --layers` but have no effect on the build. Use them to document the purpose of each step:

```yaml
steps:
  - action: pacman-add
    label: Core system packages
    packages:
      - base
      - linux
      - linux-firmware
```

### External Sources with `layer_source`

Any step can pull files from an external git repo or archive using `layer_source`. The resolved directory replaces the layer directory for that single step, so `layer_path` references resolve against the source:

```yaml
# Clone a git repo and copy its files into the target
- action: file-create
  layer_source: https://github.com/org/configs.git#v2.0
  layer_path: ./etc/myapp
  path: /etc/myapp

# Clone a repo, run a build script, and use the output
- action: file-create
  layer_source: https://github.com/org/app.git#main
  layer_script: |
    make build
  layer_path: ./output/app.bin
  path: /usr/local/bin/app
```

Sources are cached in `.starforge/cache/sources/`. The `layer_script` (inline) and `layer_script_path` (file reference) fields are mutually exclusive and both require `layer_source`.

## Splitting Layers with `!include`

The `!include` tag lets you split large layers into multiple YAML files. This is especially useful for layers with many steps across different concerns (packages, services, files).

### Scalar Form

Include an entire file by path:

```yaml
steps:
  - !include ./packages.yaml
  - !include ./services.yaml
  - action: system-hostname
    hostname: my-device
```

Paths are resolved relative to the including file's directory.

### Mapping Form

Include a specific portion of a file using `yaml_path`:

```yaml
# shared.yaml
common:
  packages:
    - action: pacman-add
      packages: [base, linux]
  services:
    - action: systemd-service
      name: sshd
      enable: true

# layer.yaml
steps:
  - !include
    layer_path: ./shared.yaml
    yaml_path: common.packages
  - !include
    layer_path: ./shared.yaml
    yaml_path: common.services
```

The `yaml_path` is dot-separated and supports mapping keys and sequence indices (e.g., `common.packages.0` for the first item).

### List Splicing

When an `!include` inside a sequence resolves to another sequence, items are spliced (flattened) into the parent list rather than nested:

```yaml
# packages.yaml
- action: pacman-add
  packages: [base, linux]
- action: pacman-add
  packages: [sudo, openssh]

# layer.yaml -- both steps appear directly in the steps list
steps:
  - !include ./packages.yaml
  - action: system-hostname
    hostname: my-device
```

### URL Includes

Includes can also reference URLs:

```yaml
steps:
  - !include https://example.com/shared-steps.yaml
```

URL includes are fetched and cached locally.

### Nesting Limits

Includes nest up to 10 levels deep. Each nested include resolves paths relative to the included file's directory, not the root file. Circular includes hit the depth limit and produce an error.

For the complete `!include` specification, see the [YAML Reference](../../yaml-reference/).

## Override Semantics

When the same action type appears across multiple layers, StarForge uses specific rules to combine their effects. Understanding these rules is key to building well-structured multi-layer projects.

| Semantics | Actions | Behavior |
|-----------|---------|----------|
| **Replace** | `systemd-boot-install`, `systemd-target` (set-default), `system-hostname`, `system-locale` (locale), `system-timezone`, `system-keymap` | Last layer wins entirely. |
| **Replace-on-path** | `file-create` (single files) | Later layer replaces an earlier file at the same path. Directory copies always accumulate. |
| **Accumulate + replace-on-name** | `partition-add` | Partitions accumulate; a later partition with the same name replaces the earlier definition in place. |
| **Remove** | `pacman-remove`, `partition-remove` | Removes matching items accumulated by earlier layers. |
| **Merge-on-name** | `system-user` | Later layer referencing the same user modifies the existing definition. Supports `!add`/`!remove` on the `groups` field. |
| **Accumulate** | Everything else | Values from all layers are combined in order. |

For example, a "desktop" layer can override the hostname set by a "base" layer (replace semantics), while packages from both layers are combined into a single deduplicated list (accumulate semantics).

## Layer Processing Order

Layers are processed in the order listed in the target's `layers` field. Within each layer, steps are processed in order from top to bottom. This means:

1. Earlier layers establish the baseline configuration.
2. Later layers can add to, override, or remove items from that baseline.
3. Within a single layer, step order matters for actions with the same semantics (e.g., a `pacman-add` followed by `pacman-remove` in the same layer).

## Best Practices

**One concern per layer.** Keep layers focused on a single responsibility -- a "base" layer for core system setup, a "desktop" layer for GUI packages, an "app" layer for application-specific configuration. This makes layers reusable across targets.

**Use labels.** Add `label` fields to steps for readable build output and `starforge inspect --layers` results. Labels are especially valuable in large layers.

**Prefer `layer_path` over inline content.** Keep configuration files as actual files in your layer directory rather than inlining them in `layer.yaml`. Files are easier to maintain, diff, and review. Use inline `content` only for short, single-line values.

**Quote octal modes.** File mode strings must be quoted (`"0755"`, not `0755`) to prevent YAML from interpreting them as octal integers. An unquoted `0755` becomes the integer `493`, which is not a valid mode string.

**Use `!include` for large layers.** Split steps across files by concern (packages, services, files) to keep individual YAML files manageable.

**Use `imports`/`exports` for reusable layers.** Document what a layer requires from earlier layers (`imports`) and what it provides to later layers (`exports`) so it can be shared across projects.

**Declare partitions early.** Define the full partition layout in the base layer. Later layers can adjust individual partitions with `partition-change` without redeclaring the entire layout.

## See Also

- [Project Structure](../project-structure/) -- The `starforge.yaml` file and target definitions.
- [Partitions](../partitions/) -- Defining disk layout across layers.
- [Packages](../packages/) -- Installing and removing packages across layers.
- [YAML Reference](../../yaml-reference/) -- Full specification for custom YAML tags and field conventions.
- [Actions Reference](../../actions/) -- Complete documentation for all 34 actions.
