package jobs

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

// TestApplyPayloadTemplate_EmptyRenderIsSilent garante que um template que
// renderiza vazio (ex.: campos ausentes/condição falsa em jobs de intervalo)
// cai de volta no output original SEM emitir o WARN "payload_template JSON parse
// failed" — o ruído observado ~40x no assistente.log. Templates não-vazios com
// JSON inválido continuam avisando.
func TestApplyPayloadTemplate_EmptyRenderIsSilent(t *testing.T) {
	output := map[string]any{"foo": "bar"}
	trigCtx := &TriggerContext{EventPayload: map[string]any{}}
	e := &JobExecutor{}

	cases := []struct {
		name        string
		template    string
		wantResult  map[string]any
		wantWarnLog bool
	}{
		{
			name:        "render vazio não avisa",
			template:    "{{if false}}x{{end}}",
			wantResult:  output,
			wantWarnLog: false,
		},
		{
			name:        "render só com espaços não avisa",
			template:    "   ",
			wantResult:  output,
			wantWarnLog: false,
		},
		{
			name:        "JSON válido é aplicado",
			template:    `{"a":"b"}`,
			wantResult:  map[string]any{"a": "b"},
			wantWarnLog: false,
		},
		{
			name:        "JSON inválido não-vazio ainda avisa",
			template:    "not json",
			wantResult:  output,
			wantWarnLog: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
			defer slog.SetDefault(previous)

			job := &Job{Events: EventsConfig{PayloadTemplate: tc.template}}
			result := e.applyPayloadTemplate(context.Background(), job, output, trigCtx)

			if !reflect.DeepEqual(result, tc.wantResult) {
				t.Fatalf("resultado = %#v, quer %#v", result, tc.wantResult)
			}
			warned := strings.Contains(buf.String(), "payload_template JSON parse failed")
			if warned != tc.wantWarnLog {
				t.Fatalf("warn=%v, quer %v (log: %q)", warned, tc.wantWarnLog, buf.String())
			}
		})
	}
}
