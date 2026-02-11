---
title: "starforge status"
weight: 9
---


Show project info and build state.

## Synopsis

```
starforge status
```

## Description

Display project metadata and the build state of each target. Shows whether each target has been built and lists its layers.

## Output Fields

| Field | Description |
|-------|-------------|
| Name | Project name from `starforge.yaml` |
| Description | Project description (if set) |
| Directory | Absolute path to the project root |
| Build dir | Path to `.starforge/` build artifacts directory |

Each target shows:
- **Build state**: `[built]` (green) if the target's build directory exists, `[not built]` (dim) otherwise.
- **Layer count and list**: Number of layers and their paths.

## Examples

```bash
starforge status
```

## See Also

- [list](list/) -- List targets without build state
- [build](build/) -- Build a target
- [clean](clean/) -- Remove build artifacts
