---
title: "YAML Reference"
weight: 7
---


StarForge layers are written in YAML with several custom tags for file inclusion, merge control, file editing, and systemd overrides. This page is a centralized reference for all YAML features, quoting rules, and common patterns.

## Custom Tags

StarForge defines 10 custom YAML tags. Each tag is scoped to specific fields — using a tag on the wrong field will produce a parse error.

| Tag | Scope | Purpose |
|-----|-------|---------|
| [`!include`](#include) | Anywhere in layer YAML | Inline external files |
| [`!add`](#add--remove) | `system-user` `groups` | Append to existing list |
| [`!remove`](#add--remove) | `system-user` `groups` | Remove from existing list |
| [`!append`](#append--prepend) | `file-edit` `content` | Insert at end of file |
| [`!prepend`](#append--prepend) | `file-edit` `content` | Insert at beginning of file |
| [`!before`](#before--after) | `file-edit` `content` | Insert before regex match |
| [`!after`](#before--after) | `file-edit` `content` | Insert after regex match |
| [`!truncate_before`](#truncate_before--truncate_after) | `file-edit` `content` | Remove content before match |
| [`!truncate_after`](#truncate_before--truncate_after) | `file-edit` `content` | Remove content after match |
| [`!replace`](#replace) | Systemd unit section values | Clear directive before setting |

### `!include`

Inlines content from external YAML files. Can appear anywhere in a layer's YAML.

**Scalar form** — include an entire file:

```yaml
steps:
  - !include ./packages.yaml
  - !include ./services.yaml
  - action: system-hostname
    hostname: my-device
```

**Mapping form** — include a specific portion of a file:

```yaml
steps:
  - !include
    layer_path: ./shared.yaml
    yaml_path: common.steps
```

The `yaml_path` is a dot-separated path that navigates into the loaded YAML structure. It supports mapping keys and sequence indices:

```yaml
# shared.yaml
common:
  steps:
    - action: pacman-add
      packages: [base, linux]
  users:
    - action: system-user
      name: admin

# layer.yaml — includes only the steps list from shared.yaml
steps:
  - !include
    layer_path: ./shared.yaml
    yaml_path: common.steps
```

**List splicing** — when an `!include` in a sequence resolves to another sequence, items are spliced (flattened) into the parent list:

```yaml
# packages.yaml (a sequence)
- action: pacman-add
  packages: [base, linux]
- action: pacman-add
  packages: [sudo, openssh]

# layer.yaml — both steps end up in the steps list, not nested
steps:
  - !include ./packages.yaml
  - action: system-hostname
    hostname: my-device
```

**URL includes**:

```yaml
steps:
  - !include https://example.com/shared-steps.yaml
```

**Limits**: Includes nest up to 10 levels deep. Each nested include resolves paths relative to the included file's directory, not the root file. Circular includes hit the depth limit and error.

See also: [Actions Reference — `!include`](actions/#include--file-inclusion)

### `!add` / `!remove`

Control how list fields merge across layers. Currently supported on the `groups` field of [`system-user`](actions/system-user/).

```yaml
# Base layer — set initial groups
- action: system-user
  name: admin
  groups: [wheel, video, audio]

# Later layer — add groups without replacing
- action: system-user
  name: admin
  groups: !add [docker, render]

# Another layer — remove specific groups
- action: system-user
  name: admin
  groups: !remove [audio]

# Result: [wheel, video, docker, render]
```

Without a tag, the groups list is **replaced** entirely. This is the default behavior — a plain list overwrites whatever earlier layers set.

### `!append` / `!prepend`

Insert content at the end or beginning of an existing file. Used on the `content` field of [`file-edit`](actions/file-edit/).

```yaml
# Append lines to the end
- action: file-edit
  path: /etc/hosts
  content: !append |
    192.168.1.100  myserver

# Prepend lines to the beginning
- action: file-edit
  path: /etc/hosts
  content: !prepend |
    # Custom hosts
```

These tags take a scalar string value (the content to insert).

### `!before` / `!after`

Insert content before or after lines matching a regex pattern. Used on the `content` field of [`file-edit`](actions/file-edit/).

```yaml
# Insert before the first line matching the pattern
- action: file-edit
  path: /etc/pacman.conf
  content: !before
    pattern: "^\\[extra\\]"
    value: |
      [custom]
      Server = https://repo.example.com/$arch

# Insert after the second match
- action: file-edit
  path: /etc/config
  content: !after
    pattern: "^\\[section\\]"
    match: 2
    value: |
      key = value
```

These tags take a mapping with:
- `pattern` (required) — Go regex matched against each line
- `value` (required) — Content to insert
- `match` (optional) — Limit which matches to act on. Default `0` means all matches.

### `!truncate_before` / `!truncate_after`

Remove content before or after a matching line. The matched line itself is always kept. Used on the `content` field of [`file-edit`](actions/file-edit/).

```yaml
# Keep only content from the matching line onward
- action: file-edit
  path: /etc/config
  content: !truncate_before
    pattern: "^\\[main\\]"

# Keep content up to and including the second match
- action: file-edit
  path: /etc/config
  content: !truncate_after
    pattern: "^# END"
    match: 2
```

These tags take a mapping with:
- `pattern` (required) — Go regex matched against each line
- `match` (optional) — Which occurrence to match. Default `1` (first).

### `!replace`

In systemd unit section maps (`unit`, `service`, `mount`, `timer`, `socket`, `slice`, `install`), `!replace` generates a clear-then-set pattern. This is essential for drop-in overrides where the parent unit's directive must be reset before applying a new value.

```yaml
- action: systemd-service
  name: override.conf
  extends:
    service: getty@tty1
  service:
    exec_start: !replace "-/sbin/agetty --autologin player"
```

Renders as:

```ini
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin player
```

Without `!replace`, the value would be *added* to the parent's `ExecStart` list rather than replacing it. Use `!replace` whenever a drop-in overrides a list-type directive like `ExecStart`, `ExecStartPre`, or `Environment`.

See also: [systemd-service — `!replace` tag](actions/systemd-service/#replace-tag)

## YAML Quoting Rules

YAML has implicit type inference that can cause unexpected behavior with certain values. Here are the rules relevant to StarForge:

### Octal numbers — always quote file modes

YAML interprets unquoted numbers starting with `0` as octal integers. File mode strings **must be quoted**:

```yaml
# CORRECT — mode is the string "0755"
mode: "0755"

# WRONG — YAML interprets 0755 as the integer 493
mode: 0755

# WRONG — YAML interprets 0644 as the integer 420
mode: 0644
```

This applies to:
- `file-create` `mode`
- `file-permissions` `mode`
- `file-mkdir` `mode`

### Regex patterns — escape backslashes

YAML double-quoted strings interpret `\\` as a literal backslash. When writing regex patterns for `file-edit` or `!before`/`!after`/`!truncate_*`, double the backslashes:

```yaml
# Match a line starting with [section]
pattern: "^\\[section\\]"

# Match a comment line
pattern: "^#\\s+"

# Match an IP address pattern
pattern: "^\\d+\\.\\d+\\.\\d+\\.\\d+"
```

Single-quoted strings pass content literally — no escaping needed:

```yaml
# Also correct — single quotes don't interpret escapes
pattern: '^\[section\]'
```

### Booleans — quote when you mean strings

YAML treats `yes`, `no`, `true`, `false`, `on`, `off` (case-insensitive) as booleans. Quote them when you need a literal string:

```yaml
# Boolean true (correct for enable/disable/mask fields)
enable: true

# String "yes" (if a config file needs the literal word)
content: |
  Setting=yes
```

### Special characters — use block scalars for multiline

For inline file content or scripts, use YAML block scalars:

```yaml
# Literal block (preserves newlines and indentation)
content: |
  [Unit]
  Description=My Service

  [Service]
  ExecStart=/usr/bin/myapp

# Strip trailing newline
content: |-
  single line without trailing newline

# Folded block (joins lines with spaces, keeps blank line breaks)
content: >
  This is a long description that
  will be joined into one line.
```

The `|` (literal) style is recommended for file content and scripts — it preserves formatting exactly as written. The `>` (folded) style is useful for long strings that should be joined.

### Colons in values — quote or use block scalars

A colon followed by a space (`: `) starts a YAML mapping. Values containing colons need quoting:

```yaml
# WRONG — YAML sees this as a nested mapping
options: rw root=UUID=abc-123

# CORRECT — quoted string
options: "rw root=UUID=abc-123"

# ALSO CORRECT — block scalar
options: |
  rw root=UUID=abc-123
```

In practice, most StarForge fields handle this correctly because simple values without spaces after colons work fine (`UUID=abc` is not ambiguous). But kernel options and complex strings should be quoted.

## Systemd INI Field Names

All systemd unit actions (`systemd-service`, `systemd-mount`, `systemd-timer`, `systemd-socket`, `systemd-slice`, `systemd-target`) use `snake_case` field names in YAML, which are automatically converted to systemd's `CamelCase` in the generated INI files.

### Conversion rules

1. Split on underscores
2. Capitalize the first letter of each word
3. Uppercase known acronyms entirely

```yaml
# YAML                    # Generated INI
exec_start            →   ExecStart
wanted_by             →   WantedBy
restart_sec           →   RestartSec
cpu_weight            →   CPUWeight
oom_score_adjust      →   OOMScoreAdjust
dns_stub_listener     →   DNSStubListener
io_device_weight      →   IODeviceWeight
```

### Known acronyms

These abbreviations are uppercased entirely when they appear as a word in a field name:

`CPU`, `PID`, `OOM`, `IO`, `TCP`, `UDP`, `IP`, `UID`, `GID`, `DNS`, `NTP`, `TTY`

### Value types

| YAML type | INI output | Example |
|-----------|-----------|---------|
| String | As-is | `exec_start: /usr/bin/app` → `ExecStart=/usr/bin/app` |
| Number | As-is | `restart_sec: 5` → `RestartSec=5` |
| Boolean | `yes`/`no` | `persistent: true` → `Persistent=yes` |
| List | Repeated key | `exec_start_pre: [/bin/a, /bin/b]` → `ExecStartPre=/bin/a\nExecStartPre=/bin/b` |

Sections are ordered canonically: `[Unit]`, then the type-specific section (`[Service]`, `[Mount]`, `[Timer]`, `[Socket]`, `[Slice]`), then `[Install]`. Keys within each section are sorted alphabetically.

## Common Patterns

### Splitting a large layer with `!include`

```
layers/base/
├── layer.yaml
├── packages.yaml
├── services.yaml
├── users.yaml
└── files/
    └── etc/...
```

```yaml
# layer.yaml
steps:
  - action: partition-add
    partitions:
      - name: boot
        filesystem: vfat
        size: 512M
        mount_point: /boot
        type: efi
      - name: root
        filesystem: ext4
        size: 8G
        mount_point: /
        type: linux

  - !include ./packages.yaml
  - !include ./users.yaml
  - !include ./services.yaml

  - action: file-create
    layer_path: ./files/etc
    path: /etc
```

### Sharing steps across targets

```yaml
# shared/common-packages.yaml
- action: pacman-add
  packages: [base, linux, linux-firmware, sudo, openssh]

# layers/minimal/layer.yaml
steps:
  - !include ../../shared/common-packages.yaml
  - action: system-hostname
    hostname: minimal-device

# layers/desktop/layer.yaml
steps:
  - !include ../../shared/common-packages.yaml
  - action: pacman-add
    packages: [sway, pipewire, firefox]
```

### Drop-in override with `!replace`

```yaml
# Override the getty autologin service
- action: systemd-service
  name: override.conf
  extends:
    service: getty@tty1
  service:
    exec_start: !replace "-/sbin/agetty --autologin player --noclear - $TERM"
```

### Building user accounts across layers

```yaml
# Base layer: create user with core groups
- action: system-user
  name: player
  groups: [video, audio, input]
  shell: /bin/bash

# Desktop layer: add compositor groups
- action: system-user
  name: player
  groups: !add [seat, render]

# App layer: add application-specific groups
- action: system-user
  name: player
  groups: !add [docker]
```
