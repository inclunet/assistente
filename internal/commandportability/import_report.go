package commandportability

import "sort"

// ImportReport resume o plano sem conteúdo de configuração ou identificadores
// de warnings. Os avisos descrevem o planejamento validado, não efeitos
// aplicados a Keep. O relatório não descreve habilitação ou conteúdo persistido.
type ImportReport struct {
	Version   int                  `json:"version"`
	NoChanges bool                 `json:"noChanges"`
	Layers    []ImportLayerReport  `json:"layers"`
	Warnings  []ImportWarningCount `json:"warnings"`
}

type ImportLayerReport struct {
	TargetID  string        `json:"targetId"`
	Scope     PortableScope `json:"scope"`
	Action    PlanMode      `json:"action"`
	DeltaOnly bool          `json:"deltaOnly"`
}

// ImportWarningCount agrega códigos do planejador, nunca patterns/Identifier.
type ImportWarningCount struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

func buildImportReport(plan Plan, noChanges bool) ImportReport {
	report := ImportReport{
		Version: plan.Version, NoChanges: noChanges,
		Layers:   make([]ImportLayerReport, 0, len(plan.Layers)),
		Warnings: make([]ImportWarningCount, 0),
	}
	for _, item := range plan.Layers {
		layer := ImportLayerReport{
			TargetID: item.TargetID, Scope: item.TargetScope, Action: item.Action,
			DeltaOnly: item.Layer.DeltaOnly,
		}
		report.Layers = append(report.Layers, layer)
	}
	counts := make(map[string]int)
	for _, warning := range plan.Warnings {
		counts[warning.Code]++
	}
	for code, count := range counts {
		report.Warnings = append(report.Warnings, ImportWarningCount{Code: code, Count: count})
	}
	sort.Slice(report.Warnings, func(i, j int) bool { return report.Warnings[i].Code < report.Warnings[j].Code })
	return report
}
