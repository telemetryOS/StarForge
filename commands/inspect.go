package commands

import (
	"fmt"
	"sort"
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
	"files", "permissions", "boot", "system", "scripts",
}

var inspectCmd = &cobra.Command{
	Use:   "inspect <target> [concern]",
	Short: "Inspect the resolved build context for a target",
	Long: `Show the final resolved state after all layers are collected for a target.

If no concern is specified, shows a summary of everything. Use a specific
concern to focus on one aspect. Use --layers to see which layer contributed
each item.

Concerns: partitions, packages, groups, users, services, files,
          permissions, boot, system, scripts`,
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

	// Print all concerns
	for _, c := range validConcerns {
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
		printPartitions(ctx)
	case "packages":
		printPackages(ctx)
	case "groups":
		printGroups(ctx)
	case "users":
		printUsers(ctx)
	case "services":
		printServices(ctx)
	case "files":
		printFiles(ctx)
	case "permissions":
		printPermissions(ctx)
	case "boot":
		printBoot(ctx)
	case "scripts":
		printScripts(ctx)
	}
	return nil
}

func printSystem(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("System"))
	fmt.Println()

	printReplaceField("Hostname", ctx.Hostname, ctx.HostnameHistory)
	printReplaceField("Locale", ctx.Locale, ctx.LocaleHistory)
	printReplaceField("Timezone", ctx.Timezone, ctx.TimezoneHistory)
	printReplaceField("Keymap", ctx.Keymap, ctx.KeymapHistory)

	if len(ctx.Locales) > 0 {
		fmt.Printf("  %-12s %s\n", "Locales:", strings.Join(ctx.Locales, ", "))
	}
	fmt.Println()
}

func printReplaceField(label, value string, history []actions.LayerValue) {
	if value == "" && len(history) == 0 {
		return
	}

	if !inspectLayers || len(history) <= 1 {
		fmt.Printf("  %-12s %s\n", label+":", value)
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
	fmt.Println()

	if len(ctx.Partitions) == 0 {
		fmt.Println("  (none)")
		fmt.Println()
		return
	}

	// Column headers
	fmt.Printf("  %-12s %-10s %-10s %-12s %s\n",
		"Name", "FS", "Size", "Mount", "Type")
	fmt.Printf("  %-12s %-10s %-10s %-12s %s\n",
		"----", "--", "----", "-----", "----")

	for _, p := range ctx.Partitions {
		sizeStr := actions.FormatSize(p.Size)
		if p.Grow {
			sizeStr += " (grow)"
		}
		pType := p.Type
		if pType == "" {
			pType = "-"
		}
		fmt.Printf("  %-12s %-10s %-10s %-12s %s\n",
			p.Name, p.Filesystem, sizeStr, p.MountPoint, pType)
	}

	if inspectLayers && len(ctx.PartitionHistory) > 0 {
		fmt.Println()
		fmt.Println(inspectDim.Render("  Layer history:"))
		for _, snap := range ctx.PartitionHistory {
			names := make([]string, len(snap.Partitions))
			for i, p := range snap.Partitions {
				names[i] = p.Name
			}
			fmt.Printf("    %s → [%s]\n",
				inspectDim.Render(snap.Layer),
				strings.Join(names, ", "))
		}
	}
	fmt.Println()
}

func printPackages(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Packages"))
	fmt.Println()

	if len(ctx.Packages) == 0 {
		fmt.Println("  (none)")
		fmt.Println()
		return
	}

	if inspectLayers && len(ctx.PackageGroups) > 0 {
		for _, g := range ctx.PackageGroups {
			for _, pkg := range g.Items {
				fmt.Printf("  %s  %s\n", pkg, inspectDim.Render(fmt.Sprintf("← %s", g.Layer)))
			}
		}
	} else {
		// Deduplicate and sort
		seen := make(map[string]bool)
		var unique []string
		for _, pkg := range ctx.Packages {
			if !seen[pkg] {
				seen[pkg] = true
				unique = append(unique, pkg)
			}
		}
		sort.Strings(unique)
		for _, pkg := range unique {
			fmt.Printf("  %s\n", pkg)
		}
	}
	fmt.Println()
}

func printGroups(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Groups"))
	fmt.Println()

	if len(ctx.Groups) == 0 {
		fmt.Println("  (none)")
		fmt.Println()
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
			layerInfo = "  " + inspectDim.Render(fmt.Sprintf("← %s", g.Layer))
		}
		fmt.Printf("  %s%s%s\n", g.Name, extra, layerInfo)
	}
	fmt.Println()
}

func printUsers(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Users"))
	fmt.Println()

	if len(ctx.Users) == 0 {
		fmt.Println("  (none)")
		fmt.Println()
		return
	}

	for _, u := range ctx.Users {
		parts := []string{u.Name}
		if u.Shell != "" {
			parts = append(parts, fmt.Sprintf("shell=%s", u.Shell))
		}
		if len(u.Groups) > 0 {
			parts = append(parts, fmt.Sprintf("groups=[%s]", strings.Join(u.Groups, ",")))
		}
		if u.System {
			parts = append(parts, "(system)")
		}
		if u.NoPassword {
			parts = append(parts, "no-password")
		}
		layerInfo := ""
		if inspectLayers && u.Layer != "" {
			layerInfo = "  " + inspectDim.Render(fmt.Sprintf("← %s", u.Layer))
		}
		fmt.Printf("  %s%s\n", strings.Join(parts, "  "), layerInfo)
	}
	fmt.Println()
}

func printServices(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Services"))
	fmt.Println()

	if ctx.DefaultTarget != "" {
		if inspectLayers && len(ctx.DefaultTargetHistory) > 0 {
			printReplaceField("Default", ctx.DefaultTarget, ctx.DefaultTargetHistory)
		} else {
			fmt.Printf("  Default target: %s\n", ctx.DefaultTarget)
		}
		fmt.Println()
	}

	printServiceList("Enable", ctx.Services.Enable, ctx.EnableGroups)
	printServiceList("Disable", ctx.Services.Disable, ctx.DisableGroups)
	printServiceList("Mask", ctx.Services.Mask, ctx.MaskGroups)

	if len(ctx.Services.UserEnable) > 0 {
		fmt.Println("  User enable:")
		for _, op := range ctx.Services.UserEnable {
			fmt.Printf("    %s (user: %s)\n", op.Service, op.User)
		}
	}
	if len(ctx.Services.UserDisable) > 0 {
		fmt.Println("  User disable:")
		for _, op := range ctx.Services.UserDisable {
			fmt.Printf("    %s (user: %s)\n", op.Service, op.User)
		}
	}
	fmt.Println()
}

func printServiceList(label string, items []string, groups []actions.LayerGroup) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("  %s:\n", label)
	if inspectLayers && len(groups) > 0 {
		for _, g := range groups {
			for _, svc := range g.Items {
				fmt.Printf("    %s  %s\n", svc, inspectDim.Render(fmt.Sprintf("← %s", g.Layer)))
			}
		}
	} else {
		for _, svc := range items {
			fmt.Printf("    %s\n", svc)
		}
	}
}

func printFiles(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Files"))
	fmt.Println()

	empty := len(ctx.Mkdirs) == 0 && len(ctx.FileCreates) == 0 &&
		len(ctx.FileEdits) == 0 && len(ctx.Copies) == 0 &&
		len(ctx.Moves) == 0 && len(ctx.Links) == 0 &&
		len(ctx.Removes) == 0 && len(ctx.InternalCopies) == 0

	if empty {
		fmt.Println("  (none)")
		fmt.Println()
		return
	}

	printFileOps("mkdir", ctx.Mkdirs, func(op actions.MkdirOp) string {
		extra := ""
		if op.Mode != "" {
			extra += fmt.Sprintf(" mode=%s", op.Mode)
		}
		if op.Owner != "" {
			extra += fmt.Sprintf(" owner=%s", op.Owner)
		}
		return op.Path + extra
	}, func(op actions.MkdirOp) string { return op.Layer })

	printFileOps("create", ctx.FileCreates, func(op actions.FileCreateOp) string {
		extra := ""
		if op.Mode != "" {
			extra += fmt.Sprintf(" mode=%s", op.Mode)
		}
		return op.Path + extra
	}, func(op actions.FileCreateOp) string { return op.Layer })

	printFileOps("edit", ctx.FileEdits, func(op actions.FileEditOp) string {
		extra := ""
		if op.Insert != "" {
			extra += fmt.Sprintf(" insert=%s", op.Insert)
		}
		if op.Pattern != "" {
			extra += fmt.Sprintf(" pattern=%q", op.Pattern)
		}
		return op.Path + extra
	}, func(op actions.FileEditOp) string { return op.Layer })

	printFileOps("copy", ctx.Copies, func(op actions.CopyOp) string {
		return fmt.Sprintf("%s → %s", op.FromPath, op.ToPath)
	}, func(op actions.CopyOp) string { return op.Layer })

	printFileOps("icopy", ctx.InternalCopies, func(op actions.InternalCopyOp) string {
		return fmt.Sprintf("%s → %s", op.FromPath, op.ToPath)
	}, func(op actions.InternalCopyOp) string { return op.Layer })

	printFileOps("move", ctx.Moves, func(op actions.MoveOp) string {
		return fmt.Sprintf("%s → %s", op.FromPath, op.ToPath)
	}, func(op actions.MoveOp) string { return op.Layer })

	printFileOps("link", ctx.Links, func(op actions.LinkOp) string {
		return fmt.Sprintf("%s → %s (%s)", op.ToPath, op.FromPath, op.Type)
	}, func(op actions.LinkOp) string { return op.Layer })

	printFileOps("delete", ctx.Removes, func(op actions.RemoveOp) string {
		extra := ""
		if op.Recursive {
			extra = " (recursive)"
		}
		return op.Path + extra
	}, func(op actions.RemoveOp) string { return op.Layer })

	fmt.Println()
}

func printFileOps[T any](label string, ops []T, format func(T) string, getLayer func(T) string) {
	if len(ops) == 0 {
		return
	}
	for _, op := range ops {
		layerInfo := ""
		if inspectLayers {
			layer := getLayer(op)
			if layer != "" {
				layerInfo = "  " + inspectDim.Render(fmt.Sprintf("← %s", layer))
			}
		}
		fmt.Printf("  %-8s %s%s\n", label, format(op), layerInfo)
	}
}

func printPermissions(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Permissions"))
	fmt.Println()

	if len(ctx.Ownerships) == 0 && len(ctx.Permissions) == 0 {
		fmt.Println("  (none)")
		fmt.Println()
		return
	}

	for _, o := range ctx.Ownerships {
		extra := ""
		if o.Recursive {
			extra = " (recursive)"
		}
		layerInfo := ""
		if inspectLayers && o.Layer != "" {
			layerInfo = "  " + inspectDim.Render(fmt.Sprintf("← %s", o.Layer))
		}
		fmt.Printf("  chown    %s:%s %s%s%s\n", o.Owner, o.Group, o.Path, extra, layerInfo)
	}

	for _, p := range ctx.Permissions {
		extra := ""
		if p.Recursive {
			extra = " (recursive)"
		}
		layerInfo := ""
		if inspectLayers && p.Layer != "" {
			layerInfo = "  " + inspectDim.Render(fmt.Sprintf("← %s", p.Layer))
		}
		fmt.Printf("  chmod    %s %s%s%s\n", p.Mode, p.Path, extra, layerInfo)
	}
	fmt.Println()
}

func printBoot(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Boot"))
	fmt.Println()

	if ctx.Boot == nil {
		fmt.Println("  (not configured)")
		fmt.Println()
		return
	}

	fmt.Printf("  Loader:\n")
	fmt.Printf("    Default: %s\n", ctx.Boot.Loader.Default)
	fmt.Printf("    Timeout: %d\n", ctx.Boot.Loader.Timeout)
	fmt.Printf("    Editor:  %v\n", ctx.Boot.Loader.Editor)
	fmt.Println()

	if len(ctx.Boot.Entries) > 0 {
		fmt.Printf("  Entries:\n")
		for _, e := range ctx.Boot.Entries {
			fmt.Printf("    %s:\n", e.Name)
			fmt.Printf("      Title:   %s\n", e.Title)
			fmt.Printf("      Linux:   %s\n", e.Linux)
			fmt.Printf("      Initrd:  %s\n", e.Initrd)
			fmt.Printf("      Options: %s\n", e.Options)
		}
	}
	fmt.Println()
}

func printScripts(ctx *actions.BuildContext) {
	fmt.Println(inspectHeader.Render("Scripts"))
	fmt.Println()

	if len(ctx.Scripts) == 0 {
		fmt.Println("  (none)")
		fmt.Println()
		return
	}

	for i, s := range ctx.Scripts {
		label := fmt.Sprintf("[%d]", i+1)
		if s.Label != "" {
			label = s.Label
		}

		desc := s.Script
		if desc == "" && s.Content != "" {
			// Show first line of inline content
			lines := strings.SplitN(s.Content, "\n", 2)
			desc = "(inline) " + strings.TrimSpace(lines[0])
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
		}

		extra := ""
		if s.User != "" {
			extra = fmt.Sprintf(" (user: %s)", s.User)
		}

		layerInfo := ""
		if inspectLayers && s.Layer != "" {
			layerInfo = "  " + inspectDim.Render(fmt.Sprintf("← %s", s.Layer))
		}
		fmt.Printf("  %-20s %s%s%s\n", label, desc, extra, layerInfo)
	}
	fmt.Println()
}
