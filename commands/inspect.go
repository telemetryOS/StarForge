package commands

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/telemetryos/starforge/actions"
	"github.com/telemetryos/starforge/config"
	"github.com/telemetryos/starforge/engine"
)

var inspectLayers bool

var (
	inspectHeader     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	inspectDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	inspectOverridden = lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("8"))
	inspectActive     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

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

	proj, err := config.FindProject()
	if err != nil {
		return err
	}

	target, ok := proj.Targets[targetName]
	if !ok {
		return fmt.Errorf("unknown target %q", targetName)
	}

	builder := engine.NewBuilder(proj)
	ctx, err := builder.Collect(target, false)
	if err != nil {
		return err
	}

	if concern != "" {
		return printConcern(concern, ctx)
	}

	// Print all concerns in display order, skipping empty sections
	for _, c := range displayOrder {
		printConcern(c, ctx)
	}

	return nil
}

func isValidConcern(s string) bool {
	for _, c := range validConcerns {
		if c == s {
			return true
		}
	}
	return false
}

func printConcern(concern string, ctx *actions.BuildContext) error {
	switch concern {
	case "system":
		printSystem(ctx)
	case "partitions":
		if len(ctx.Partitions) == 0 {
			return nil
		}
		printPartitions(ctx)
	case "packages":
		if len(ctx.Packages) == 0 {
			return nil
		}
		printPackages(ctx)
	case "groups":
		if len(ctx.Groups) == 0 {
			return nil
		}
		printGroups(ctx)
	case "users":
		if len(ctx.Users) == 0 {
			return nil
		}
		printUsers(ctx)
	case "services":
		printServices(ctx)
	case "files":
		if isConcernEmpty("files", ctx) {
			return nil
		}
		printFiles(ctx)
	case "permissions":
		if len(ctx.Ownerships) == 0 && len(ctx.Permissions) == 0 {
			return nil
		}
		printPermissions(ctx)
	case "boot":
		if ctx.Boot == nil {
			return nil
		}
		printBoot(ctx)
	case "scripts":
		if len(ctx.Scripts) == 0 {
			return nil
		}
		printScripts(ctx)
	case "installer":
		if len(ctx.InstallerPayloads) == 0 && ctx.InstallerServer == nil && ctx.InstallerClient == nil {
			return nil
		}
		printInstaller(ctx)
	}
	return nil
}

func isConcernEmpty(concern string, ctx *actions.BuildContext) bool {
	if concern == "files" {
		return len(ctx.Mkdirs) == 0 && len(ctx.FileCreates) == 0 &&
			len(ctx.FileEdits) == 0 && len(ctx.Copies) == 0 &&
			len(ctx.Moves) == 0 && len(ctx.Links) == 0 &&
			len(ctx.Removes) == 0 && len(ctx.InternalCopies) == 0
	}
	return false
}

func printSystem(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("System"))
	defer fmt.Println()
	printReplaceField("hostname", ctx.Hostname, ctx.HostnameHistory)
	printReplaceField("locale", ctx.Locale, ctx.LocaleHistory)
	printReplaceField("timezone", ctx.Timezone, ctx.TimezoneHistory)
	printReplaceField("keymap", ctx.Keymap, ctx.KeymapHistory)

	if len(ctx.Locales) > 0 {
		fmt.Printf("  %-12s %s\n", "locales:", strings.Join(ctx.Locales, ", "))
	}
}

func printReplaceField(label, value string, history []actions.LayerValue) {
	if value == "" && len(history) == 0 {
		return
	}

	if !inspectLayers {
		fmt.Printf("  %-12s %s\n", label+":", value)
		return
	}

	if len(history) <= 1 {
		// Single layer — always show provenance when --layers is set
		layer := ""
		if len(history) == 1 {
			layer = history[0].Layer
		}
		fmt.Printf("  %-12s %s  %s\n", label+":", value, inspectDim.Render(layer))
		return
	}

	// Show history with overrides
	for i, h := range history {
		if i < len(history)-1 {
			fmt.Printf("  %-12s %s  %s\n", label+":",
				inspectOverridden.Render(h.Value),
				inspectDim.Render(fmt.Sprintf("← %s (overridden)", h.Layer)))
		} else {
			fmt.Printf("  %-12s %s  %s\n", label+":",
				inspectActive.Render(h.Value),
				inspectDim.Render(fmt.Sprintf("← %s (active)", h.Layer)))
		}
	}
}

func printPartitions(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Partitions"))
	defer fmt.Println()

	if len(ctx.Partitions) == 0 {
		fmt.Println("  (none)")
		return
	}

	if inspectLayers && len(ctx.PartitionHistory) > 0 {
		// Show the active (last) layer on its own line before partition data
		activeLayer := ctx.PartitionHistory[len(ctx.PartitionHistory)-1].Layer
		fmt.Printf("  %s\n", inspectDim.Render(activeLayer))
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
		fmt.Printf("  %-12s %-6s %-8s %-12s %s\n",
			p.Name, p.Filesystem, sizeStr, p.MountPoint, pType)
	}
}

func printPackages(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Packages"))
	defer fmt.Println()

	if len(ctx.Packages) == 0 {
		fmt.Println("  (none)")
		return
	}

	if inspectLayers && len(ctx.PackageGroups) > 0 {
		// Group by layer (layer name as header, packages indented)
		for _, g := range ctx.PackageGroups {
			fmt.Printf("  %s\n", inspectDim.Render(g.Layer))
			for _, pkg := range g.Items {
				fmt.Printf("    %s\n", pkg)
			}
		}
	} else {
		// Preserve layer order, deduplicate
		seen := make(map[string]bool)
		for _, pkg := range ctx.Packages {
			if !seen[pkg] {
				seen[pkg] = true
				fmt.Printf("  %s\n", pkg)
			}
		}
	}
}

func printGroups(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Groups"))
	defer fmt.Println()

	if len(ctx.Groups) == 0 {
		fmt.Println("  (none)")
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
		fmt.Printf("  %s%s%s\n", g.Name, extra, layerInfo)
	}
}

func printUsers(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Users"))
	defer fmt.Println()

	if len(ctx.Users) == 0 {
		fmt.Println("  (none)")
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
			fmt.Println(line)
		} else {
			line := fmt.Sprintf("  %-12s", u.Name)
			if u.Shell != "" {
				line += fmt.Sprintf(" shell: %s", u.Shell)
			}
			if len(u.Groups) > 0 {
				line += fmt.Sprintf("  groups: %s", strings.Join(u.Groups, ", "))
			}
			fmt.Println(line)
		}
	}
}

func printServices(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Services"))
	defer fmt.Println()

	if len(ctx.Services.Enable) > 0 {
		if inspectLayers && len(ctx.EnableGroups) > 0 {
			fmt.Println("  enable")
			for _, g := range ctx.EnableGroups {
				for _, svc := range g.Items {
					fmt.Printf("    %s\n", inspectDim.Render(g.Layer))
					fmt.Printf("      %s\n", svc)
				}
			}
		} else {
			fmt.Printf("  enable:   %s\n", strings.Join(ctx.Services.Enable, ", "))
		}
	}

	if len(ctx.Services.Disable) > 0 {
		if inspectLayers && len(ctx.DisableGroups) > 0 {
			fmt.Println("  disable")
			for _, g := range ctx.DisableGroups {
				for _, svc := range g.Items {
					fmt.Printf("    %s\n", inspectDim.Render(g.Layer))
					fmt.Printf("      %s\n", svc)
				}
			}
		} else {
			fmt.Printf("  disable:  %s\n", strings.Join(ctx.Services.Disable, ", "))
		}
	}

	if len(ctx.Services.Mask) > 0 {
		if inspectLayers && len(ctx.MaskGroups) > 0 {
			fmt.Println("  mask")
			for _, g := range ctx.MaskGroups {
				for _, svc := range g.Items {
					fmt.Printf("    %s\n", inspectDim.Render(g.Layer))
					fmt.Printf("      %s\n", svc)
				}
			}
		} else {
			fmt.Printf("  mask:     %s\n", strings.Join(ctx.Services.Mask, ", "))
		}
	}

	if len(ctx.Services.UserEnable) > 0 {
		if inspectLayers {
			fmt.Println("  user enable")
			for _, op := range ctx.Services.UserEnable {
				layer := ""
				if op.Layer != "" {
					layer = op.Layer
				}
				fmt.Printf("    %s\n", inspectDim.Render(fmt.Sprintf("%s (user: %s)", layer, op.User)))
				fmt.Printf("      %s\n", op.Service)
			}
		} else {
			byUser := groupUserServices(ctx.Services.UserEnable)
			for _, ug := range byUser {
				fmt.Printf("  enable:   %s (user: %s)\n",
					strings.Join(ug.services, ", "), ug.user)
			}
		}
	}

	if len(ctx.Services.UserDisable) > 0 {
		if inspectLayers {
			fmt.Println("  user disable")
			for _, op := range ctx.Services.UserDisable {
				layer := ""
				if op.Layer != "" {
					layer = op.Layer
				}
				fmt.Printf("    %s\n", inspectDim.Render(fmt.Sprintf("%s (user: %s)", layer, op.User)))
				fmt.Printf("      %s\n", op.Service)
			}
		} else {
			byUser := groupUserServices(ctx.Services.UserDisable)
			for _, ug := range byUser {
				fmt.Printf("  disable:  %s (user: %s)\n",
					strings.Join(ug.services, ", "), ug.user)
			}
		}
	}

	if ctx.DefaultTarget != "" {
		if inspectLayers && len(ctx.DefaultTargetHistory) > 0 {
			layer := ctx.DefaultTargetHistory[len(ctx.DefaultTargetHistory)-1].Layer
			fmt.Printf("  target:      %s  %s\n", ctx.DefaultTarget, inspectDim.Render(layer))
		} else {
			fmt.Printf("  target:      %s\n", ctx.DefaultTarget)
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

func printFiles(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Files"))
	defer fmt.Println()

	empty := len(ctx.Mkdirs) == 0 && len(ctx.FileCreates) == 0 &&
		len(ctx.FileEdits) == 0 && len(ctx.Copies) == 0 &&
		len(ctx.Moves) == 0 && len(ctx.Links) == 0 &&
		len(ctx.Removes) == 0 && len(ctx.InternalCopies) == 0

	if empty {
		fmt.Println("  (none)")
		return
	}

	// Copies (directory copies from layer_path/layer_source) are shown as "create ... (dir copy)"
	for _, cp := range ctx.Copies {
		desc := fmt.Sprintf("%s -> %s  (dir copy)", cp.FromPath, cp.ToPath)
		printFileLine("create", desc, cp.Label, cp.Layer)
	}

	for _, fc := range ctx.FileCreates {
		mode := ""
		if fc.Mode != "" {
			mode = " " + fc.Mode
		}
		desc := fmt.Sprintf("%-30s%s", fc.Path, mode)
		printFileLine("create", desc, fc.Label, fc.Layer)
	}

	for _, fe := range ctx.FileEdits {
		extra := ""
		if fe.Insert != "" {
			extra += fmt.Sprintf(" insert=%s", fe.Insert)
		}
		if fe.Pattern != "" {
			extra += fmt.Sprintf(" pattern=%q", fe.Pattern)
		}
		printFileLine("edit", fe.Path+extra, fe.Label, fe.Layer)
	}

	for _, ic := range ctx.InternalCopies {
		printFileLine("icopy", fmt.Sprintf("%s -> %s", ic.FromPath, ic.ToPath), ic.Label, ic.Layer)
	}

	for _, mv := range ctx.Moves {
		printFileLine("move", fmt.Sprintf("%s -> %s", mv.FromPath, mv.ToPath), mv.Label, mv.Layer)
	}

	for _, ln := range ctx.Links {
		printFileLine("link", fmt.Sprintf("%s -> %s (%s)", ln.ToPath, ln.FromPath, ln.Type), ln.Label, ln.Layer)
	}

	for _, r := range ctx.Removes {
		extra := ""
		if r.Recursive {
			extra = " (recursive)"
		}
		printFileLine("delete", r.Path+extra, r.Label, r.Layer)
	}

	for _, m := range ctx.Mkdirs {
		extra := ""
		if m.Mode != "" {
			extra += " mode=" + m.Mode
		}
		if m.Owner != "" {
			extra += " owner=" + m.Owner
		}
		printFileLine("mkdir", m.Path+extra, m.Label, m.Layer)
	}
}

// printFileLine prints a single file operation line.
// When --layers is set, the layer path is prefixed; otherwise it's omitted.
func printFileLine(op, desc, label, layer string) {
	labelStr := ""
	if label != "" {
		labelStr = "  " + label
	}
	if inspectLayers && layer != "" {
		fmt.Printf("  %-21s %-7s %s%s\n", layer, op, desc, labelStr)
	} else {
		fmt.Printf("  %-7s %s%s\n", op, desc, labelStr)
	}
}

func printPermissions(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Permissions"))
	defer fmt.Println()

	if len(ctx.Ownerships) == 0 && len(ctx.Permissions) == 0 {
		fmt.Println("  (none)")
		return
	}

	for _, o := range ctx.Ownerships {
		extra := ""
		if o.Recursive {
			extra = " (recursive)"
		}
		label := ""
		if o.Label != "" {
			label = "  " + o.Label
		}
		if inspectLayers && o.Layer != "" {
			fmt.Printf("  %-21s %-30s chown %s:%s%s%s\n", o.Layer, o.Path, o.Owner, o.Group, extra, label)
		} else {
			fmt.Printf("  %-30s chown %s:%s%s%s\n", o.Path, o.Owner, o.Group, extra, label)
		}
	}

	for _, p := range ctx.Permissions {
		extra := ""
		if p.Recursive {
			extra = " (recursive)"
		}
		label := ""
		if p.Label != "" {
			label = "  " + p.Label
		}
		if inspectLayers && p.Layer != "" {
			fmt.Printf("  %-21s %-30s chmod %s%s%s\n", p.Layer, p.Path, p.Mode, extra, label)
		} else {
			fmt.Printf("  %-30s chmod %s%s%s\n", p.Path, p.Mode, extra, label)
		}
	}
}

func printBoot(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Boot"))
	defer fmt.Println()

	if ctx.Boot == nil {
		fmt.Println("  (not configured)")
		return
	}

	if inspectLayers && ctx.Boot.Layer != "" {
		fmt.Printf("  %s\n", inspectDim.Render(fmt.Sprintf("%s (active)", ctx.Boot.Layer)))
	}

	fmt.Printf("  loader: default=%s timeout=%d editor=%v\n",
		ctx.Boot.Loader.Default, ctx.Boot.Loader.Timeout, ctx.Boot.Loader.Editor)
	for _, e := range ctx.Boot.Entries {
		fmt.Printf("  entry:  %s\n", e.Name)
		fmt.Printf("    title:   %s\n", e.Title)
		fmt.Printf("    linux:   %s\n", e.Linux)
		fmt.Printf("    initrd:  %s\n", e.Initrd)
		fmt.Printf("    options: %s\n", e.Options)
	}
}

func printScripts(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Scripts"))
	defer fmt.Println()

	if len(ctx.Scripts) == 0 {
		fmt.Println("  (none)")
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
			fmt.Printf("  %-21s %s%s\n", s.Layer, desc, extra)
		} else {
			fmt.Printf("  %s%s\n", desc, extra)
		}
	}
}

func printInstaller(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Installer"))
	defer fmt.Println()

	if len(ctx.InstallerPayloads) == 0 && ctx.InstallerServer == nil && ctx.InstallerClient == nil {
		fmt.Println("  (not configured)")
		return
	}

	if len(ctx.InstallerPayloads) > 0 {
		fmt.Println("  payloads")
		for _, p := range ctx.InstallerPayloads {
			label := ""
			if p.Label != "" {
				label = " " + p.Label
			}
			if inspectLayers && p.Layer != "" {
				fmt.Printf("    %s  %s%s\n", p.Layer, p.Target, label)
			} else {
				fmt.Printf("    %s%s\n", p.Target, label)
			}
		}
	}

	if ctx.InstallerServer != nil {
		layerInfo := ""
		if inspectLayers && ctx.InstallerServer.Layer != "" {
			layerInfo = "  " + inspectDim.Render(ctx.InstallerServer.Layer)
		}
		fmt.Printf("  server  port: %d%s\n", ctx.InstallerServer.Port, layerInfo)
	}

	if ctx.InstallerClient != nil {
		layerInfo := ""
		if inspectLayers && ctx.InstallerClient.Layer != "" {
			layerInfo = "  " + inspectDim.Render(ctx.InstallerClient.Layer)
		}
		if ctx.InstallerClient.AutoLogin != "" {
			fmt.Printf("  client  auto_login: %s%s\n", ctx.InstallerClient.AutoLogin, layerInfo)
		} else {
			fmt.Printf("  client%s\n", layerInfo)
		}
	}
}
