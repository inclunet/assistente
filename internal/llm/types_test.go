package llm

import "testing"

func TestConteudoEmPartesTipadasDevolveOTexto(t *testing.T) {
	// O builder monta a mensagem do turno com partes tipadas quando ela é
	// multimodal. Sem tratar esse formato, o texto sairia como despejo da
	// estrutura ("[{text ...}]") para quem lê o conteúdo da mensagem.
	msg := Message{Role: "user", Content: []ContentPart{
		{Type: "text", Text: "<turn_context>"},
		{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,abc"}},
		{Type: "text", Text: "o que tem nesta imagem?"},
	}}

	if got, want := msg.GetContentAsString(), "<turn_context>\no que tem nesta imagem?"; got != want {
		t.Errorf("GetContentAsString() = %q, quer %q", got, want)
	}
}

func TestConteudoEmPartesDestipadasDevolveOTexto(t *testing.T) {
	msg := Message{Role: "user", Content: []interface{}{
		map[string]interface{}{"type": "text", "text": "primeira"},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,abc"}},
		map[string]interface{}{"type": "text", "text": "segunda"},
	}}

	if got, want := msg.GetContentAsString(), "primeira\nsegunda"; got != want {
		t.Errorf("GetContentAsString() = %q, quer %q", got, want)
	}
}
