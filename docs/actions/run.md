# run

Run a script inside the target chroot during the build. Use this for custom setup tasks that cannot be expressed with other declarative actions.

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `script` | string | Conditional | Inline script content. Mutually exclusive with `script_path`. |
| `script_path` | string | Conditional | Path to a script file relative to the layer directory, or an HTTP(S) URL. Mutually exclusive with `script`. |
| `user` | string | No | Run the script as this user. Defaults to `root`. |
| `env` | map | No | Environment variables to pass to the script. Values support `${{ var }}` substitution. Step-level values override target-level `env`. |

Exactly one of `script` or `script_path` is required.

## Examples

### Inline script

```yaml
- action: run
  script: |
    echo "Configuring system..."
    systemctl preset-all
```

### Script file from layer directory

```yaml
- action: run
  script_path: ./scripts/setup.sh
```

### Run as a specific user

```yaml
- action: run
  user: player
  script: |
    mkdir -p ~/.config/autostart
    echo "User-level setup complete"
```

### Script from URL

```yaml
- action: run
  script_path: https://example.com/scripts/install-agent.sh
```

### With environment variables

```yaml
- action: run
  env:
    APP_VERSION: "2.1.0"
    CONFIG_URL: ${{ config_url }}
  script: |
    curl -o /tmp/app.tar.gz "$CONFIG_URL/releases/$APP_VERSION"
    tar xzf /tmp/app.tar.gz -C /opt/app
```

### Reading build variables

```yaml
- action: run
  script: |
    hostname=$(sf_get hostname)
    echo "Finalizing build for $hostname"
```

## Semantics

**Accumulate.** Scripts from all layers are collected and run in order during execution. Each script runs independently.

## Build Phase

Phase 8 (`scripts`). This is the last build phase -- scripts run after all other configuration (packages, files, users, services, boot) has been applied.

## Notes

- The script runs inside the target rootfs via `arch-chroot`, with full access to the built filesystem.
- Network access is available during script execution.
- Scripts can read build variables using the `sf_get` bash function, which is automatically injected into every script. For example: `sf_get my_var`.
- `sf_set` calls inside chroot scripts are no-ops. Variable output only works in [`layer-run`](../variables.md) scripts, which execute on the host during the Collect phase.
- When `user` is specified, the script runs via `su - <user> -s /bin/bash -c <script>`, which loads the user's login environment.
- If a script exits with a non-zero status, the build fails immediately.
- For file-based scripts (`script_path`), the file is copied into `/var/tmp/` inside the chroot before execution. Inline scripts are written to `/var/tmp/starforge-inline.sh`.
- A bash prelude is injected after the shebang line (or at the top if no shebang is present), providing `sf_get`, `sf_set`, and a `__sf_vars` associative array containing all collected variables.
- Target-level `env` (from `starforge.yaml`) and step-level `env` are merged, with step-level values taking precedence.
