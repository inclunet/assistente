package questionnaire

import (
	"encoding/json"
	"strings"
)

// Text é um texto de diálogo que o backend manda para a tela sabendo que ela
// pode falar outro idioma. Carrega a chave de tradução e os valores que ela
// interpola, mais o texto pronto (Fallback) para quem não tem como traduzir.
//
// Quem exibe traduz a chave e usa o Fallback quando ela não existe naquele
// idioma. O Fallback nunca é decoração: é ele que aparece nas superfícies sem
// camada de tradução (canais, por exemplo, onde o diálogo vira mensagem) e é
// ele que impede o diálogo de chegar em branco a quem lê por leitor de telas
// se uma chave for esquecida em algum locale.
//
// O Fallback vai sempre pronto — já interpolado pelo backend —, e os Params
// existem para a tradução repetir a mesma interpolação no idioma dela.
type Text struct {
	// Key é a chave de tradução do frontend (ex.: "app.questionnaire.shell.title").
	// Vazia significa "este texto não se traduz": é conteúdo, não rótulo.
	Key string `json:"key,omitempty"`
	// Params são os valores que a tradução interpola. Use nomes próprios do
	// domínio; evite os nomes reservados do i18next (count, context, lng).
	Params map[string]any `json:"params,omitempty"`
	// Fallback é o texto pronto em pt-BR, usado quando não há tradução para a
	// chave — ou quando quem exibe não traduz nada.
	Fallback string `json:"fallback,omitempty"`
}

// Plain é o texto que não se traduz: conteúdo vindo de fora (do modelo, do
// agente, do disco) e textos ainda não migrados para chaves.
func Plain(text string) Text {
	return Text{Fallback: text}
}

// Keyed é o texto traduzível: a chave manda, e o texto atual em pt-BR fica
// como fallback.
func Keyed(key, fallback string) Text {
	return Text{Key: key, Fallback: fallback}
}

// KeyedWith é o texto traduzível que interpola valores. O fallback já deve vir
// interpolado: é ele que segue para as superfícies que não traduzem.
func KeyedWith(key string, params map[string]any, fallback string) Text {
	return Text{Key: key, Params: params, Fallback: fallback}
}

// String é o texto pronto para uma superfície que não traduz, e também o valor
// estável de uma opção — o que volta em Response.Answers quando a pessoa
// escolhe. Cai na chave quando não há fallback, porque devolver vazio deixaria
// o diálogo sem o rótulo que a pessoa acabou de escolher.
func (t Text) String() string {
	if t.Fallback != "" {
		return t.Fallback
	}
	return t.Key
}

// IsZero diz que não há texto nenhum, para o campo sair do JSON (omitzero).
func (t Text) IsZero() bool {
	return t.Key == "" && t.Fallback == "" && len(t.Params) == 0
}

// WithoutKey descarta a chave e mantém só o texto. Serve para o que vem de
// fora: chave de tradução é decisão do app, nunca de quem manda o conteúdo.
func (t Text) WithoutKey() Text {
	return Plain(t.String())
}

// textPayload espelha Text sem os métodos, para o marshal/unmarshal do objeto
// não recair em MarshalJSON/UnmarshalJSON.
type textPayload struct {
	Key      string         `json:"key,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
	Fallback string         `json:"fallback,omitempty"`
}

// MarshalJSON manda só a string quando não há nada a traduzir. Isso mantém o
// formato de antes para todo texto sem chave: quem lê o payload continua
// achando uma string onde sempre houve uma.
func (t Text) MarshalJSON() ([]byte, error) {
	if t.Key == "" && len(t.Params) == 0 {
		return json.Marshal(t.Fallback)
	}
	return json.Marshal(textPayload(t))
}

// UnmarshalJSON aceita as duas formas: a string (texto puro) e o objeto com
// chave. A string é o caso de quem manda o payload de fora — a tool
// collect_responses recebe as perguntas do modelo em JSON.
func (t *Text) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "null" {
		*t = Text{}
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*t = Plain(text)
		return nil
	}
	var payload textPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*t = Text(payload)
	return nil
}

// TextValues devolve os textos prontos de uma lista — os valores estáveis das
// opções, na ordem em que foram oferecidas.
func TextValues(texts []Text) []string {
	if texts == nil {
		return nil
	}
	out := make([]string, 0, len(texts))
	for _, text := range texts {
		out = append(out, text.String())
	}
	return out
}

// PlainTexts embrulha textos que não se traduzem (listas de opções vindas de
// fora, nomes de modelo, caminhos).
func PlainTexts(values []string) []Text {
	if values == nil {
		return nil
	}
	out := make([]Text, 0, len(values))
	for _, value := range values {
		out = append(out, Plain(value))
	}
	return out
}

// PlainQuestions tira as chaves de tradução de perguntas montadas fora do app.
// Texto que vem do modelo é conteúdo: aceitar chave dele deixaria o diálogo
// exibir o texto de outro lugar do app — ou nada, se a chave não existisse.
func PlainQuestions(questions []Question) []Question {
	for i := range questions {
		questions[i].Prompt = questions[i].Prompt.WithoutKey()
		questions[i].Description = questions[i].Description.WithoutKey()
		questions[i].Placeholder = questions[i].Placeholder.WithoutKey()
		for j := range questions[i].Options {
			questions[i].Options[j] = questions[i].Options[j].WithoutKey()
		}
	}
	return questions
}
