package actions

import (
	"github.com/telemetryos/starforge/config"
)

type SystemLocale struct{}

func (a *SystemLocale) Name() string { return "system-locale" }

func (a *SystemLocale) Execute(step config.Step, layerDir string, ctx *BuildContext) error {
	s := step.SystemLocale

	if s.Locale != "" {
		ctx.LocaleHistory = append(ctx.LocaleHistory, LayerValue{
			Layer: ctx.CurrentLayer,
			Value: s.Locale,
		})
		ctx.Locale = s.Locale
	}

	if len(s.Locales) > 0 {
		ctx.Locales = append(ctx.Locales, s.Locales...)
	}

	return nil
}
