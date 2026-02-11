# Variables

StarForge has a variable system that lets you parameterize layers so the same layer can be reused across targets with different values. Variables flow through the build in a well-defined scope chain and are substituted into YAML values before each step is decoded.

## Substitution Syntax

Use `${{ var_name }}` in any scalar string value in `layer.yaml`:

```yaml
- action: system-hostname
  hostname: ${{ hostname }}

- action: file-create
  path: /etc/${{ app_name }}/config.toml
  content: |
    version = "${{ version }}"
    channel = "${{ channel }}"
```

The `${{ }}` syntax is chosen to avoid conflicts with shell variable syntax (`$VAR`, `${VAR}`) that frequently appears in scripts and configuration files.

Variable names must match `[a-zA-Z_][a-zA-Z0-9_]*`. Whitespace inside the braces is ignored (`${{ version }}` and `${{version}}` are equivalent).

If a referenced variable is not defined, the build fails immediately with an error.

## Variable Scope

### Target `args`

The `args` field in `starforge.yaml` seeds the initial variable scope for a target. These values are available to all layers in the target.

```yaml
# starforge.yaml
targets:
  device:
    args:
      version: "2.1.0"
      channel: stable
      hostname: edge-device
    layers:
      - ./layers/base
      - ./layers/app
```

### Layer `vars`

The `vars` field in `layer.yaml` provides default values. Layer vars do not overwrite existing values -- they only fill in variables that are not already defined.

```yaml
# layers/app/layer.yaml
vars:
  channel: dev
  log_level: info

steps:
  - action: file-create
    path: /etc/app.conf
    content: |
      channel = ${{ channel }}
      log_level = ${{ log_level }}
```

If the target sets `channel: stable` in `args`, the layer's default `channel: dev` is ignored. But `log_level: info` takes effect because no earlier source defines it.

### Layer `imports`

The `imports` field declares variables that the layer requires. If any imported variable is missing from the current scope when the layer starts, the build fails with an error.

```yaml
# layers/app/layer.yaml
imports:
  - version
  - hostname

vars:
  log_level: info

steps:
  - action: system-hostname
    hostname: ${{ hostname }}
```

Use `imports` to document a layer's requirements and catch missing variables early.

### Layer `exports`

The `exports` field controls which variables propagate to subsequent layers. If `exports` is specified, only the listed variables are passed forward. If `exports` is omitted, all variables propagate.

```yaml
# layers/compute-hash/layer.yaml
imports:
  - repo_url

exports:
  - commit_hash

steps:
  - action: layer-run
    script: |
      hash=$(git ls-remote "$repo_url" HEAD | cut -f1)
      sf_set commit_hash "$hash"
```

In this example, `commit_hash` is exported to subsequent layers, but any internal variables the layer may have defined (like `repo_url` from imports or `vars`) are filtered out unless explicitly listed in `exports`.

## Scoping Flow

The complete flow for a target with three layers:

```
Target args: { version: "2.1.0", channel: "stable" }
                        |
                        v
        +------ Layer 1 (base) ------+
        | vars: { locale: "en_US" }  |    locale added as default
        | imports: []                |
        | exports: (none)           |    all vars propagate
        +----------------------------+
                        |
          { version, channel, locale }
                        |
                        v
        +------ Layer 2 (app) -------+
        | vars: { log_level: "info" }|    log_level added as default
        | imports: [version]         |    version verified present
        | exports: [version, app_id] |    only version and app_id propagate
        +----------------------------+
                        |
            { version, app_id }
                        |
                        v
        +------ Layer 3 (deploy) ----+
        | vars: {}                   |
        | imports: [version, app_id] |    both verified present
        | exports: (none)            |    all vars propagate
        +----------------------------+
                        |
          { version, app_id }
```

Note that `channel`, `locale`, and `log_level` are filtered out by Layer 2's `exports`. Layer 3 only sees `version` and `app_id`.

## `layer-run` Action

The `layer-run` action executes a script on the host during the Collect phase. It can capture output as variables using the `sf_set` function.

```yaml
- action: layer-run
  script: |
    # Compute a value on the host
    hash=$(git -C /path/to/repo rev-parse --short HEAD)
    sf_set git_hash "$hash"

    # Read existing variables
    version=$(sf_get version)
    sf_set full_version "${version}-${hash}"
```

### `sf_set` and `sf_get` in `layer-run`

| Function | Purpose |
|----------|---------|
| `sf_set key value` | Set a variable in the build scope. The value is available to all subsequent steps and layers (subject to `exports`). |
| `sf_get key` | Read a variable from the current scope. Returns the value via `printf`. |

Variables set by `sf_set` are written to a temporary file and read back by the build engine after the script completes. They update the layer-scoped variable map, so they are subject to the same `exports` filtering as `vars`.

The `layer-run` action also receives all current variables as `STARFORGE_VAR_<NAME>` environment variables (uppercased), plus any `env` values from the target and step.

## Environment Variables

Environment variables are separate from build variables. They are passed to scripts (`run` and `layer-run`) as actual environment variables, not substituted into YAML.

### Target-level `env`

Defined in `starforge.yaml` on the target. Supports `${{ var }}` substitution (resolved against `args`).

```yaml
targets:
  device:
    args:
      version: "2.1.0"
    env:
      APP_VERSION: ${{ version }}
      APP_ENV: production
    layers:
      - ./layers/base
```

### Step-level `env`

Defined on `run` and `layer-run` steps. Step-level values override target-level values. Also supports `${{ var }}` substitution.

```yaml
- action: run
  env:
    INSTALL_DIR: /opt/app
    APP_VERSION: ${{ version }}
  script: |
    echo "Installing to $INSTALL_DIR version $APP_VERSION"
```

## Using Variables in `run` Scripts

Scripts executed by the `run` action run inside the target chroot during the [Execute](build-pipeline.md) phase (phase 8). They have access to variables through the `sf_get` function:

```yaml
- action: run
  script: |
    version=$(sf_get version)
    hostname=$(sf_get hostname)
    echo "Finalizing $hostname at version $version"
```

The build engine injects a bash prelude into every script that defines `sf_get`, `sf_set`, and a `__sf_vars` associative array. In chroot scripts, `sf_set` is a no-op -- variable output only works in `layer-run` scripts that execute on the host during Collect.

## Practical Examples

### Passing a version through layers

```yaml
# starforge.yaml
targets:
  release:
    args:
      version: "3.0.0"
    layers:
      - ./layers/base
      - ./layers/app

# layers/app/layer.yaml
imports:
  - version

steps:
  - action: file-create
    path: /etc/app-version
    content: ${{ version }}

  - action: run
    script: |
      version=$(sf_get version)
      echo "Building release $version"
```

### Computing a variable with `layer-run`

```yaml
# layers/build-info/layer.yaml
exports:
  - build_date
  - git_hash

steps:
  - action: layer-run
    script: |
      sf_set build_date "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      sf_set git_hash "$(git rev-parse --short HEAD)"
```

Subsequent layers can use `${{ build_date }}` and `${{ git_hash }}` in any string field.

### Reusable layer with imports and exports

```yaml
# layers/nginx/layer.yaml
imports:
  - server_name
  - document_root

vars:
  nginx_worker_processes: auto

exports:
  - server_name

steps:
  - action: pacman-add
    packages: [nginx]

  - action: file-create
    path: /etc/nginx/conf.d/default.conf
    content: |
      server {
          listen 80;
          server_name ${{ server_name }};
          root ${{ document_root }};
          worker_processes ${{ nginx_worker_processes }};
      }

  - action: systemd-service
    name: nginx
    enable: true
```

The target provides `server_name` and `document_root` through `args`. The layer provides a default for `nginx_worker_processes`. Only `server_name` is exported, keeping `document_root` and `nginx_worker_processes` internal to this layer.

## See Also

- [Actions Reference](../actions/README.md) -- all actions and their fields
- [run](../actions/run.md) -- `run` action details and `sf_get` usage
