# Projects

A StarForge project is a directory containing a `starforge.yaml` file. This file defines the project name, an optional description, and one or more targets. StarForge searches upward from the current directory to find this file, so you can run commands from any subdirectory.

## Project File

```yaml
name: Edge-OS
description: TelemetryOS Edge Player OS

targets:
  device:
    layers:
      - ./layers/base
      - ./layers/desktop
      - ./layers/player
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Project name. |
| `description` | string | No | Human-readable description. |
| `targets` | map | Yes | Named build targets. At least one is required. |

## Targets

A target is a named build profile with an ordered list of layers. Different targets can combine different layers to produce different OS variants from a shared base.

```yaml
targets:
  device:
    args:
      version: "2.1.0"
      channel: stable
    env:
      APP_ENV: production
    layers:
      - ./layers/base
      - ./layers/desktop
      - ./layers/player

  dev:
    args:
      version: "dev"
      channel: dev
    layers:
      - ./layers/base
      - ./layers/desktop
      - ./layers/player
      - ./layers/dev-tools
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `layers` | list | Yes | Ordered list of layer paths or URLs. |
| `args` | map | No | Variables that seed the initial scope. See [Variables](variables.md). |
| `env` | map | No | Environment variables passed to `run` and `layer-run` scripts. Supports `${{ var }}` substitution. |

## Remote Layers

Layer entries can be local paths, git repositories, archives, or HTTP(S) URLs pointing to a remote layer directory.

```yaml
targets:
  device:
    layers:
      - ./layers/base                                        # local path
      - https://github.com/org/shared-layer.git#v2.0        # git repo
      - https://example.com/resources-v2.tar.gz              # archive
      - https://example.com/layers/desktop/                  # remote layer
      - ./layers/app
```

### Git repositories

URLs ending in `.git`, with an optional `#branch` or `#tag` ref. Shallow cloned into the source cache.

### Archives

`.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz`, `.zip` URLs. Extracted with the top-level directory stripped (`--strip-components=1` for tar).

### Remote HTTP(S) layers

Any other HTTP(S) URL. StarForge downloads `layer.yaml` from the URL and automatically fetches all files referenced by the layer's steps (`layer_path`, `script_path`, and `!include` paths). Relative paths in the layer are resolved against the remote URL as the layer directory root.

## Multi-Target Projects

Multiple targets can share layers. Each target has its own independent build cache, so changing one target does not affect another.

```yaml
targets:
  standard:
    layers:
      - ./layers/base
      - ./layers/desktop

  kiosk:
    layers:
      - ./layers/base
      - ./layers/kiosk
```

Build artifacts for each target are stored in separate directories under `.starforge/`:

```
.starforge/standard/cache/...
.starforge/kiosk/cache/...
```

## Source Caching

All remote sources are fetched once and cached locally:

| Source type | Cache location |
|-------------|----------------|
| Git repos and archives | `.starforge/cache/sources/{sha256(url)}/` |
| Remote HTTP layers | `.starforge/cache/remote/{sha256(url)}/` |
| Downloaded files | `.starforge/cache/downloads/{sha256(url)}` |

A `.resolved` marker file in each source directory prevents re-fetching. For git sources, the marker contains the resolved commit hash. Delete the cached directory to force a re-fetch.
