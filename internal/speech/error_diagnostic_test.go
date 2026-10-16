package speech

import "testing"

func TestTranslateBackendErrorForSpeech(t *testing.T) {
	tests := []struct {
		name, input, language, want string
	}{
		{
			name:     "provider diagnostic in Portuguese",
			input:    "Falha na resposta do agente: provider_error:v1:authentication::401",
			language: "pt-BR",
			want:     "Falha na resposta do agente: O provedor recusou a autenticação. Confira a credencial configurada. (HTTP 401)",
		},
		{
			name:     "provider diagnostic in Spanish",
			input:    "Falha na resposta do agente: provider_error:v1:unsupported_parameter:temperature:400",
			language: "es-ES",
			want:     "Falló la respuesta del agente: El proveedor no acepta el parámetro temperature para este modelo. Comprueba los parámetros configurados. (HTTP 400)",
		},
		{
			name:     "MCP diagnostic decodes server name",
			input:    "mcp_error:v1:Equipe%3A%20Produ%C3%A7%C3%A3o:call",
			language: "en-US",
			want:     "MCP server Equipe: Produção failed while running a tool. Try again or check its credentials and connection.",
		},
		{
			name:     "ordinary text remains unchanged",
			input:    "provider_error:v1:authentication:",
			language: "pt-BR",
			want:     "provider_error:v1:authentication:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := translateBackendErrorForSpeech(tt.input, tt.language); got != tt.want {
				t.Fatalf("translateBackendErrorForSpeech() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpeakMessageRecognizesOnlyPersistedDiagnostics(t *testing.T) {
	if !isBackendErrorDiagnostic("Falha na resposta do agente: provider_error:v1:authentication:") {
		t.Fatal("diagnóstico de provider deve ser reconhecido")
	}
	if !isBackendErrorDiagnostic("mcp_error:v1:server:list_tools") {
		t.Fatal("diagnóstico de MCP deve ser reconhecido")
	}
	for _, content := range []string{
		"provider_error:v1:authentication:",
		"Exemplo: Falha na resposta do agente: provider_error:v1:authentication:",
		"const diagnostic = \"provider_error:v1:authentication:\";",
	} {
		if isBackendErrorDiagnostic(content) {
			t.Errorf("conteúdo comum não deve ser tratado como diagnóstico: %q", content)
		}
	}
}
