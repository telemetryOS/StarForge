---
title: StarForge
layout: hextra-home
---

{{< hextra/hero-badge >}}
  <span>Open Source</span>
  {{< icon name="github" attributes="height=14" >}}
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  Build Arch Linux images&nbsp;<br class="hx:sm:block hx:hidden" />from layered YAML recipes
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  Define your OS once. Compose it into as many variants as you need.&nbsp;<br class="hx:sm:block hx:hidden" />No Arch host required --- StarForge runs on any Linux distribution.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-6 hx:flex hx:gap-4">
{{< hextra/hero-button text="Get Started" link="docs/getting-started/" >}}
<a href="docs/examples/" class="not-prose hx:font-medium hx:cursor-pointer hx:px-6 hx:py-3 hx:rounded-full hx:text-center hx:inline-block hx:border hx:border-gray-300 hx:dark:border-neutral-600 hx:text-gray-700 hx:dark:text-gray-300 hx:hover:border-gray-500 hx:dark:hover:border-gray-400 hx:hover:text-gray-900 hx:dark:hover:text-white hx:bg-transparent hx:transition-all hx:ease-in hx:duration-200">See It In Action</a>
</div>

<div class="hx:mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Define Once, Compose Many"
    subtitle="Write a base layer with partitions, packages, and system config. Stack feature layers on top. Build production, development, and installer variants from one project."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(89,112,209,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Fast Rebuilds"
    subtitle="9 build phases, each independently cached. Change a config file and only affected phases re-execute --- no full reinstall on every iteration."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(45,175,105,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="34 Built-in Actions"
    subtitle="Partitions, packages, users, files, systemd units, bootloader, scripts, and more. Everything to define a complete OS without writing shell scripts."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(221,128,54,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Test Instantly in QEMU"
    subtitle="Boot your image in a VM with one command. SSH in on port 2222, attach a serial console, or persist changes with named overlays."
  >}}
  {{< hextra/feature-card
    title="Runs on Any Linux"
    subtitle="Build tools are vendored automatically --- pacstrap, pacman, mkfs, and more. No Arch installation needed on your build machine."
  >}}
  {{< hextra/feature-card
    title="Share Layers Across Projects"
    subtitle="Pull layers from git repos, archives, or HTTP URLs. Build a shared base once and reuse it everywhere."
  >}}
{{< /hextra/feature-grid >}}
