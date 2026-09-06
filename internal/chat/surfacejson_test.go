package chat

import "testing"

func TestDecodeSurfaceJSONMap(t *testing.T) {
	t.Run("retorna nil para vazio e whitespace", func(t *testing.T) {
		if got := DecodeSurfaceJSONMap("   \n\t ", "[test] surface"); got != nil {
			t.Fatalf("esperava nil para whitespace, recebeu %#v", got)
		}
	})

	t.Run("retorna nil para json inválido", func(t *testing.T) {
		if got := DecodeSurfaceJSONMap("{", "[test] surface"); got != nil {
			t.Fatalf("esperava nil para json inválido, recebeu %#v", got)
		}
	})

	t.Run("retorna mapa para objeto válido", func(t *testing.T) {
		got := DecodeSurfaceJSONMap(`{"filePath":"a.txt","count":2}`, "[test] surface")
		if got == nil {
			t.Fatal("esperava mapa decodificado, recebeu nil")
		}
		if got["filePath"] != "a.txt" {
			t.Fatalf("filePath inesperado: %#v", got["filePath"])
		}
		if got["count"] != float64(2) {
			t.Fatalf("count inesperado: %#v", got["count"])
		}
	})
}

func TestDecodeCanonicalSurfaceContextJSON(t *testing.T) {
	t.Run("aceita envelope completo", func(t *testing.T) {
		got := DecodeCanonicalSurfaceContextJSON(
			`{"surfaceType":"editor","surfaceId":"tab-1","snapshotVersion":"editor:tab-1:1"}`,
			"[test] surface context",
		)
		if got == nil {
			t.Fatal("envelope canônico deveria ser aceito")
		}
	})

	for name, payload := range map[string]string{
		"campos legados":          `{"selectedText":"conteúdo"}`,
		"sem surfaceType":         `{"surfaceId":"tab-1","snapshotVersion":"editor:tab-1:1"}`,
		"sem surfaceId":           `{"surfaceType":"editor","snapshotVersion":"editor:tab-1:1"}`,
		"sem snapshotVersion":     `{"surfaceType":"editor","surfaceId":"tab-1"}`,
		"campo obrigatório vazio": `{"surfaceType":"editor","surfaceId":" ","snapshotVersion":"editor:tab-1:1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := DecodeCanonicalSurfaceContextJSON(payload, "[test] surface context"); got != nil {
				t.Fatalf("payload incompleto deveria ser descartado: %#v", got)
			}
		})
	}
}
