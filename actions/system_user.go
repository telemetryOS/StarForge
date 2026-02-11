package actions

import (
	"fmt"

	"github.com/telemetryos/starforge/config"
)

type SystemUser struct{}

func (a *SystemUser) Name() string { return "system-user" }

func (a *SystemUser) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemUser
	if s.Name == "" {
		return fmt.Errorf("system-user: name is required")
	}

	// Check if user already exists (merge semantics)
	for i, existing := range ctx.Users {
		if existing.Name == s.Name {
			// Merge groups based on mode
			switch s.Groups.Mode {
			case config.ModeAdd:
				ctx.Users[i].Groups = append(ctx.Users[i].Groups, s.Groups.Value...)
			case config.ModeRemove:
				remove := make(map[string]bool, len(s.Groups.Value))
				for _, g := range s.Groups.Value {
					remove[g] = true
				}
				var filtered []string
				for _, g := range ctx.Users[i].Groups {
					if !remove[g] {
						filtered = append(filtered, g)
					}
				}
				ctx.Users[i].Groups = filtered
			default: // ModeReplace
				if len(s.Groups.Value) > 0 {
					ctx.Users[i].Groups = s.Groups.Value
				}
			}

			// Override scalar fields if set
			if s.Shell != "" {
				ctx.Users[i].Shell = s.Shell
			}
			if s.Password != "" {
				ctx.Users[i].Password = s.Password
				ctx.Users[i].NoPassword = false
			}
			if s.NoPassword {
				ctx.Users[i].NoPassword = true
				ctx.Users[i].Password = ""
			}
			if s.UID != 0 {
				ctx.Users[i].UID = s.UID
			}

			return nil
		}
	}

	// New user
	ctx.Users = append(ctx.Users, UserDef{
		Name:       s.Name,
		Groups:     s.Groups.Value,
		Shell:      s.Shell,
		Password:   s.Password,
		NoPassword: s.NoPassword,
		System:     s.System,
		UID:        s.UID,
		Layer:      ctx.CurrentLayer,
	})
	return nil
}
