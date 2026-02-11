---
title: "Remote Layers"
weight: 12
---

Layers do not need to live inside your project directory. StarForge supports three types of remote layer sources -- git repositories, archives, and HTTP URLs -- as well as per-step external sources via `layer_source`. All remote content is fetched once and cached locally.

## Git Repository Layers

Any layer URL ending in `.git` is treated as a git repository. An optional `#branch` or `#tag` ref can be appended to pin a specific version. When no ref is given, the default branch is used.

```yaml
targets:
  device:
    layers:
      - ./layers/base
      - https://github.com/org/shared-layer.git          # default branch
      - https://github.com/org/shared-layer.git#v2.0     # pinned tag
      - https://github.com/org/shared-layer.git#develop   # specific branch
      - ./layers/app
```

Git layers are shallow cloned (depth 1), so only the files at the specified ref are downloaded. The repository root is used as the layer directory, and it must contain a `layer.yaml` at the top level.

## Archive Layers

URLs ending in `.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz`, or `.zip` are treated as archives. StarForge downloads and extracts them with the top-level directory stripped (equivalent to `--strip-components=1` for tar archives).

```yaml
targets:
  device:
    layers:
      - https://example.com/resources-v2.tar.gz
      - https://releases.example.com/base-layer-1.0.tar.xz
      - https://example.com/configs.zip
```

After extraction, the resulting directory is used as the layer directory. It must contain a `layer.yaml` at the top level, just like a local layer.

Archives are useful for distributing pre-built layer bundles that include both the `layer.yaml` and all referenced files (configuration files, scripts, etc.) in a single download.

## HTTP Remote Layers

Any other HTTP or HTTPS URL is treated as a remote layer directory. Unlike git and archive sources, StarForge does not download the entire directory. Instead, it fetches files individually based on what the layer references.

The fetch process works as follows:

1. StarForge appends `layer.yaml` to the URL and downloads it. For example, `https://example.com/layers/base/` causes a download of `https://example.com/layers/base/layer.yaml`.
2. The downloaded `layer.yaml` is scanned for `!include` directives. Any included files are fetched from the same base URL and expanded inline.
3. After include resolution, StarForge scans all steps for relative `layer_path` and `script_path` references.
4. Each referenced file is downloaded from the base URL. For example, a step with `layer_path: ./etc/hostname` fetches `https://example.com/layers/base/etc/hostname`.

```yaml
targets:
  device:
    layers:
      - https://example.com/layers/base/
      - ./layers/app
```

If the remote `layer.yaml` contains:

```yaml
steps:
  - action: file-create
    layer_path: ./etc/hostname
    path: /etc/hostname
  - action: run
    script_path: scripts/setup.sh
```

StarForge will fetch:
- `https://example.com/layers/base/layer.yaml`
- `https://example.com/layers/base/etc/hostname`
- `https://example.com/layers/base/scripts/setup.sh`

**Limitation:** Remote HTTP layers only support individual file references. A `layer_path` pointing to a directory (for recursive copying) is not supported over HTTP. Use git or archive layer sources when you need to copy entire directory trees.

## Per-Step External Sources with `layer_source`

Any individual step can pull files from an external git repository or archive using the `layer_source` field. This overrides the layer directory for that single step, leaving all other steps in the layer unaffected.

```yaml
steps:
  # Clone a git repo and copy its config files into the target
  - action: file-create
    layer_source: https://github.com/org/configs.git#v2.0
    layer_path: ./etc/myapp
    path: /etc/myapp

  # Download an archive and use a file from it
  - action: file-create
    layer_source: https://example.com/assets-v3.tar.gz
    layer_path: ./fonts/custom.ttf
    path: /usr/share/fonts/custom.ttf
```

### Building from Source

`layer_source` can be combined with `layer_script` or `layer_script_path` to build files from source before using them. The script runs inside the fetched source directory, and any files it produces become available to `layer_path`:

```yaml
steps:
  # Clone a repo, build it, and install the binary
  - action: file-create
    layer_source: https://github.com/org/app.git#main
    layer_script: |
      make build
    layer_path: ./output/app.bin
    path: /usr/local/bin/app

  # Use an external build script instead of inline
  - action: file-create
    layer_source: https://github.com/org/app.git#main
    layer_script_path: ./build.sh
    layer_path: ./dist/app
    path: /usr/local/bin/app
```

`layer_script` (inline) and `layer_script_path` (file reference) are mutually exclusive. Both require `layer_source` to be set.

## Caching

All remote content is cached locally inside the `.starforge/` directory to avoid redundant downloads:

| Source Type | Cache Location |
|-------------|----------------|
| Git repositories | `.starforge/cache/sources/{sha256(url)}/` |
| Archives | `.starforge/cache/sources/{sha256(url)}/` |
| Remote HTTP layers | `.starforge/cache/remote/{sha256(url)}/` |
| Individual file downloads | `.starforge/cache/downloads/{sha256(url)}` |

Each cached source directory contains a `.resolved` marker file that prevents re-fetching. For git sources, this marker contains the resolved commit hash.

To force a re-fetch, either delete the specific cached directory or use `starforge clean` to clear the entire cache:

```bash
# Clear all build artifacts and caches for a target
starforge clean device

# Clear only the cache
starforge clean device cache
```

Source caches under `.starforge/cache/` are shared across targets, so a git repository used by both `device` and `device-dev` is only cloned once.

## See Also

- [Project Structure](../project-structure/) -- How layer paths are specified in `starforge.yaml`.
- [Multi-Target Projects](../multi-target-projects/) -- Sharing layers across multiple targets.
- [Building & Testing](../building-and-testing/) -- Build commands and caching behavior.
- [YAML Reference](../../yaml-reference/) -- Full syntax for layer files and `!include` directives.
