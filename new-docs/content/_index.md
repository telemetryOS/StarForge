---
title: StarForge
layout: hextra-home
---

{{< hextra/hero-badge >}}
  <span>Declarative OS Builder</span>
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  Build custom Arch Linux&nbsp;<br class="hx:sm:block hx:hidden" />images from layered YAML recipes
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  Define partitions, packages, users, services, files, and boot configuration&nbsp;<br class="hx:sm:block hx:hidden" />declaratively. StarForge assembles bootable disk images with incremental caching.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-6 hx:flex hx:gap-4">
{{< hextra/hero-button text="Get Started" link="docs/getting-started/" >}}
{{< hextra/hero-button text="Documentation" link="docs/" style="outline" >}}
</div>

<div class="hx:mt-6"></div>

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Declarative Layers"
    subtitle="Stack YAML layers to compose OS images. Later layers override or extend earlier ones with well-defined merge semantics."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(89,112,209,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="Incremental Builds"
    subtitle="OverlayFS-based phase caching means only changed phases rebuild. SHA-256 manifests track what's changed."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(45,175,105,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="34 Built-in Actions"
    subtitle="From packages and partitions to systemd units and boot configuration. Everything you need to define a complete OS."
    style="background: radial-gradient(ellipse at 50% 80%,rgba(221,128,54,0.15),hsla(0,0%,100%,0));"
  >}}
  {{< hextra/feature-card
    title="QEMU Integration"
    subtitle="Test your images instantly with built-in QEMU support. Serial console, named overlays, and persistent changes."
  >}}
  {{< hextra/feature-card
    title="Vendored Dependencies"
    subtitle="No Arch Linux host required. Build tools are automatically downloaded and cached from Arch repositories."
  >}}
  {{< hextra/feature-card
    title="Remote Layers"
    subtitle="Pull layers from git repos, archives, or HTTP URLs. Share base configurations across projects."
  >}}
{{< /hextra/feature-grid >}}
