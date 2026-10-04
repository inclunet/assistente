package database

import (
	"context"
	"math"
	"testing"
	"time"

	"assistente/internal/llmcapabilities"
)

func TestLLMModelIntegerBoundsAgreeWithSQLite(t *testing.T) {
	db := llmModelCapabilitiesTestDB(t)
	ctx := WithUserID(context.Background(), "owner")
	if err := db.Create(&LLMProvider{ID: "provider", UserID: "owner", Name: "Provider", Type: "custom", BaseURL: "https://example.com"}).Error; err != nil {
		t.Fatal(err)
	}
	repository := NewLLMModelCapabilitiesRepository(db)
	model, err := repository.SaveModel(ctx, "provider", "remote", "Remote")
	if err != nil {
		t.Fatal(err)
	}
	upper := math.Ldexp(1, 63)
	values := []struct {
		name  string
		value float64
		valid bool
	}{
		{"ordinary", 128000, true},
		{"largest-representable-below-upper", math.Nextafter(upper, 0), true},
		{"exclusive-upper", upper, false},
		{"above-upper", 1e20, false},
		{"inclusive-lower", -upper, true},
		{"below-lower", math.Nextafter(-upper, math.Inf(-1)), false},
		{"large-negative", -1e20, false},
		{"fractional", 0.5, false},
	}
	for _, column := range []string{"minimum", "maximum", "step"} {
		for _, value := range values {
			t.Run(column+"/"+value.name, func(t *testing.T) {
				claim := LLMModelCapabilityField{ModelID: model.ID, CapabilityKey: "chat", FieldKey: "max_output_tokens", SupportState: "supported", Source: "endpoint_discovery", Scope: "connection", ProviderCompatibilityRevision: 1, ObservedAt: time.Now().Add(-time.Second)}
				switch column {
				case "minimum":
					claim.Minimum = floatPtr(value.value)
				case "maximum":
					claim.Maximum = floatPtr(value.value)
				case "step":
					claim.Step = floatPtr(value.value)
				}
				wantValid := value.valid && (column != "step" || value.value > 0)
				if err := llmcapabilities.ValidateFieldAssertion(fieldAssertionFromModel(&claim, nil, false, 0)); (err == nil) != wantValid {
					t.Errorf("validação de domínio: value=%g valid=%v err=%v", value.value, wantValid, err)
				}
				if err := db.Create(&claim).Error; (err == nil) != wantValid {
					t.Errorf("barreira SQLite: value=%g valid=%v err=%v", value.value, wantValid, err)
				}
				claim.UUIDModel = UUIDModel{}
				if err := repository.RecordField(ctx, &claim, nil); (err == nil) != wantValid {
					t.Errorf("repositório: value=%g valid=%v err=%v", value.value, wantValid, err)
				}
			})
		}
	}
}
