package commanddeck

import "testing"

func TestBuiltinCandidateReportIsNotReadyWithoutManualHIDValidation(t *testing.T) {
	report := BuiltinCandidateReport()
	if report.Ready {
		t.Fatalf("candidato builtin não deveria estar pronto sem validação real: %+v", report)
	}
	want := map[string]bool{
		"license-unverified":      true,
		"maintenance-unverified":  true,
		"physical-hid-unverified": true,
		"wails-build-unverified":  true,
		"windows-unverified":      true,
		"models-missing":          true,
	}
	for _, reason := range report.Reasons {
		delete(want, reason)
	}
	if len(want) != 0 {
		t.Fatalf("motivos esperados ausentes: %+v em %+v", want, report.Reasons)
	}
}

func TestValidateDriverRequiresModelAndPlatformEvidence(t *testing.T) {
	report := ValidateDriver(DriverValidation{
		Name:             "driver-ok",
		License:          "MIT",
		SupportsWindows:  true,
		SupportsWails:    true,
		Maintained:       true,
		ValidatedWithHID: true,
		Models: []Model{
			testModel,
		},
	})
	if !report.Ready || len(report.Reasons) != 0 {
		t.Fatalf("driver completo deveria ficar pronto: %+v", report)
	}
	report = ValidateDriver(DriverValidation{
		Name:             "driver-dup",
		License:          "MIT",
		SupportsWindows:  true,
		SupportsWails:    true,
		Maintained:       true,
		ValidatedWithHID: true,
		Models:           []Model{testModel, testModel},
	})
	if report.Ready {
		t.Fatalf("modelo duplicado não deveria ficar pronto")
	}
	found := false
	for _, reason := range report.Reasons {
		if reason == "model-duplicate" {
			found = true
		}
	}
	if !found {
		t.Fatalf("faltou model-duplicate: %+v", report.Reasons)
	}
}
