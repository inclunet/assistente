package acpregistry

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestUmDocumentoBomViraCatalogoTipado(t *testing.T) {
	index, err := ParseIndex(context.Background(), []byte(indiceBom))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}
	if index.Version != "1.0.0" {
		t.Errorf("versão = %q, quer %q", index.Version, "1.0.0")
	}
	if len(index.Agents) != 3 {
		t.Fatalf("agentes = %d, quer 3", len(index.Agents))
	}

	codex := agentePorID(t, index.Agents, "codex-acp")
	if codex.Distribution.NPX == nil {
		t.Fatal("codex-acp deveria ter distribuição npx")
	}
	if got := codex.Distribution.NPX.Package; got != "@agentclientprotocol/codex-acp@1.1.9" {
		t.Errorf("pacote npx = %q", got)
	}
	if got := codex.Distribution.NPX.Args; len(got) != 1 || got[0] != "--acp" {
		t.Errorf("args npx = %v, quer [--acp]", got)
	}
	if got := codex.Authors; len(got) != 2 || got[0] != "OpenAI" {
		t.Errorf("autores = %v", got)
	}

	goose := agentePorID(t, index.Agents, "goose")
	alvo, ok := goose.Distribution.Binary["windows-x86_64"]
	if !ok {
		t.Fatalf("goose deveria ter alvo windows-x86_64, tem %v", goose.Distribution.Binary)
	}
	// O formato aceita digest em maiúsculas; o saneamento normaliza para
	// minúsculas para a conferência da Fase 4 comparar sem surpresa.
	if alvo.SHA256 != strings.ToLower(digestDeTeste) {
		t.Errorf("sha256 = %q, quer %q", alvo.SHA256, strings.ToLower(digestDeTeste))
	}
	if alvo.Cmd != `./goose-package\goose.exe` {
		t.Errorf("cmd = %q", alvo.Cmd)
	}
	if got := alvo.Env["GOOSE_ACP"]; got != "1" {
		t.Errorf("env GOOSE_ACP = %q, quer %q", got, "1")
	}

	fast := agentePorID(t, index.Agents, "fast-agent")
	if fast.Distribution.UVX == nil || fast.Distribution.UVX.Package != "fast-agent-mcp" {
		t.Errorf("distribuição uvx = %+v", fast.Distribution.UVX)
	}
}

func TestODocumentoDeMajorDesconhecidoERecusado(t *testing.T) {
	casos := []string{"2.0.0", "0.9.1", "10.0.0"}
	for _, versao := range casos {
		t.Run(versao, func(t *testing.T) {
			doc := strings.Replace(indiceBom, `"version": "1.0.0"`, `"version": "`+versao+`"`, 1)
			_, err := ParseIndex(context.Background(), []byte(doc))
			if !errors.Is(err, ErrUnsupportedVersion) {
				t.Fatalf("erro = %v, quer ErrUnsupportedVersion", err)
			}
		})
	}
}

func TestAVersaoIlegivelEhRecusadaComoMalformada(t *testing.T) {
	casos := map[string]string{
		"ausente":     `{"agents":[]}`,
		"vazia":       `{"version":"","agents":[]}`,
		"texto":       `{"version":"latest","agents":[]}`,
		"sem número":  `{"version":".1.0","agents":[]}`,
		"não é json":  `nem json é`,
		"agents mapa": `{"version":"1.0.0","agents":{}}`,
		"lixo no fim": `{"version":"1.0.0","agents":[]} sobrou`,
	}
	for nome, doc := range casos {
		t.Run(nome, func(t *testing.T) {
			_, err := ParseIndex(context.Background(), []byte(doc))
			if !errors.Is(err, ErrMalformedIndex) {
				t.Fatalf("erro = %v, quer ErrMalformedIndex", err)
			}
		})
	}
}

func TestODocumentoSemAgenteAproveitavelERecusado(t *testing.T) {
	casos := map[string]string{
		"lista vazia":            `{"version":"1.0.0","agents":[]}`,
		"entrada sem id":         `{"version":"1.0.0","agents":[{"name":"Sem id","distribution":{"npx":{"package":"p"}}}]}`,
		"entrada sem formato":    `{"version":"1.0.0","agents":[{"id":"a","name":"A","distribution":{}}]}`,
		"entrada de tipo errado": `{"version":"1.0.0","agents":["texto"]}`,
	}
	for nome, doc := range casos {
		t.Run(nome, func(t *testing.T) {
			_, err := ParseIndex(context.Background(), []byte(doc))
			if !errors.Is(err, ErrMalformedIndex) {
				t.Fatalf("erro = %v, quer ErrMalformedIndex", err)
			}
		})
	}
}

// Uma entrada podre não pode custar o catálogo inteiro: o registro é curado por
// PR de terceiros, e um vizinho com campo de tipo errado não diz nada sobre os
// outros 37 agentes.
func TestAEntradaPodreEhDescartadaSemLevarAsVizinhas(t *testing.T) {
	doc := `{"version":"1.0.0","agents":[
		{"id":"bom","name":"Bom","distribution":{"npx":{"package":"bom"}}},
		{"id":"podre","name":"Podre","authors":"texto onde deveria ser lista","distribution":{"npx":{"package":"podre"}}},
		{"id":"../fuga","name":"Fuga","distribution":{"npx":{"package":"fuga"}}},
		{"id":"sem-formato","name":"Sem formato","distribution":{}}
	]}`
	index, err := ParseIndex(context.Background(), []byte(doc))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}
	if len(index.Agents) != 1 || index.Agents[0].ID != "bom" {
		t.Fatalf("agentes = %+v, quer só o 'bom'", index.Agents)
	}
}

func TestOAgenteRepetidoEntraUmaVezSo(t *testing.T) {
	doc := `{"version":"1.0.0","agents":[
		{"id":"dup","name":"Primeiro","distribution":{"npx":{"package":"a"}}},
		{"id":"dup","name":"Segundo","distribution":{"npx":{"package":"b"}}}
	]}`
	index, err := ParseIndex(context.Background(), []byte(doc))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}
	if len(index.Agents) != 1 || index.Agents[0].Name != "Primeiro" {
		t.Fatalf("agentes = %+v, quer só o primeiro", index.Agents)
	}
}

// O texto do catálogo vai direto para a tela e para o leitor de telas (D9), e
// quem preenche a descrição é quem submeteu o PR ao registro.
func TestOTextoMaliciosoDoDocumentoEhSaneado(t *testing.T) {
	index, err := ParseIndex(context.Background(), []byte(indiceComTextoMalicioso))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}
	agente := agentePorID(t, index.Agents, "sujo")

	if agente.Name != "Agente" {
		t.Errorf("nome = %q, quer %q (sem escape de terminal)", agente.Name, "Agente")
	}
	if agente.Version != "1.0.0" {
		t.Errorf("versão = %q, quer %q (sem caractere de controle)", agente.Version, "1.0.0")
	}
	if strings.ContainsAny(agente.Description, "\n\r\x1b\u202e") {
		t.Errorf("descrição = %q, ainda carrega controle ou marca invisível", agente.Description)
	}
	if agente.License != "MIT" {
		t.Errorf("licença = %q, quer %q", agente.License, "MIT")
	}
	if got := agente.Authors; len(got) != 1 || got[0] != "Autor" {
		t.Errorf("autores = %v, quer [Autor]", got)
	}
	if agente.Website != "" {
		t.Errorf("website = %q, quer vazio (não era https)", agente.Website)
	}
	if agente.Icon != "" {
		t.Errorf("ícone = %q, quer vazio (não era https)", agente.Icon)
	}
	if agente.Repository != "https://exemplo.test/repo" {
		t.Errorf("repositório = %q, quer o https intacto", agente.Repository)
	}
	if got := agente.Distribution.NPX.Args; len(got) != 1 || got[0] != "--acp" {
		t.Errorf("args = %v, quer [--acp] sem escape", got)
	}
}

// O `id` vira nome de diretório em ~/.assistente/agents/<id>/ (D5). Recusar na
// fronteira é mais barato do que confiar em quem monta o caminho depois.
func TestOIdentificadorQueEscapariaDoDiretorioEhRecusado(t *testing.T) {
	casos := map[string]string{
		"subida de diretório": "../fuga",
		"dois pontos no meio": "a..b",
		"barra":               "escopo/agente",
		"barra invertida":     `escopo\agente`,
		"espaço":              "com espaço",
		"começa com ponto":    ".oculto",
		"vazio":               "",
		"só espaços":          "   ",
		"nulo":                "a\x00b",
		"comprido":            strings.Repeat("a", maxIdentLen+1),
	}
	for nome, id := range casos {
		t.Run(nome, func(t *testing.T) {
			if got := sanitizeIdent(id); got != "" {
				t.Errorf("sanitizeIdent(%q) = %q, quer vazio", id, got)
			}
		})
	}
	for _, id := range []string{"codex-acp", "claude_code", "agente.v2", "x86_64", "windows-x86_64"} {
		if got := sanitizeIdent(id); got != id {
			t.Errorf("sanitizeIdent(%q) = %q, quer o próprio", id, got)
		}
	}
}

func TestSoSobreviveURLAbsolutaEmHTTPS(t *testing.T) {
	recusadas := map[string]string{
		"http":            "http://exemplo.test/x",
		"javascript":      "javascript:alert(1)",
		"dados":           "data:text/html,<script>alert(1)</script>",
		"arquivo":         "file:///c:/windows",
		"relativa":        "/apenas/caminho",
		"sem host":        "https:///caminho",
		"com espaço":      "https://exemplo.test/a b",
		"com controle":    "https://exemplo.test/\x07",
		"marca invisível": "https://exemplo.test/\u202egnp.exe",
		"vazia":           "",
		"comprida demais": "https://exemplo.test/" + strings.Repeat("a", maxURLLen),
	}
	for nome, raw := range recusadas {
		t.Run(nome, func(t *testing.T) {
			if got := sanitizeHTTPSURL(raw); got != "" {
				t.Errorf("sanitizeHTTPSURL(%q) = %q, quer vazio", raw, got)
			}
		})
	}
	aceita := "https://github.com/agentclientprotocol/registry"
	if got := sanitizeHTTPSURL(aceita); got != aceita {
		t.Errorf("sanitizeHTTPSURL(%q) = %q, quer a própria", aceita, got)
	}
}

// Digest que não é SHA-256 é descartado, e não corrigido: o alvo cai no mesmo
// conjunto de quem não publica digest, que é o lado seguro do D4.
func TestODigestQueNaoESha256EhDescartado(t *testing.T) {
	casos := map[string]string{
		"curto":       "abc123",
		"não é hex":   strings.Repeat("z", 64),
		"com prefixo": "sha256:" + strings.Repeat("a", 64),
		"comprido":    strings.Repeat("a", 65),
		"vazio":       "",
		"com espaço":  strings.Repeat("a", 63) + " ",
	}
	for nome, digest := range casos {
		t.Run(nome, func(t *testing.T) {
			if got := sanitizeDigest(digest); got != "" {
				t.Errorf("sanitizeDigest(%q) = %q, quer vazio", digest, got)
			}
		})
	}
	if got := sanitizeDigest("  " + digestDeTeste + "  "); got != strings.ToLower(digestDeTeste) {
		t.Errorf("sanitizeDigest do digest bom = %q", got)
	}
}

// O nome do pacote vira argumento de `npm install` na Fase 3.
func TestOPacoteComMetacaractereEhRecusado(t *testing.T) {
	casos := map[string]string{
		"ponto e vírgula": "pacote; rm -rf /",
		"cifrão":          "pacote$(whoami)",
		"crase":           "pacote`id`",
		"espaço":          "dois pacotes",
		"subida":          "../pacote",
		"vazio":           "",
	}
	for nome, pacote := range casos {
		t.Run(nome, func(t *testing.T) {
			if got := sanitizePackage(&PackageDistribution{Package: pacote}); got != nil {
				t.Errorf("sanitizePackage(%q) = %+v, quer nil", pacote, got)
			}
		})
	}
	for _, pacote := range []string{"@agentclientprotocol/codex-acp@1.1.9", "fast-agent-mcp", "@github/copilot@1.0.78"} {
		got := sanitizePackage(&PackageDistribution{Package: pacote})
		if got == nil || got.Package != pacote {
			t.Errorf("sanitizePackage(%q) = %+v, quer o próprio", pacote, got)
		}
	}
}

// Alvo sem archive https ou sem cmd não descreve nada que a Fase 4 possa fazer.
func TestOAlvoBinarioIncompletoEhDescartado(t *testing.T) {
	doc := `{"version":"1.0.0","agents":[{"id":"meio","name":"Meio","distribution":{"binary":{
		"linux-x86_64":   {"archive":"https://exemplo.test/a.tar.gz","cmd":"./a"},
		"linux-aarch64":  {"archive":"https://exemplo.test/b.tar.gz"},
		"darwin-x86_64":  {"cmd":"./c"},
		"windows-x86_64": {"archive":"http://exemplo.test/d.zip","cmd":"./d.exe"},
		"../fuga":        {"archive":"https://exemplo.test/e.zip","cmd":"./e.exe"}
	}}}]}`
	index, err := ParseIndex(context.Background(), []byte(doc))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}
	alvos := index.Agents[0].Distribution.Binary
	if len(alvos) != 1 {
		t.Fatalf("alvos = %v, quer só linux-x86_64", alvos)
	}
	if _, ok := alvos["linux-x86_64"]; !ok {
		t.Errorf("alvos = %v, quer linux-x86_64", alvos)
	}
}

func TestONomeVazioCaiNoIdentificador(t *testing.T) {
	doc := `{"version":"1.0.0","agents":[{"id":"anonimo","name":"\u001b[0m","distribution":{"npx":{"package":"p"}}}]}`
	index, err := ParseIndex(context.Background(), []byte(doc))
	if err != nil {
		t.Fatalf("ParseIndex devolveu erro: %v", err)
	}
	if got := index.Agents[0].Name; got != "anonimo" {
		t.Errorf("nome = %q, quer o identificador", got)
	}
}

func agentePorID(t *testing.T, agents []Agent, id string) Agent {
	t.Helper()
	for _, agent := range agents {
		if agent.ID == id {
			return agent
		}
	}
	t.Fatalf("agente %q não está no catálogo", id)
	return Agent{}
}
