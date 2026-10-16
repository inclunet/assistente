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
	// Registro documental e testável da biblioteca candidata citada no AEP. A
	// biblioteca compila como dependência direta, é BSD-3-Clause e pure Go, mas
	// ValidatedWithHID fica falso até execução no Stream Deck físico do usuário.
	return ValidateDriver(DriverValidation{
		Name:             "rafaelmartins.com/p/streamdeck",
		License:          "BSD-3-Clause",
		Repository:       "rafaelmartins.com/p/streamdeck",
		SupportsWindows:  true,
		SupportsLinux:    true,
		SupportsMac:      true,
		SupportsWails:    true,
		Maintained:       true,
		RequiresCGO:      false,
		ValidatedWithHID: false,
		Models: []Model{
			{ID: "streamdeck-mini", Name: "Stream Deck Mini", Rows: 2, Columns: 3, KeyImageW: 80, KeyImageH: 80, SupportsHID: true},
			{ID: "streamdeck-v2", Name: "Stream Deck V2", Rows: 3, Columns: 5, KeyImageW: 72, KeyImageH: 72, SupportsHID: true},
			{ID: "streamdeck-mk2", Name: "Stream Deck MK.2", Rows: 3, Columns: 5, KeyImageW: 72, KeyImageH: 72, SupportsHID: true},
			{ID: "streamdeck-plus", Name: "Stream Deck Plus", Rows: 2, Columns: 4, KeyImageW: 120, KeyImageH: 120, SupportsHID: true},
			{ID: "streamdeck-neo", Name: "Stream Deck Neo", Rows: 2, Columns: 4, KeyImageW: 96, KeyImageH: 96, SupportsHID: true},
		},
		Notes: []string{"AEP ainda exige validação manual com HID físico para fechar I13.5."},
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
