---
title: "layer-run"
weight: 41
---


Run a script on the host during the Collect phase to compute build variables dynamically. Unlike [`run`](run/), which executes inside the target chroot during the build, `layer-run` executes on the host machine before any build phases begin.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `script` | string | Conditional | Inline script content. Mutually exclusive with `script_path`. |
| `script_path` | string | Conditional | Path to a script file relative to the layer directory. Mutually exclusive with `script`. |
| `env` | map | No | Environment variables to pass to the script. Values support `${{ var }}` substitution. |

Exactly one of `script` or `script_path` is required.

## Variable Capture

The script has access to two bash functions for interacting with the build variable scope:

- **`sf_set key value`** -- Sets a variable in the build scope. The variable is immediately available to subsequent steps via `${{ key }}`.
- **`sf_get key`** -- Retrieves a previously set variable from the build scope. Returns the value via stdout (use command substitution to capture it).

Variables are stored in the `__sf_vars` associative array, which is populated with all variables currently in scope when the script starts.

## Environment

Variables from the build scope are passed to the script as `STARFORGE_VAR_<NAME>` environment variables (names are uppercased). Step-level `env:` values are also available and support `${{ var }}` substitution. Target-level `env` from `starforge.yaml` is included as well.

## Examples

### Computing a build date and git hash

```yaml
- action: layer-run
  script: |
    sf_set build_date "$(date -u +%Y%m%d)"
    sf_set git_hash "$(cd /path/to/repo && git rev-parse --short HEAD)"
```

### Using with layer_source

When `layer_source` is set, the script's working directory is the resolved source directory:

```yaml
- action: layer-run
  layer_source: https://github.com/org/app.git#main
  script: |
    sf_set app_version "$(git describe --tags)"
```

### Reading a previously set variable

```yaml
- action: layer-run
  script: |
    prev=$(sf_get build_date)
    sf_set build_label "build-${prev}-final"
```

### Using a script file

```yaml
- action: layer-run
  script_path: ./scripts/compute-vars.sh
```

### With environment variables

```yaml
- action: layer-run
  env:
    REGISTRY: https://registry.example.com
    APP_NAME: ${{ app_name }}
  script: |
    version=$(curl -s "$REGISTRY/api/$APP_NAME/latest")
    sf_set app_version "$version"
```

## Semantics

**Accumulate.** Scripts from all layers run in order during the Collect phase. Each script can read variables set by previous scripts.

## Build Phase

Runs during Collect (before any Execute phases). Not assigned a build phase number. The `layer-run` action is handled directly by the builder during layer collection rather than dispatched through the action registry.

## Notes

- Scripts run on the host machine, not inside the target chroot. This is the key difference from [`run`](run/), which executes in the chroot during phase 8.
- The script's working directory is the layer directory, or the resolved `layer_source` directory if `layer_source` is set.
- Variables set by `sf_set` are immediately available to subsequent steps in the same layer and to later layers via `${{ var_name }}` substitution.
- The script runs via `bash` and inherits the host's environment.
- If the script exits with a non-zero status, the build fails immediately.
- Variable output is captured through a temporary file. Each `sf_set` call appends a `KEY=VALUE` line that is parsed after the script completes.
- See [Variables](../guide/variables/) for the full variable system documentation.
