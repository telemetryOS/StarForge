---
title: "StarForge Documentation"
linkTitle: "Overview"
weight: 1
---


Build custom Arch Linux images from layered YAML recipes.

From zero to a bootable USB in four commands:

```bash
starforge init my-os && cd my-os
starforge build distribution
starforge run distribution
starforge write distribution /dev/sdX
```

StarForge composes partitions, packages, users, files, services, and boot configuration into disk images --- declaratively, incrementally, and reproducibly. Define a shared base layer once, then combine it with feature and variant layers to produce as many OS targets as you need from a single project.

## Why StarForge

**Define once, compose many.** Write a base layer with your partitions, packages, and system config. Stack feature layers for a desktop, a kiosk lockdown, or a development environment. StarForge merges them in order and builds each target independently --- one project, many OS variants, zero duplication.

**Declarative, not imperative.** Your OS is YAML, not shell scripts. Every action is collected before the build starts, so StarForge sees the full picture before it touches the filesystem. No ordering surprises. No half-applied states.

**Fast rebuilds.** The build pipeline is split into 9 phases, each independently hashed and cached via overlayfs snapshots. Change a service file and only the affected phases re-execute --- no waiting for a full `pacstrap` on every iteration.

**No Arch host required.** StarForge vendors its own build tools (pacstrap, pacman, mkfs, sgdisk) on first use. Run it on Ubuntu, Fedora, or any Linux distribution with a modern kernel.

## Get Started

1. [**Getting Started**](getting-started/) --- Install StarForge, scaffold a project, build your first image, and boot it in QEMU.
2. [**Complete Guide**](guide/) --- Topic-by-topic walkthrough covering partitions, packages, users, files, systemd units, variables, multi-target projects, and deployment.
3. [**Examples**](examples/) --- Walk through a complete multi-target kiosk project with 4 targets built from 7 layers, plus common patterns you can copy.

## Reference

- [**Actions**](actions/) --- All 32 built-in actions with fields, override semantics, and examples.
- [**Commands**](commands/) --- CLI reference for all 10 commands.
- [**YAML Reference**](yaml-reference/) --- Custom tags (`!include`, `!add`, `!replace`, and more), quoting rules, and systemd INI field names.
- [**Troubleshooting**](troubleshooting/) --- Common issues and solutions.
