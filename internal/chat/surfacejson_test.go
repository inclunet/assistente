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
