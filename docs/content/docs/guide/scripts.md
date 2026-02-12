---
title: "Scripts"
weight: 10
---

StarForge provides two script actions that serve different purposes. The `run` action executes scripts inside the target chroot during the final build phase. The `layer-run` action executes scripts on the host during the Collect phase, before any build phases begin. Understanding when each runs is key to using them effectively.

## The `run` Action (Chroot Scripts)

The `run` action executes a shell script inside the target filesystem via `arch-chroot`. It runs in phase 8, the last build phase -- after packages are installed, files are written, users are created, services are enabled, and the bootloader is configured. This makes it suitable for any final setup that cannot be expressed with declarative actions.

### Inline Scripts

Use the `script` field for short, self-contained scripts:

```yaml
- action: run
  script: |
    # Enable multilib repository
    sed -i '/\[multilib\]/,/Include/s/^#//' /etc/pacman.conf
    pacman -Sy
```

### File-Based Scripts

Use `script_path` to reference a script file in your layer directory:

```yaml
- action: run
  script_path: ./scripts/post-install.sh
```

The path is relative to the layer directory. The file is copied into `/var/tmp/` inside the chroot before execution. You can also reference a remote script by URL:

```yaml
- action: run
  script_path: https://example.com/scripts/install-agent.sh
```

The `script` and `script_path` fields are mutually exclusive -- use one or the other, not both.

### Running as a Specific User

By default, scripts run as root. Use the `user` field to run as a different user:

```yaml
- action: run
  user: player
  script: |
    mkdir -p ~/.config/autostart
    cp /usr/share/applications/myapp.desktop ~/.config/autostart/
    echo "User setup complete"
```

When `user` is specified, the script runs via `su - <user> -s /bin/bash -c <script>`, which loads the user's login environment.

### Environment Variables

Pass environment variables to scripts with the `env` field. Values support `${{ var }}` substitution:

```yaml
- action: run
  env:
    APP_VERSION: ${{ version }}
    CONFIG_URL: https://example.com/config
  script: |
    curl -o /tmp/app.tar.gz "$CONFIG_URL/releases/$APP_VERSION"
    tar xzf /tmp/app.tar.gz -C /opt/app
```

Step-level `env` values override target-level `env` values defined in `starforge.yaml`. Both are available as standard environment variables inside the script.

### Reading Build Variables with `sf_get`

Every chroot script has access to the `sf_get` function, which reads build variables from the current scope:

```yaml
- action: run
  script: |
    hostname=$(sf_get hostname)
    version=$(sf_get version)
    echo "Finalizing $hostname at version $version"
```

The build engine injects a bash prelude into every script that defines `sf_get`, `sf_set`, and a `__sf_vars` associative array containing all collected variables.

Note that `sf_set` is a no-op in chroot scripts. Variable output only works in `layer-run` scripts, which run on the host during the Collect phase. If you need to compute a variable for use in later steps, use `layer-run` instead.

### Debugging Tips

Add `set -ex` at the top of your scripts during development. The `-e` flag causes the script to fail on the first error, and `-x` prints each command before executing it:

```yaml
- action: run
  script: |
    set -ex
    systemctl preset-all
    useradd -m testuser
```

If a script exits with a non-zero status, the build fails immediately. You can also use `starforge chroot <target>` to enter the built filesystem interactively and test commands before scripting them:

```bash
starforge chroot distribution
```

## The `layer-run` Action (Host-Side Scripts)

The `layer-run` action executes a script on the host machine during the Collect phase, before any build phases begin. Its primary purpose is computing build variables dynamically.

Unlike `run`, which operates inside the chroot, `layer-run` runs on the host with full access to the build environment. It cannot modify the target filesystem.

### Computing Variables with `sf_set`

The `sf_set` function sets a variable in the build scope. The variable is immediately available to subsequent steps via `${{ var_name }}` substitution:

```yaml
- action: layer-run
  script: |
    sf_set build_date "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    sf_set git_hash "$(git rev-parse --short HEAD)"
```

### Reading Variables with `sf_get`

The `sf_get` function retrieves a previously set variable:

```yaml
- action: layer-run
  script: |
    version=$(sf_get version)
    hash=$(git rev-parse --short HEAD)
    sf_set full_version "${version}-${hash}"
```

### Environment

All current build variables are available as `STARFORGE_VAR_<NAME>` environment variables (names are uppercased). Step-level and target-level `env` values are also available:

```yaml
- action: layer-run
  env:
    REGISTRY: https://registry.example.com
    APP_NAME: ${{ app_name }}
  script: |
    latest=$(curl -s "$REGISTRY/api/$APP_NAME/latest")
    sf_set app_version "$latest"
```

### Using `layer_source`

When `layer_source` is set on a `layer-run` step, the script's working directory is the resolved source directory. This lets you extract version information from external repositories:

```yaml
- action: layer-run
  layer_source: https://github.com/org/app.git#main
  script: |
    sf_set app_version "$(git describe --tags)"
```

### File-Based Scripts

Just like `run`, you can use `script_path` instead of `script`:

```yaml
- action: layer-run
  script_path: ./scripts/compute-vars.sh
```

### Variable Scoping

Variables set by `sf_set` in `layer-run` are subject to the same scoping rules as `vars`. They update the layer-scoped variable map and are filtered by the layer's `exports` field if one is defined.

For full details on how variables flow through layers, see [Variables](../variables/).

## Comparison

| Feature | `run` | `layer-run` |
|---------|-------|-------------|
| When it runs | Phase 8 (last build phase) | Collect phase (before build) |
| Where it runs | Inside the target chroot | On the host machine |
| `sf_get` | Works | Works |
| `sf_set` | No-op | Sets build variables |
| `user` field | Supported | Not available |
| Can modify target filesystem | Yes (via chroot) | No |
| Network access | Yes | Yes |
| Primary use | Final setup tasks | Dynamic variable computation |

## Accumulation

Both `run` and `layer-run` use accumulate semantics. Scripts from all layers are collected and run in order. Each script runs independently.

## See Also

- [run reference](../../actions/run/) -- Complete field reference for chroot scripts.
- [layer-run reference](../../actions/layer-run/) -- Complete field reference for host-side scripts.
- [Variables](../variables/) -- Full variable system documentation, including scope chain and `exports`.
- [Building & Testing](../building-and-testing/) -- Using `starforge chroot` for interactive debugging.
