---
title: "System Configuration"
weight: 5
---

StarForge provides four actions for configuring system identity: hostname, locale, timezone, and keymap. All four use **replace semantics** -- if multiple layers set the same value, the last layer wins.

## Hostname

The `system-hostname` action writes `/etc/hostname` in the target filesystem.

```yaml
- action: system-hostname
  hostname: my-device
```

Only one hostname can be set. A later layer can override it:

```yaml
# layers/kiosk/layer.yaml -- overrides the base hostname
- action: system-hostname
  hostname: kiosk-01
```

See the [`system-hostname` reference](../../actions/system-hostname/) for full details.

## Locale

The `system-locale` action sets the system language (`LANG=`) and optionally generates additional locales via `locale-gen`.

```yaml
- action: system-locale
  locale: en_US.UTF-8
```

The `locale` field sets the default `LANG` value. To generate additional locales beyond the default, use the `locales` field:

```yaml
- action: system-locale
  locale: en_US.UTF-8
  locales: [en_US.UTF-8, fr_FR.UTF-8, de_DE.UTF-8]
```

The `locales` list controls which locales are uncommented in `/etc/locale.gen` and built by `locale-gen` during the build. The `locale` field sets which one is used as the system default.

See the [`system-locale` reference](../../actions/system-locale/) for full details.

## Timezone

The `system-timezone` action configures the system timezone by creating the appropriate `/etc/localtime` symlink.

```yaml
- action: system-timezone
  timezone: America/New_York
```

The value must be a valid IANA timezone name (e.g., `UTC`, `America/New_York`, `Europe/London`, `Asia/Tokyo`).

See the [`system-timezone` reference](../../actions/system-timezone/) for full details.

## Keymap

The `system-keymap` action sets the console keymap by writing `/etc/vconsole.conf`. This runs in **phase 0** (preinstall) -- before packages are installed -- so that the keymap is available when `mkinitcpio` generates the initramfs.

```yaml
- action: system-keymap
  keymap: us
```

Common values include `us`, `uk`, `de`, `fr`, and `dvorak`.

See the [`system-keymap` reference](../../actions/system-keymap/) for full details.

## Combined Example

A typical base layer sets all four in sequence:

```yaml
steps:
  - action: system-hostname
    hostname: kiosk-01

  - action: system-locale
    locale: en_US.UTF-8

  - action: system-timezone
    timezone: UTC

  - action: system-keymap
    keymap: us
```

A variant layer can override individual values without repeating the others. For example, a layer targeting a French deployment only needs to set the fields that differ:

```yaml
steps:
  - action: system-hostname
    hostname: kiosk-paris

  - action: system-locale
    locale: fr_FR.UTF-8
    locales: [fr_FR.UTF-8, en_US.UTF-8]

  - action: system-timezone
    timezone: Europe/Paris

  - action: system-keymap
    keymap: fr
```

## Build Phases

| Action | Phase | What it writes |
|--------|-------|----------------|
| `system-keymap` | 0 (preinstall) | `/etc/vconsole.conf` |
| `system-hostname` | 2 (sysconfig) | `/etc/hostname` |
| `system-locale` | 2 (sysconfig) | `/etc/locale.conf`, `/etc/locale.gen` + `locale-gen` |
| `system-timezone` | 2 (sysconfig) | `/etc/localtime` symlink |

The keymap is written before `pacstrap` (phase 1) so the kernel keymap is correct from the start. The other three are applied in phase 2, after packages are installed.
