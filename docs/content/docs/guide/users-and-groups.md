---
title: "Users & Groups"
weight: 6
---

StarForge creates user accounts and groups during **phase 3** of the build. Groups are created first, then users. Both actions support cross-layer modification -- later layers can refine accounts defined in earlier layers.

## Creating Users

The `system-user` action defines a user account. The only required field is `name`:

```yaml
- action: system-user
  name: admin
  uid: 1000
  groups: [wheel, video, audio]
  shell: /bin/bash
  password: changeme
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | -- | Username. |
| `groups` | list | No | -- | Group memberships. Supports merge tags. |
| `shell` | string | No | `/bin/bash` | Login shell. |
| `password` | string | No | -- | Plaintext password, hashed during build. |
| `no_password` | bool | No | `false` | Allow login without a password. |
| `uid` | int | No | auto | Explicit UID. `0` means auto-assign. |
| `system` | bool | No | `false` | Create as a system user (no home directory). |

See the [`system-user` reference](../../actions/system-user/) for full details.

## Merge-on-Name Semantics

When a later layer includes a `system-user` step with the same `name` as one defined in an earlier layer, the two definitions are **merged** rather than replaced. Only the fields specified in the later layer are updated; unspecified fields retain their earlier values.

```yaml
# layers/base/layer.yaml
- action: system-user
  name: staff
  groups: [wheel]
  shell: /bin/bash
  password: fiber-buffer-deploy-vault

# layers/development/layer.yaml -- modifies the existing staff user
- action: system-user
  name: staff
  shell: /usr/bin/fish
  no_password: true
```

In this example, the development layer changes the staff user's shell to `fish` and enables passwordless login, but the `wheel` group membership and the original UID (auto-assigned) are preserved.

## Group Merge Tags

The `groups` field supports YAML merge tags for fine-grained control across layers:

| Syntax | Effect |
|--------|--------|
| `groups: [wheel, video]` | Replace the entire group list. |
| `groups: !add [docker, lp]` | Append to the existing group list. |
| `groups: !remove [audio]` | Remove specific groups from the list. |

### Multi-Layer Example

Consider a three-layer project where each layer progressively refines a user's group memberships:

```yaml
# layers/base/layer.yaml -- create the user with initial groups
- action: system-user
  name: admin
  uid: 1000
  groups: [wheel, video, audio]
  shell: /bin/bash

# layers/desktop/layer.yaml -- add groups for desktop capabilities
- action: system-user
  name: admin
  groups: !add [docker, render, seat]

# layers/kiosk/layer.yaml -- remove audio access for kiosk mode
- action: system-user
  name: admin
  groups: !remove [audio]
```

After all three layers are collected, the `admin` user has groups: `wheel`, `video`, `docker`, `render`, `seat`. The `audio` group was added in the base layer and removed by the kiosk layer.

Without a merge tag, specifying `groups` replaces the entire list:

```yaml
# This replaces ALL groups -- admin would only be in 'wheel'
- action: system-user
  name: admin
  groups: [wheel]
```

## System Users

Set `system: true` to create a system user with no home directory. System users are typically service accounts:

```yaml
- action: system-user
  name: data
  system: true
  shell: /usr/bin/nologin
```

## Creating Groups

The `system-group` action creates groups explicitly. This is needed for groups that are not created as a side effect of package installation.

```yaml
- action: system-group
  name: data
  gid: 500
  system: true
```

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | -- | Group name. |
| `gid` | int | No | auto | Explicit GID. `0` means auto-assign. |
| `system` | bool | No | `false` | Create as a system group. |

See the [`system-group` reference](../../actions/system-group/) for full details.

Groups declared with `system-group` are created before any users, ensuring that a user's `groups` list can reference them without ordering concerns.

## Creation Order

Phase 3 executes in a fixed order:

1. **Groups** -- all `system-group` steps are applied first.
2. **Users** -- all `system-user` steps are applied second.

This means you can define a group in one step and reference it in a user's `groups` list in the same layer or a later layer:

```yaml
- action: system-group
  name: data
  system: true

- action: system-user
  name: player
  groups: [data, video, audio]
```

## Practical Example

This example creates three users across base and development layers:

```yaml
# layers/base/layer.yaml

# System user for data partition ownership
- action: system-user
  name: data
  system: true
  shell: /usr/bin/nologin

# Interactive kiosk user with broad hardware access
- action: system-user
  name: player
  groups: [wheel, video, render, seat, audio, input, data, docker, lp, network]
  shell: /bin/bash

# Admin user with password-protected sudo
- action: system-user
  name: staff
  groups: [wheel]
  shell: /bin/bash
  password: fiber-buffer-deploy-vault
```

The development layer then modifies the staff user for a better developer experience:

```yaml
# layers/development/layer.yaml

- action: system-user
  name: staff
  shell: /usr/bin/fish
  no_password: true
  groups: !add [player]
```

This changes the staff shell to `fish`, enables passwordless login, and adds the `player` group -- all without affecting the base-layer settings for the `data` or `player` users, and without replacing staff's existing `wheel` group.
