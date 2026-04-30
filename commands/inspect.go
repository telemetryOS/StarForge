package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var inspectLayers bool

var validConcerns = []string{
	"partitions", "packages", "groups", "users", "services",
	"files", "permissions", "boot", "system", "scripts", "installer",
}

// displayOrder is the section order when printing all concerns.
// Matches the original: empty sections are skipped in all-view.
var displayOrder = []string{
	"partitions", "packages", "system", "groups", "users",
	"files", "permissions", "services", "boot", "scripts", "installer",
}

var inspectCmd = &cobra.Command{
	Use:   "inspect <target> [concern]",
	Short: "Inspect the resolved build context for a target",
	Long: `Inspect shows the final resolved state after all layers are collected.

Available concerns:
  partitions   Disk partition layout
  packages     Packages to install
  groups       Explicit group definitions
  users        User accounts
  services     Enabled/disabled/masked systemd services and default target
  files        File operations (create, edit, copy, move, link, mkdir, delete)
  permissions  File ownership (chown) and mode (chmod) settings
  boot         Bootloader configuration
  system       Hostname, locale, timezone, keymap, additional locales
  scripts      Scripts to run
  installer    Installer payloads, server, and client configuration

If no concern is specified, shows a summary of everything.
Use --layers to see which layer contributed each item.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runInspect,
}

func init() {
	inspectCmd.Flags().BoolVarP(&inspectLayers, "layers", "l", false, "show layer provenance for each item")
}

func runInspect(cmd *cobra.Command, args []string) error {
	targetName := args[0]
	concern := ""
	if len(args) > 1 {
		concern = args[1]
		if !isValidConcern(concern) {
			return fmt.Errorf("unknown concern %q — valid concerns: %s", concern, strings.Join(validConcerns, ", "))
		}
	}

	// inspect uses DryRun mode — it only parses layer configuration without
	// mounting overlayfs or writing any files, so root is not required.

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	target, ok := proj.Targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	builder := engine.NewBuilder(proj)
	builder.DryRun = true
	ctx, err := builder.Collect(target, false)
	if err != nil {
		return err
	}

	if !engine.IsInteractive() {
		// Non-interactive: print directly
		if concern != "" {
			return printConcern(concern, ctx)
		}
		for _, c := range displayOrder {
			printConcern(c, ctx)
		}
		return nil
	}

	// Interactive: split-pane TUI
	cursor := 0
	if concern != "" {
		for i, s := range displayOrder {
			if s == concern {
				cursor = i
				break
			}
		}
	}
	return runInspectTUI(targetName, ctx, cursor)
}

// section represents a sidebar entry in the inspect TUI.
type section struct {
	name  string // display name (e.g. "Partitions")
	key   string // matches displayOrder key
	count int    // item count; -1 = always show (system)
	empty bool
}

// sectionCount returns the item count for a concern, or -1 for "always show".
func sectionCount(key string, ctx *actions.BuildContext) int {
	switch key {
	case "partitions":
		return len(ctx.Partitions)
	case "packages":
		return len(ctx.Packages)
	case "system":
		return -1
	case "groups":
		return len(ctx.Groups)
	case "users":
		return len(ctx.Users)
	case "files":
		return len(ctx.FileMkdirs) + len(ctx.LayerCopies) + len(ctx.FileCreates) +
			len(ctx.FileEdits) + len(ctx.FileCopies) + len(ctx.FileMoves) +
			len(ctx.FileLinks) + len(ctx.FileDeletes)
	case "permissions":
		return len(ctx.FileOwnerships) + len(ctx.FilePermissions)
	case "services":
		n := len(ctx.Services.Enable) + len(ctx.Services.Disable) + len(ctx.Services.Mask) +
			len(ctx.Services.UserEnable) + len(ctx.Services.UserDisable)
		if ctx.DefaultTarget != "" {
			n++
		}
		return n
	case "boot":
		if ctx.Boot != nil {
			return 1
		}
		return 0
	case "scripts":
		return len(ctx.Scripts)
	case "installer":
		n := len(ctx.InstallPayloads)
		if ctx.InstallServer != nil {
			n++
		}
		if ctx.InstallClient != nil {
			n++
		}
		return n
	}
	return 0
}

// sectionDisplayName returns a title-case display name for a concern key.
func sectionDisplayName(key string) string {
	switch key {
	case "partitions":
		return "Partitions"
	case "packages":
		return "Packages"
	case "system":
		return "System"
	case "groups":
		return "Groups"
	case "users":
		return "Users"
	case "files":
		return "Files"
	case "permissions":
		return "Permissions"
	case "services":
		return "Services"
	case "boot":
		return "Boot"
	case "scripts":
		return "Scripts"
	case "installer":
		return "Installer"
	}
	return key
}

// buildSections creates the sidebar section list from a BuildContext.
func buildSections(ctx *actions.BuildContext) []section {
	sections := make([]section, len(displayOrder))
	for i, key := range displayOrder {
		count := sectionCount(key, ctx)
		sections[i] = section{
			name:  sectionDisplayName(key),
			key:   key,
			count: count,
			empty: count == 0,
		}
	}
	return sections
}

func isValidConcern(s string) bool {
	for _, c := range validConcerns {
		if c == s {
			return true
		}
	}
	return false
}

// printConcern renders a single concern to stdout.
func printConcern(concern string, ctx *actions.BuildContext) error {
	var w strings.Builder
	renderConcern(&w, concern, ctx)
	fmt.Print(w.String())
	return nil
}

// renderConcerns renders all concerns (or a single named one) to a string
// for use by the viewport TUI.
func renderConcerns(concern string, ctx *actions.BuildContext) string {
	var w strings.Builder
	if concern != "" {
		renderConcern(&w, concern, ctx)
	} else {
		for _, c := range displayOrder {
			renderConcern(&w, c, ctx)
		}
	}
	return w.String()
}

func renderConcern(w *strings.Builder, concern string, ctx *actions.BuildContext) {
	switch concern {
	case "system":
		renderSystem(w, ctx)
	case "partitions":
		if len(ctx.Partitions) == 0 {
			return
		}
		renderPartitions(w, ctx)
	case "packages":
		if len(ctx.Packages) == 0 {
			return
		}
		renderPackages(w, ctx)
	case "groups":
		if len(ctx.Groups) == 0 {
			return
		}
		renderGroups(w, ctx)
	case "users":
		if len(ctx.Users) == 0 {
			return
		}
		renderUsers(w, ctx)
	case "services":
		renderServices(w, ctx)
	case "files":
		if isConcernEmpty("files", ctx) {
			return
		}
		renderFiles(w, ctx)
	case "permissions":
		if len(ctx.FileOwnerships) == 0 && len(ctx.FilePermissions) == 0 {
			return
		}
		renderPermissions(w, ctx)
	case "boot":
		if ctx.Boot == nil {
			return
		}
		renderBoot(w, ctx)
	case "scripts":
		if len(ctx.Scripts) == 0 {
			return
		}
		renderScripts(w, ctx)
	case "installer":
		if len(ctx.InstallPayloads) == 0 && ctx.InstallServer == nil && ctx.InstallClient == nil {
			return
		}
		renderInstaller(w, ctx)
	}
}

func isConcernEmpty(concern string, ctx *actions.BuildContext) bool {
	if concern == "files" {
		return len(ctx.FileMkdirs) == 0 && len(ctx.FileCreates) == 0 &&
			len(ctx.FileEdits) == 0 && len(ctx.LayerCopies) == 0 &&
			len(ctx.FileMoves) == 0 && len(ctx.FileLinks) == 0 &&
			len(ctx.FileDeletes) == 0 && len(ctx.FileCopies) == 0
	}
	return false
}

func renderSystem(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("System"))
	defer fmt.Fprintln(w)
	renderReplaceField(w, "hostname", ctx.Hostname, ctx.HostnameHistory)
	renderReplaceField(w, "locale", ctx.Locale, ctx.LocaleHistory)
	renderReplaceField(w, "timezone", ctx.Timezone, ctx.TimezoneHistory)
	renderReplaceField(w, "keymap", ctx.Keymap, ctx.KeymapHistory)

	if len(ctx.Locales) > 0 {
		fmt.Fprintf(w, "  %-12s %s\n", "locales:", strings.Join(ctx.Locales, ", "))
	}
}

func renderReplaceField(w *strings.Builder, label, value string, history []actions.LayerValue) {
	if value == "" && len(history) == 0 {
		return
	}

	if !inspectLayers {
		fmt.Fprintf(w, "  %-12s %s\n", label+":", value)
		return
	}

	if len(history) <= 1 {
		// Single layer — always show provenance when --layers is set
		layer := ""
		if len(history) == 1 {
			layer = history[0].Layer
		}
		fmt.Fprintf(w, "  %-12s %s  %s\n", label+":", value, inspectDim.Render(layer))
		return
	}

	// Show history with overrides
	for i, h := range history {
		if i < len(history)-1 {
			fmt.Fprintf(w, "  %-12s %s  %s\n", label+":",
				inspectOverridden.Render(h.Value),
				inspectDim.Render(fmt.Sprintf("← %s (overridden)", h.Layer)))
		} else {
			fmt.Fprintf(w, "  %-12s %s  %s\n", label+":",
				inspectActive.Render(h.Value),
				inspectDim.Render(fmt.Sprintf("← %s (active)", h.Layer)))
		}
	}
}

func renderPartitions(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Partitions"))
	defer fmt.Fprintln(w)

	if len(ctx.Partitions) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}

	if inspectLayers && len(ctx.PartitionHistory) > 0 {
		// Show the active (last) layer on its own line before partition data
		activeLayer := ctx.PartitionHistory[len(ctx.PartitionHistory)-1].Layer
		fmt.Fprintf(w, "  %s\n", inspectDim.Render(activeLayer))
	}

	for _, p := range ctx.Partitions {
		sizeStr := actions.FormatSize(p.Size)
		if p.Grow {
			sizeStr += "+"
		}
		pType := p.Type
		if pType == "" {
			pType = "-"
		}
		fmt.Fprintf(w, "  %-12s %-6s %-8s %-12s %s\n",
			p.Name, p.Filesystem, sizeStr, p.MountPoint, pType)
	}
}

func renderPackages(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Packages"))
	defer fmt.Fprintln(w)

	if len(ctx.Packages) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}

	if inspectLayers && len(ctx.PackageGroups) > 0 {
		// Group by layer (layer name as header, packages indented)
		for _, g := range ctx.PackageGroups {
			fmt.Fprintf(w, "  %s\n", inspectDim.Render(g.Layer))
			for _, pkg := range g.Items {
				fmt.Fprintf(w, "    %s\n", pkg)
			}
		}
	} else {
		// Preserve layer order, deduplicate
		seen := make(map[string]bool)
		for _, pkg := range ctx.Packages {
			if !seen[pkg.Name] {
				seen[pkg.Name] = true
				fmt.Fprintf(w, "  %s\n", pkg.String())
			}
		}
	}
}

func renderGroups(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Groups"))
	defer fmt.Fprintln(w)

	if len(ctx.Groups) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}

	for _, g := range ctx.Groups {
		extra := ""
		if g.System {
			extra = " (system)"
		}
		if g.GID > 0 {
			extra += fmt.Sprintf(" gid=%d", g.GID)
		}
		layerInfo := ""
		if inspectLayers && g.Layer != "" {
			layerInfo = "  " + inspectDim.Render(g.Layer)
		}
		fmt.Fprintf(w, "  %s%s%s\n", g.Name, extra, layerInfo)
	}
}

func renderUsers(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Users"))
	defer fmt.Fprintln(w)

	if len(ctx.Users) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}

	for _, u := range ctx.Users {
		if inspectLayers && u.Layer != "" {
			line := fmt.Sprintf("  %-21s %s", u.Layer, u.Name)
			if u.Shell != "" {
				line += fmt.Sprintf("  shell: %s", u.Shell)
			}
			if len(u.Groups) > 0 {
				line += fmt.Sprintf("  groups: %s", strings.Join(u.Groups, ", "))
			}
			fmt.Fprintln(w, line)
		} else {
			line := fmt.Sprintf("  %-12s", u.Name)
			if u.Shell != "" {
				line += fmt.Sprintf(" shell: %s", u.Shell)
			}
			if len(u.Groups) > 0 {
				line += fmt.Sprintf("  groups: %s", strings.Join(u.Groups, ", "))
			}
			fmt.Fprintln(w, line)
		}
	}
}

func renderServices(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Services"))
	defer fmt.Fprintln(w)

	if len(ctx.Services.Enable) > 0 {
		if inspectLayers && len(ctx.EnableGroups) > 0 {
			fmt.Fprintln(w, "  enable")
			for _, g := range ctx.EnableGroups {
				for _, svc := range g.Items {
					fmt.Fprintf(w, "    %s\n", inspectDim.Render(g.Layer))
					fmt.Fprintf(w, "      %s\n", svc)
				}
			}
		} else {
			fmt.Fprintf(w, "  enable:   %s\n", strings.Join(ctx.Services.Enable, ", "))
		}
	}

	if len(ctx.Services.Disable) > 0 {
		if inspectLayers && len(ctx.DisableGroups) > 0 {
			fmt.Fprintln(w, "  disable")
			for _, g := range ctx.DisableGroups {
				for _, svc := range g.Items {
					fmt.Fprintf(w, "    %s\n", inspectDim.Render(g.Layer))
					fmt.Fprintf(w, "      %s\n", svc)
				}
			}
		} else {
			fmt.Fprintf(w, "  disable:  %s\n", strings.Join(ctx.Services.Disable, ", "))
		}
	}

	if len(ctx.Services.Mask) > 0 {
		if inspectLayers && len(ctx.MaskGroups) > 0 {
			fmt.Fprintln(w, "  mask")
			for _, g := range ctx.MaskGroups {
				for _, svc := range g.Items {
					fmt.Fprintf(w, "    %s\n", inspectDim.Render(g.Layer))
					fmt.Fprintf(w, "      %s\n", svc)
				}
			}
		} else {
			fmt.Fprintf(w, "  mask:     %s\n", strings.Join(ctx.Services.Mask, ", "))
		}
	}

	if len(ctx.Services.UserEnable) > 0 {
		if inspectLayers {
			fmt.Fprintln(w, "  user enable")
			for _, op := range ctx.Services.UserEnable {
				layer := ""
				if op.Layer != "" {
					layer = op.Layer
				}
				fmt.Fprintf(w, "    %s\n", inspectDim.Render(fmt.Sprintf("%s (user: %s)", layer, op.User)))
				fmt.Fprintf(w, "      %s\n", op.Service)
			}
		} else {
			byUser := groupUserServices(ctx.Services.UserEnable)
			for _, ug := range byUser {
				fmt.Fprintf(w, "  enable:   %s (user: %s)\n",
					strings.Join(ug.services, ", "), ug.user)
			}
		}
	}

	if len(ctx.Services.UserDisable) > 0 {
		if inspectLayers {
			fmt.Fprintln(w, "  user disable")
			for _, op := range ctx.Services.UserDisable {
				layer := ""
				if op.Layer != "" {
					layer = op.Layer
				}
				fmt.Fprintf(w, "    %s\n", inspectDim.Render(fmt.Sprintf("%s (user: %s)", layer, op.User)))
				fmt.Fprintf(w, "      %s\n", op.Service)
			}
		} else {
			byUser := groupUserServices(ctx.Services.UserDisable)
			for _, ug := range byUser {
				fmt.Fprintf(w, "  disable:  %s (user: %s)\n",
					strings.Join(ug.services, ", "), ug.user)
			}
		}
	}

	if ctx.DefaultTarget != "" {
		if inspectLayers && len(ctx.DefaultTargetHistory) > 0 {
			layer := ctx.DefaultTargetHistory[len(ctx.DefaultTargetHistory)-1].Layer
			fmt.Fprintf(w, "  target:      %s  %s\n", ctx.DefaultTarget, inspectDim.Render(layer))
		} else {
			fmt.Fprintf(w, "  target:      %s\n", ctx.DefaultTarget)
		}
	}
}

type userServiceGroup struct {
	user     string
	services []string
}

func groupUserServices(ops []actions.UserServiceOp) []userServiceGroup {
	var groups []userServiceGroup
	for _, op := range ops {
		found := false
		for i := range groups {
			if groups[i].user == op.User {
				groups[i].services = append(groups[i].services, op.Service)
				found = true
				break
			}
		}
		if !found {
			groups = append(groups, userServiceGroup{user: op.User, services: []string{op.Service}})
		}
	}
	return groups
}

func renderFiles(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Files"))
	defer fmt.Fprintln(w)

	empty := len(ctx.FileMkdirs) == 0 && len(ctx.FileCreates) == 0 &&
		len(ctx.FileEdits) == 0 && len(ctx.LayerCopies) == 0 &&
		len(ctx.FileMoves) == 0 && len(ctx.FileLinks) == 0 &&
		len(ctx.FileDeletes) == 0 && len(ctx.FileCopies) == 0

	if empty {
		fmt.Fprintln(w, "  (none)")
		return
	}

	// Copies (directory copies from layer_path/layer_source) are shown as "create ... (dir copy)"
	for _, cp := range ctx.LayerCopies {
		desc := fmt.Sprintf("%s -> %s  (dir copy)", cp.FromPath, cp.ToPath)
		renderFileLine(w, "create", desc, cp.Label, cp.Layer)
	}

	for _, fc := range ctx.FileCreates {
		mode := ""
		if fc.Mode != "" {
			mode = " " + fc.Mode
		}
		desc := fmt.Sprintf("%-30s%s", fc.Path, mode)
		renderFileLine(w, "create", desc, fc.Label, fc.Layer)
	}

	for _, fe := range ctx.FileEdits {
		extra := ""
		if fe.Insert != "" {
			extra += fmt.Sprintf(" insert=%s", fe.Insert)
		}
		if fe.Pattern != "" {
			extra += fmt.Sprintf(" pattern=%q", fe.Pattern)
		}
		renderFileLine(w, "edit", fe.Path+extra, fe.Label, fe.Layer)
	}

	for _, ic := range ctx.FileCopies {
		renderFileLine(w, "icopy", fmt.Sprintf("%s -> %s", ic.FromPath, ic.ToPath), ic.Label, ic.Layer)
	}

	for _, mv := range ctx.FileMoves {
		renderFileLine(w, "move", fmt.Sprintf("%s -> %s", mv.FromPath, mv.ToPath), mv.Label, mv.Layer)
	}

	for _, ln := range ctx.FileLinks {
		renderFileLine(w, "link", fmt.Sprintf("%s -> %s (%s)", ln.ToPath, ln.FromPath, ln.Type), ln.Label, ln.Layer)
	}

	for _, r := range ctx.FileDeletes {
		extra := ""
		if r.Recursive {
			extra = " (recursive)"
		}
		renderFileLine(w, "delete", r.Path+extra, r.Label, r.Layer)
	}

	for _, m := range ctx.FileMkdirs {
		extra := ""
		if m.Mode != "" {
			extra += " mode=" + m.Mode
		}
		if m.Owner != "" {
			extra += " owner=" + m.Owner
		}
		renderFileLine(w, "mkdir", m.Path+extra, m.Label, m.Layer)
	}
}

// renderFileLine renders a single file operation line.
// When --layers is set, the layer path is prefixed; otherwise it's omitted.
func renderFileLine(w *strings.Builder, op, desc, label, layer string) {
	labelStr := ""
	if label != "" {
		labelStr = "  " + label
	}
	if inspectLayers && layer != "" {
		fmt.Fprintf(w, "  %-21s %-7s %s%s\n", layer, op, desc, labelStr)
	} else {
		fmt.Fprintf(w, "  %-7s %s%s\n", op, desc, labelStr)
	}
}

func renderPermissions(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Permissions"))
	defer fmt.Fprintln(w)

	if len(ctx.FileOwnerships) == 0 && len(ctx.FilePermissions) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}

	for _, o := range ctx.FileOwnerships {
		extra := ""
		if o.Recursive {
			extra = " (recursive)"
		}
		label := ""
		if o.Label != "" {
			label = "  " + o.Label
		}
		if inspectLayers && o.Layer != "" {
			fmt.Fprintf(w, "  %-21s %-30s chown %s:%s%s%s\n", o.Layer, o.Path, o.Owner, o.Group, extra, label)
		} else {
			fmt.Fprintf(w, "  %-30s chown %s:%s%s%s\n", o.Path, o.Owner, o.Group, extra, label)
		}
	}

	for _, p := range ctx.FilePermissions {
		extra := ""
		if p.Recursive {
			extra = " (recursive)"
		}
		label := ""
		if p.Label != "" {
			label = "  " + p.Label
		}
		if inspectLayers && p.Layer != "" {
			fmt.Fprintf(w, "  %-21s %-30s chmod %s%s%s\n", p.Layer, p.Path, p.Mode, extra, label)
		} else {
			fmt.Fprintf(w, "  %-30s chmod %s%s%s\n", p.Path, p.Mode, extra, label)
		}
	}
}

func renderBoot(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Boot"))
	defer fmt.Fprintln(w)

	if ctx.Boot == nil {
		fmt.Fprintln(w, "  (not configured)")
		return
	}

	if inspectLayers && ctx.Boot.Layer != "" {
		fmt.Fprintf(w, "  %s\n", inspectDim.Render(fmt.Sprintf("%s (active)", ctx.Boot.Layer)))
	}

	if ctx.Boot.Loader != nil {
		fmt.Fprintf(w, "  loader: default=%s timeout=%d editor=%v\n",
			ctx.Boot.Loader.Default, ctx.Boot.Loader.Timeout, ctx.Boot.Loader.Editor)
	} else {
		fmt.Fprintf(w, "  loader: (none — entries-only)\n")
	}
	for _, e := range ctx.Boot.Entries {
		fmt.Fprintf(w, "  entry:  %s\n", e.Name)
		fmt.Fprintf(w, "    title:   %s\n", e.Title)
		fmt.Fprintf(w, "    kernel:  %s\n", e.Kernel)
		if e.Path != "" {
			fmt.Fprintf(w, "    path:    %s\n", e.Path)
		}
		if e.Extended != nil {
			fmt.Fprintf(w, "    extended: %v\n", *e.Extended)
		}
		fmt.Fprintf(w, "    options: %s\n", e.Options)
	}
}

func renderScripts(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Scripts"))
	defer fmt.Fprintln(w)

	if len(ctx.Scripts) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}

	for _, s := range ctx.Scripts {
		desc := s.Script
		if desc == "" {
			desc = "(inline)"
		}

		extra := ""
		if s.User != "" {
			extra = fmt.Sprintf("  user: %s", s.User)
		}

		if inspectLayers && s.Layer != "" {
			fmt.Fprintf(w, "  %-21s %s%s\n", s.Layer, desc, extra)
		} else {
			fmt.Fprintf(w, "  %s%s\n", desc, extra)
		}
	}
}

func renderInstaller(w *strings.Builder, ctx *actions.BuildContext) {
	fmt.Fprintln(w, inspectHeader.Render("Installer"))
	defer fmt.Fprintln(w)

	if len(ctx.InstallPayloads) == 0 && ctx.InstallServer == nil && ctx.InstallClient == nil {
		fmt.Fprintln(w, "  (not configured)")
		return
	}

	if len(ctx.InstallPayloads) > 0 {
		fmt.Fprintln(w, "  payloads")
		for _, p := range ctx.InstallPayloads {
			label := ""
			if p.Label != "" {
				label = " " + p.Label
			}
			if inspectLayers && p.Layer != "" {
				fmt.Fprintf(w, "    %s  %s%s\n", p.Layer, p.Target, label)
			} else {
				fmt.Fprintf(w, "    %s%s\n", p.Target, label)
			}
		}
	}

	if ctx.InstallServer != nil {
		layerInfo := ""
		if inspectLayers && ctx.InstallServer.Layer != "" {
			layerInfo = "  " + inspectDim.Render(ctx.InstallServer.Layer)
		}
		fmt.Fprintf(w, "  server  port: %d%s\n", ctx.InstallServer.Port, layerInfo)
	}

	if ctx.InstallClient != nil {
		layerInfo := ""
		if inspectLayers && ctx.InstallClient.Layer != "" {
			layerInfo = "  " + inspectDim.Render(ctx.InstallClient.Layer)
		}
		if ctx.InstallClient.AutoLogin != "" {
			fmt.Fprintf(w, "  client  auto_login: %s%s\n", ctx.InstallClient.AutoLogin, layerInfo)
		} else {
			fmt.Fprintf(w, "  client%s\n", layerInfo)
		}
	}
}
