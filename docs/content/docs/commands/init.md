---
title: "starforge init"
weight: 2
---


Create a new StarForge project.

## Synopsis

```
starforge init [name]
```

## Description

Interactively scaffold a new StarForge project with a `starforge.yaml` and an initial base layer. If `name` is provided as an argument, it is used as the project name; otherwise you will be prompted for it.

The command prompts for:
- **Project name** (required, unless provided as argument)
- **Description** (optional)
- **First target name** (defaults to `distribution`)

## Arguments

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | No | Project name. Prompted interactively if not provided. |

## Generated Structure

```
<name>/
├── starforge.yaml
├── .gitignore
└── layers/
    └── base/
        ├── layer.yaml
        └── files/
            └── etc/
```

The `files/etc/` directory is scaffolded as a convenience for the `file-create` action, but layers have no required directory structure -- you can organize files however you like and reference them with any relative path from the `file-create` action's `layer_path` field.

The generated `layer.yaml` includes a starter configuration with:
- Two partitions: `boot` (512M, vfat, EFI) and `root` (8G, ext4)
- Base packages: `base`, `linux`, `linux-firmware`
- Hostname set to the project name
- Locale set to `en_US.UTF-8`
- Timezone set to `UTC`

The `.gitignore` excludes the `.starforge/` build directory.

## Examples

```bash
# Interactive -- prompts for name, description, and target
starforge init

# Provide name as argument -- still prompts for description and target
starforge init my-os
```

## Next Steps

After creating a project:

```bash
cd my-os
starforge list                # View defined targets
starforge build <target>      # Build the target (default name: distribution)
```
