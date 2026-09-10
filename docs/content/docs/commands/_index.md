---
title: "CLI Commands"
linkTitle: "Commands"
weight: 5
---

StarForge provides commands for creating, building, testing, and deploying OS images.

## Command Summary

| Command | Description |
|---------|-------------|
| [init](init/) | Create a new StarForge project |
| [build](build/) | Build overlay layers for a target |
| [run](run/) | Boot a target in QEMU |
| [write](write/) | Write a target to a block device |
| [export](export/) | Export build artifacts as disk or partition images |
| [inspect](inspect/) | Inspect the resolved build context |
| [list](list/) | List targets defined in the project |
| [status](status/) | Show project and build status |
| [clean](clean/) | Remove build artifacts |

## Typical Workflow

```bash
# 1. Create a new project
starforge init my-os

# 2. Edit layers to define packages, files, services, etc.

# 3. Verify layer resolution before building
starforge inspect distribution

# 4. Build the target
starforge build distribution

# 5. Test in a QEMU virtual machine
starforge run distribution

# 6. Deploy to a physical device
starforge write distribution /dev/sdX
```

## Global Behavior

- **Project discovery.** StarForge looks for `starforge.yaml` by walking up from the current directory. All commands that operate on a project use this mechanism, so you can run them from any subdirectory.
- **Executor selection.** StarForge always defaults to the managed QEMU builder VM, regardless of host privileges. Use `--executor=native` to opt into host execution. Compatibility mode `--executor=auto` may choose native execution only when StarForge is already root on the target architecture.
- **Root access.** VM builds, inspection, export, run, status, list, and clean are unprivileged. `write` elevates only its final block-device operation.
- **Target isolation.** Architecture, machine, and distribution are target properties in `starforge.yaml`; they are independent of the executor and host architecture.
