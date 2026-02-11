---
title: "starforge list"
weight: 8
---


List targets defined in the project.

## Synopsis

```
starforge list
```

## Description

Display the project name, description, and all targets with their constituent layers. Targets are shown in sorted order.

This command does not require a prior build -- it reads directly from `starforge.yaml`.

## Examples

```bash
starforge list
```

### Sample Output

```
my-os
My custom operating system

  device
    - ./layers/base
    - ./layers/desktop
    - ./layers/player
```

## See Also

- [status](status/) -- Show build state alongside targets
- [inspect](inspect/) -- View the resolved configuration for a target
