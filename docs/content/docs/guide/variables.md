---
title: "Variables"
weight: 11
---

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

Variable names must match `[a-zA-Z_][a-zA-Z0-9_]*`. Whitespace inside the braces is ignored -- `${{ version }}` and `${{version}}` are equivalent.

If a referenced variable is not defined in the current scope, the build fails immediately with an error identifying the undefined variable and the step that referenced it.

## Variable Scope Chain

Variables flow through four mechanisms, evaluated in a specific order for each layer.

### Target `args`

The `args` field in `starforge.yaml` seeds the initial variable scope for a target. These values are available to all layers.

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

#### Environment variable expansion

Arg values support shell-style environment variable expansion. Values containing `$NAME` or `${NAME}` are resolved from the host environment at build time.

```yaml
targets:
  device:
    args:
      hostname: $EDGE_HOSTNAME
      channel: ${UPDATE_CHANNEL}
      label: device-${EDGE_HOSTNAME}
      version: "2.1.0"
    layers:
      - ./layers/base
```

This lets you pass dynamic values without modifying the project file:

```bash
EDGE_HOSTNAME=factory-test UPDATE_CHANNEL=dev starforge build device
```

If an env var is not set, it expands to an empty string. Use `default_env` to provide fallback values:

```yaml
targets:
  device:
    args:
      hostname: $EDGE_HOSTNAME
      channel: $UPDATE_CHANNEL
    default_env:
      EDGE_HOSTNAME: edge-device
      UPDATE_CHANNEL: stable
    layers:
      - ./layers/base
```

With `default_env`, the arg uses the environment variable if set, otherwise the default. This keeps the project buildable without requiring any environment setup while still allowing overrides.

Plain string values (without `$`) are unaffected by expansion and work exactly as before.

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

Use `imports` to document a layer's requirements and catch missing variables early. This is especially valuable for reusable layers that will be consumed by multiple projects or targets.

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

In this example, `commit_hash` is exported to subsequent layers, but `repo_url` and any internal variables the layer may have defined are filtered out.

## Scoping Flow

The complete flow for a target with three layers:

```
Target args (after env expansion): { version: "2.1.0", channel: "stable" }
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

## Dynamic Variables with `layer-run`

The `layer-run` action executes a script on the host during the Collect phase. It can compute values at build time and inject them into the variable scope using `sf_set`.

```yaml
- action: layer-run
  script: |
    hash=$(git -C /path/to/repo rev-parse --short HEAD)
    sf_set git_hash "$hash"

    version=$(sf_get version)
    sf_set full_version "${version}-${hash}"
```

Variables set by `sf_set` update the layer-scoped variable map. They are subject to the same `exports` filtering as `vars` -- if the layer has an `exports` field, only listed variables propagate.

The `sf_get` function reads variables from the current scope. Both functions are injected automatically into every `layer-run` script.

For more on the `layer-run` action, see [Scripts](../scripts/).

## Environment Variables

Environment variables are separate from build variables. They are passed to scripts (`run` and `layer-run`) as actual process environment variables, not substituted into YAML.

### Target-Level `env`

Defined in `starforge.yaml` on the target. Values support `${{ var }}` substitution, resolved against the target `args` at the start of the build.

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

### Step-Level `env`

Defined on `run` and `layer-run` steps. Step-level values override target-level values for the same key. Also supports `${{ var }}` substitution.

```yaml
- action: run
  env:
    INSTALL_DIR: /opt/app
    APP_VERSION: ${{ version }}
  script: |
    echo "Installing to $INSTALL_DIR version $APP_VERSION"
```

### `STARFORGE_VAR_*` Environment Variables

In `layer-run` scripts, all current build variables are also available as `STARFORGE_VAR_<NAME>` environment variables, with names uppercased. For example, a variable named `version` is available as `STARFORGE_VAR_VERSION`. This provides an alternative to `sf_get` for reading variables.

## Using Variables in `run` Scripts

Scripts executed by the `run` action run inside the target chroot during phase 8. They have access to build variables through the `sf_get` function:

```yaml
- action: run
  script: |
    version=$(sf_get version)
    hostname=$(sf_get hostname)
    echo "Finalizing $hostname at version $version"
```

The build engine injects a bash prelude into every script that defines `sf_get`, `sf_set`, and a `__sf_vars` associative array containing all collected variables. In chroot scripts, `sf_set` is a no-op -- variable output only works in `layer-run` scripts that execute on the host during Collect.

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

- [Scripts](../scripts/) -- The `run` and `layer-run` actions in detail.
- [run reference](../../actions/run/) -- Complete field reference for chroot scripts and `sf_get` usage.
- [layer-run reference](../../actions/layer-run/) -- Complete field reference for host-side scripts and `sf_set` usage.
- [Writing Layers](../writing-layers/) -- Layer anatomy, including `vars`, `imports`, and `exports` fields.
- [YAML Reference](../../yaml-reference/) -- Full YAML syntax and substitution details.
