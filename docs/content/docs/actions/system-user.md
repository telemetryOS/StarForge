---
title: "system-user"
weight: 24
---


Create or modify a user account in the target system.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | User name. |
| `groups` | list of strings | No | Supplementary groups. Supports `!add` and `!remove` tags for merge control. |
| `shell` | string | No | Login shell (e.g., `/bin/bash`, `/usr/bin/zsh`). |
| `password` | string | No | Password hash (use `mkpasswd` to generate). |
| `system` | bool | No | Create as a system user. Default: `false`. |
| `uid` | int | No | Explicit UID. If omitted, assigned automatically. |

## Examples

### Basic user

```yaml
- action: system-user
  name: player
  groups: [wheel, video, audio, render]
  shell: /bin/bash
```

### System user (no login shell)

```yaml
- action: system-user
  name: myapp
  system: true
```

### Modify a user in a later layer

```yaml
# Base layer creates the user
- action: system-user
  name: player
  groups: [wheel, video, audio]
  shell: /bin/bash

# Later layer adds groups without replacing the existing list
- action: system-user
  name: player
  groups: !add [docker, render]

# Another layer removes a group
- action: system-user
  name: player
  groups: !remove [audio]
# Result: [wheel, video, docker, render]
```

## Semantics

**Merge-on-name.** If multiple layers define a user with the same `name`, the later layer modifies the existing user rather than creating a duplicate.

- **`groups` (no tag)**: Replaces the entire group list.
- **`groups: !add [...]`**: Appends to the existing group list.
- **`groups: !remove [...]`**: Removes specified groups from the existing list.

Other fields (shell, password, uid, system) are replaced if specified in a later layer.

## Build Phase

Phase 3 (`users`). Users are created after groups. Home directories are created with appropriate ownership.

## Notes

- The `password` field expects a hashed password. Generate one with: `mkpasswd -m sha-512`.
- Users with `system: true` are created with `useradd -r` (no home directory, no login shell by default).
- A user's primary group is created automatically with the same name.
- Supplementary groups must exist (created by `system-group` or provided by packages like `wheel`, `video`, etc.).
- The `!add` / `!remove` tags on `groups` are part of StarForge's `Mergeable` type system. See the [YAML Reference](../yaml-reference/) for details.
