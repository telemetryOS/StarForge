---
title: "file-edit"
weight: 11
---


Modify existing files in the target filesystem. Supports appending, prepending, and pattern-based insertion and truncation via custom YAML tags.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | Yes | Absolute path to the file to edit in the target filesystem. |
| `content` | tagged string | Yes | Content to insert, with a YAML tag specifying the edit mode. |
| `layer_path` | string | No | Path to a file (relative to the layer directory) whose content is used as the edit value. Alternative to inline `content`. |

The `content` field supports these YAML tags:

| Tag | Type | Description |
|-----|------|-------------|
| `!append` | scalar | Append content to the end of the file. |
| `!prepend` | scalar | Prepend content to the beginning of the file. |
| `!before` | mapping | Insert content before lines matching a regex pattern. |
| `!after` | mapping | Insert content after lines matching a regex pattern. |
| `!truncate_before` | mapping | Remove all content before the matching line (matched line kept). |
| `!truncate_after` | mapping | Remove all content after the matching line (matched line kept). |

### Pattern mapping fields

For `!before`, `!after`, `!truncate_before`, and `!truncate_after`:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `pattern` | string | Yes | Go regular expression to match against each line. |
| `match` | int | No | Which occurrence to match. For `!before`/`!after`: `0` means all (default). For `!truncate_before`/`!truncate_after`: default `1`. |
| `value` | string | For insert tags | Content to insert (required for `!before`/`!after`, not used by truncate tags). |

## Examples

### Append to a file

```yaml
- action: file-edit
  path: /etc/hosts
  content: !append |
    192.168.1.100  myserver
    192.168.1.101  myserver2
```

### Prepend to a file

```yaml
- action: file-edit
  path: /etc/hosts
  content: !prepend |
    # Custom hosts file header
```

### Insert before a pattern

```yaml
- action: file-edit
  path: /etc/pacman.conf
  content: !before
    pattern: "^\\[extra\\]"
    value: |
      [custom]
      Server = https://repo.example.com/$arch
```

### Insert after a pattern

```yaml
- action: file-edit
  path: /etc/config.ini
  content: !after
    pattern: "^\\[network\\]"
    match: 1
    value: |
      dns = 8.8.8.8
```

### Truncate before a pattern

```yaml
# Keep only content from [main] section onward
- action: file-edit
  path: /etc/config
  content: !truncate_before
    pattern: "^\\[main\\]"
```

### Truncate after a pattern

```yaml
# Remove everything after the END marker
- action: file-edit
  path: /etc/config
  content: !truncate_after
    pattern: "^# END"
```

## Semantics

**Accumulate.** Multiple `file-edit` steps for the same file are applied in order during phase 4. Each edit operates on the file content as modified by previous edits.

## Build Phase

Phase 4 (`files`). File edits run after file creates, so you can create a file and then edit it in the same layer.

## Notes

- The `pattern` field uses Go regular expressions (the `regexp` package). Patterns match against individual lines.
- Regex patterns containing backslashes must be quoted. YAML double-quoted strings require double backslashes (`"^\\[section\\]"`). YAML single-quoted strings do not interpret escapes (`'^\\[section\\]'` matches literally). See the [YAML Reference](../yaml-reference/#regex-patterns--always-quote) for quoting guidance.
- The `match` field selects which occurrence of the pattern to act on. For `!before`/`!after`, `0` (or omitted) means all occurrences. For `!truncate_before`/`!truncate_after`, `1` is the default.
- The file must exist in the target filesystem before editing. Use [`file-create`](file-create/) or ensure a package provides the file.
