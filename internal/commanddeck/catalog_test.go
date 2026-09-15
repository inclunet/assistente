package commanddeck

import "testing"

func TestBuiltinCandidateReportOnlyWaitsForManualHIDValidation(t *testing.T) {
	report := BuiltinCandidateReport()
	if report.Ready {
		t.Fatalf("candidato builtin não deveria estar pronto sem validação real: %+v", report)
	}
	if len(report.Reasons) != 1 || report.Reasons[0] != "physical-hid-unverified" {
		t.Fatalf("deveria restar somente validação física: %+v", report.Reasons)
	}
	if len(report.Models) < 5 {
		t.Fatalf("modelos suportados não foram registrados: %+v", report.Models)
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
