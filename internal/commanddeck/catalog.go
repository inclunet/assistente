package commanddeck

import "sort"

type DriverValidation struct {
	Name             string
	License          string
	Repository       string
	SupportsWindows  bool
	SupportsLinux    bool
	SupportsMac      bool
	SupportsWails    bool
	Maintained       bool
	Models           []Model
	RequiresCGO      bool
	Notes            []string
	ValidatedWithHID bool
}

type ValidationReport struct {
	Ready   bool
	Reasons []string
	Models  []Model
}

func BuiltinCandidateReport() ValidationReport {
	// Registro documental e testável da biblioteca candidata citada no AEP.
	// Não é aceite físico: ValidatedWithHID fica falso até execução real.
	return ValidateDriver(DriverValidation{
		Name:             "rafaelmartins.com/p/streamdeck",
		License:          "unknown-unverified",
		Repository:       "rafaelmartins.com/p/streamdeck",
		SupportsWindows:  false,
		SupportsLinux:    false,
		SupportsMac:      false,
		SupportsWails:    false,
		Maintained:       false,
		RequiresCGO:      true,
		ValidatedWithHID: false,
		Models:           nil,
		Notes:            []string{"AEP exige validação manual de licença, manutenção, modelos, reconexão, distribuição e build Wails antes de tornar dependência."},
	})
}

func ValidateDriver(value DriverValidation) ValidationReport {
	report := ValidationReport{Models: cloneModels(value.Models)}
	if !validText(value.Name) {
		report.Reasons = append(report.Reasons, "driver-name-missing")
	}
	if !validText(value.License) || value.License == "unknown-unverified" {
		report.Reasons = append(report.Reasons, "license-unverified")
	}
	if !value.Maintained {
		report.Reasons = append(report.Reasons, "maintenance-unverified")
	}
	if !value.SupportsWindows {
		report.Reasons = append(report.Reasons, "windows-unverified")
	}
	if !value.SupportsWails {
		report.Reasons = append(report.Reasons, "wails-build-unverified")
	}
	if !value.ValidatedWithHID {
		report.Reasons = append(report.Reasons, "physical-hid-unverified")
	}
	if len(value.Models) == 0 {
		report.Reasons = append(report.Reasons, "models-missing")
	}
	seen := map[string]struct{}{}
	for _, model := range value.Models {
		if err := model.validate(); err != nil {
			report.Reasons = append(report.Reasons, "model-invalid")
			continue
		}
		if _, ok := seen[model.ID]; ok {
			report.Reasons = append(report.Reasons, "model-duplicate")
			continue
		}
		seen[model.ID] = struct{}{}
		if !model.SupportsHID {
			report.Reasons = append(report.Reasons, "model-hid-unverified")
		}
	}
	report.Reasons = uniqueSorted(report.Reasons)
	report.Ready = len(report.Reasons) == 0
	return report
}

func cloneModels(models []Model) []Model {
	if len(models) == 0 {
		return nil
	}
	clone := append([]Model(nil), models...)
	sort.Slice(clone, func(i, j int) bool { return clone[i].ID < clone[j].ID })
	return clone
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
