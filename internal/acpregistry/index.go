// Package acpregistry lê o índice do registro oficial do Agent Client Protocol
// — o catálogo de agentes que este app passa a usar em vez de detecção escrita
// à mão por agente (AEP-0086 D1).
//
// O índice é JSON de terceiro, e é tratado como tal (D9): todo texto que pode
// chegar à tela ou a um anúncio passa pelo saneamento de rótulo do
// `internal/acp` antes de ser guardado, e nada que vem do documento é
// executado aqui — esta fase só lê, valida e guarda.
//
// O que o pacote entrega é um catálogo que abre sem rede (D2): o índice fica
// cacheado em disco com o carimbo da coleta, o cache é servido na hora e a
// revalidação acontece em segundo plano.
package acpregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"assistente/internal/acp"
	"assistente/internal/logging"
)

// component identifica o pacote nos logs estruturados.
const component = "acpregistry.index"

// supportedMajor é o major do documento que este código sabe ler. O que o app
// olha em `version` é o major, e não o valor exato (D2): mudança de patch ou de
// minor acrescenta campo, mudança de major troca contrato.
const supportedMajor = 1

var (
	// ErrMalformedIndex diz que o documento não é um índice aproveitável. Quem
	// recebe este erro mantém o que já tinha: um índice quebrado não vale o
	// catálogo que estava funcionando (D2).
	ErrMalformedIndex = errors.New("índice do registro ACP malformado")

	// ErrUnsupportedVersion diz que o documento declara um major que este app
	// não conhece. Ler um formato que trocou de contrato e adivinhar o resto
	// acabaria em instalar a coisa errada, então a resposta é recusar.
	ErrUnsupportedVersion = errors.New("versão do índice do registro ACP não suportada")
)

// Index é o documento do registro depois de validado e saneado. É também o que
// vai para o cache em disco, e por isso tem as tags do formato publicado: o
// arquivo gravado volta pela mesma porta de validação na leitura.
type Index struct {
	Version string  `json:"version"`
	Agents  []Agent `json:"agents"`
}

// Agent é uma linha do catálogo. Os campos de texto já estão saneados; as URLs
// já foram recusadas se não fossem `https` absolutas.
type Agent struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description,omitempty"`
	Repository   string       `json:"repository,omitempty"`
	Website      string       `json:"website,omitempty"`
	Authors      []string     `json:"authors,omitempty"`
	License      string       `json:"license,omitempty"`
	Icon         string       `json:"icon,omitempty"`
	Distribution Distribution `json:"distribution"`
}

// Distribution são as formas de obter o agente. O formato do registro exige ao
// menos uma, e um agente que não sobra nenhuma depois do saneamento não entra
// no catálogo: não há o que dizer nem o que fazer com ele.
type Distribution struct {
	// Binary é o mapa por alvo de plataforma (`windows-x86_64` e companhia).
	Binary map[string]BinaryTarget `json:"binary,omitempty"`
	NPX    *PackageDistribution    `json:"npx,omitempty"`
	UVX    *PackageDistribution    `json:"uvx,omitempty"`
}

// Empty diz que não sobrou forma alguma de obter o agente.
func (d Distribution) Empty() bool {
	return len(d.Binary) == 0 && d.NPX == nil && d.UVX == nil
}

// BinaryTarget é o artefato de um alvo de plataforma.
type BinaryTarget struct {
	Archive string `json:"archive"`
	// SHA256 é opcional no formato do registro, e a ausência dele decide se o
	// agente pode ser instalado automaticamente (D4). Um valor que não é um
	// digest SHA-256 é descartado aqui, o que joga o alvo no mesmo conjunto de
	// quem não publica digest — a direção segura.
	SHA256 string            `json:"sha256,omitempty"`
	Cmd    string            `json:"cmd"`
	Args   []string          `json:"args,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
}

// PackageDistribution é a distribuição por gerenciador de pacotes (`npx`/`uvx`).
type PackageDistribution struct {
	Package string   `json:"package"`
	Args    []string `json:"args,omitempty"`
}

// cloneAgents devolve uma cópia funda da lista. O serviço guarda um índice só e
// entrega o catálogo a quantos leitores quiserem: sem a cópia, o slice e os
// mapas de dentro seriam os mesmos objetos em todas as mãos, e bastaria um
// chamador distraído ordenar a lista ou mexer num argumento para corromper o que
// os outros estão lendo — inclusive em paralelo.
func cloneAgents(agents []Agent) []Agent {
	if agents == nil {
		return nil
	}
	copia := make([]Agent, 0, len(agents))
	for _, agent := range agents {
		copia = append(copia, agent.clone())
	}
	return copia
}

func (a Agent) clone() Agent {
	a.Authors = slices.Clone(a.Authors)
	a.Distribution = a.Distribution.clone()
	return a
}

func (d Distribution) clone() Distribution {
	if d.Binary != nil {
		binary := make(map[string]BinaryTarget, len(d.Binary))
		for alvo, target := range d.Binary {
			binary[alvo] = target.clone()
		}
		d.Binary = binary
	}
	d.NPX = d.NPX.clone()
	d.UVX = d.UVX.clone()
	return d
}

func (b BinaryTarget) clone() BinaryTarget {
	b.Args = slices.Clone(b.Args)
	b.Env = maps.Clone(b.Env)
	return b
}

func (p *PackageDistribution) clone() *PackageDistribution {
	if p == nil {
		return nil
	}
	copia := *p
	copia.Args = slices.Clone(p.Args)
	return &copia
}

// Limites de tamanho do que vem do documento. Nenhuma entrada honesta chega
// perto deles; eles existem para um índice hostil não virar tela ilegível nem
// memória gasta à toa.
const (
	maxURLLen     = 2048
	maxAuthors    = 32
	maxArgs       = 32
	maxEnvEntries = 32
	maxPackageLen = 214 // teto de nome de pacote no npm
	maxIdentLen   = 64
)

// identRule é o que um identificador de agente ou um alvo de plataforma pode
// ter. A régua é apertada porque o `id` vira nome de diretório em
// `~/.assistente/agents/<id>/` (D5): recusar aqui, na fronteira, é mais barato
// do que confiar em quem for montar o caminho depois.
var identRule = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// packageRule é a forma de uma especificação de pacote: escopo opcional, nome e
// versão opcional. Ele vira argumento de `npm install` na Fase 3, e a régua é
// dessa forma e não de um conjunto de caracteres porque `npm` também aceita
// caminho local e atalho de repositório — `./pacote`, `~/pacote`, `usuário/repo`
// instalariam outra coisa, de outro lugar, com o mesmo campo do índice.
var packageRule = regexp.MustCompile(`^(@[a-zA-Z0-9][a-zA-Z0-9._-]*/)?[a-zA-Z0-9][a-zA-Z0-9._-]*(@[a-zA-Z0-9][a-zA-Z0-9._+-]*)?$`)

// digestRule casa os 64 hex de um SHA-256. O formato aceita maiúsculas, e o
// saneamento normaliza para minúsculas.
var digestRule = regexp.MustCompile(`^[0-9a-f]{64}$`)

// envKeyRule é um nome de variável de ambiente.
var envKeyRule = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// rawIndex é a casca do documento. Os agentes ficam crus de propósito: uma
// entrada com campo de tipo errado é descartada sozinha, em vez de derrubar o
// catálogo inteiro por causa de um vizinho.
type rawIndex struct {
	Version string            `json:"version"`
	Agents  []json.RawMessage `json:"agents"`
}

// ParseIndex valida e saneia o documento do registro.
//
// É a porta única: tanto a resposta da CDN quanto o arquivo de cache entram por
// aqui, então um cache adulterado no disco recebe o mesmo tratamento que uma
// resposta adulterada na rede.
func ParseIndex(ctx context.Context, data []byte) (Index, error) {
	var raw rawIndex
	if err := json.Unmarshal(data, &raw); err != nil {
		return Index{}, fmt.Errorf("%w: %v", ErrMalformedIndex, err)
	}

	version := acp.SanitizeLabel(raw.Version)
	major, err := majorOf(version)
	if err != nil {
		return Index{}, fmt.Errorf("%w: %v", ErrMalformedIndex, err)
	}
	if major != supportedMajor {
		return Index{}, fmt.Errorf("%w: o documento está na versão %s e este app lê a versão %d", ErrUnsupportedVersion, version, supportedMajor)
	}

	agents := make([]Agent, 0, len(raw.Agents))
	seen := make(map[string]bool, len(raw.Agents))
	for _, rawAgent := range raw.Agents {
		agent, ok := parseAgent(rawAgent)
		if !ok || seen[agent.ID] {
			continue
		}
		seen[agent.ID] = true
		agents = append(agents, agent)
	}

	if descartados := len(raw.Agents) - len(agents); descartados > 0 {
		logging.Warnf(ctx, component, "índice do registro ACP: %d de %d entradas descartadas por não passarem no saneamento", descartados, len(raw.Agents))
	}

	// Índice sem agente algum não é catálogo: ou o documento veio truncado, ou
	// a origem quebrou. Aceitá-lo apagaria, na gravação do cache, a lista que
	// estava servindo — que é exatamente o que o D2 recusa.
	if len(agents) == 0 {
		return Index{}, fmt.Errorf("%w: nenhum agente aproveitável no documento", ErrMalformedIndex)
	}

	return Index{Version: version, Agents: agents}, nil
}

// majorOf lê só o major do semver declarado pelo documento.
func majorOf(version string) (int, error) {
	head := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if i := strings.IndexAny(head, ".-+"); i >= 0 {
		head = head[:i]
	}
	if head == "" {
		return 0, errors.New("o documento não declara versão")
	}
	major, err := strconv.Atoi(head)
	if err != nil || major < 0 {
		return 0, fmt.Errorf("versão do documento ilegível: %q", version)
	}
	return major, nil
}

// parseAgent desserializa e saneia uma entrada. O `false` significa "entrada
// inaproveitável", e o chamador a descarta.
func parseAgent(raw json.RawMessage) (Agent, bool) {
	var agent Agent
	if err := json.Unmarshal(raw, &agent); err != nil {
		return Agent{}, false
	}

	agent.ID = sanitizeIdent(agent.ID)
	if agent.ID == "" {
		return Agent{}, false
	}

	agent.Name = acp.SanitizeLabel(agent.Name)
	if agent.Name == "" {
		// Sem nome, o identificador é o que sobra para a tela dizer.
		agent.Name = agent.ID
	}
	agent.Version = acp.SanitizeLabel(agent.Version)
	agent.Description = acp.SanitizeLabel(agent.Description)
	agent.License = acp.SanitizeLabel(agent.License)
	agent.Repository = sanitizeHTTPSURL(agent.Repository)
	agent.Website = sanitizeHTTPSURL(agent.Website)
	agent.Icon = sanitizeHTTPSURL(agent.Icon)
	agent.Authors = sanitizeAuthors(agent.Authors)
	agent.Distribution = sanitizeDistribution(agent.Distribution)

	if agent.Distribution.Empty() {
		return Agent{}, false
	}
	return agent, true
}

// sanitizeIdent aceita o identificador como ele é ou recusa por inteiro.
// Consertar um identificador tirando caractere daria dois agentes diferentes
// com o mesmo `id`, e o `id` é a chave de tudo daqui para frente.
func sanitizeIdent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxIdentLen || strings.Contains(s, "..") {
		return ""
	}
	if !identRule.MatchString(s) {
		return ""
	}
	return s
}

// sanitizeHTTPSURL devolve a URL só quando ela é absoluta e `https` (D9). Uma
// URL com espaço, controle ou marca invisível é recusada em vez de limpa: tirar
// caractere de uma URL produz outro endereço, e um endereço que ninguém
// escreveu é pior do que nenhum.
func sanitizeHTTPSURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxURLLen {
		return ""
	}
	if strings.IndexFunc(raw, suspectInURL) >= 0 {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return raw
}

func suspectInURL(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

func sanitizeAuthors(authors []string) []string {
	if len(authors) == 0 {
		return nil
	}
	out := make([]string, 0, min(len(authors), maxAuthors))
	for _, author := range authors {
		if name := acp.SanitizeLabel(author); name != "" {
			out = append(out, name)
			if len(out) == maxAuthors {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeDistribution(dist Distribution) Distribution {
	out := Distribution{
		NPX: sanitizePackage(dist.NPX),
		UVX: sanitizePackage(dist.UVX),
	}
	if len(dist.Binary) == 0 {
		return out
	}
	targets := make(map[string]BinaryTarget, len(dist.Binary))
	for platform, target := range dist.Binary {
		platform = sanitizeIdent(platform)
		if platform == "" {
			continue
		}
		clean, ok := sanitizeBinaryTarget(target)
		if !ok {
			continue
		}
		targets[platform] = clean
	}
	if len(targets) > 0 {
		out.Binary = targets
	}
	return out
}

// sanitizeBinaryTarget exige o que faz um alvo existir: de onde baixar e o que
// rodar depois. O `cmd` e os `args` são saneados como rótulo porque é assim que
// eles aparecem no diálogo de confirmação; a garantia de que o caminho fica
// dentro do diretório do agente é da fase que instala.
func sanitizeBinaryTarget(target BinaryTarget) (BinaryTarget, bool) {
	archive := sanitizeHTTPSURL(target.Archive)
	cmd := acp.SanitizeLabel(target.Cmd)
	if archive == "" || cmd == "" {
		return BinaryTarget{}, false
	}
	return BinaryTarget{
		Archive: archive,
		SHA256:  sanitizeDigest(target.SHA256),
		Cmd:     cmd,
		Args:    sanitizeArgs(target.Args),
		Env:     sanitizeEnv(target.Env),
	}, true
}

func sanitizeDigest(digest string) string {
	digest = strings.ToLower(strings.TrimSpace(digest))
	if !digestRule.MatchString(digest) {
		return ""
	}
	return digest
}

func sanitizePackage(pkg *PackageDistribution) *PackageDistribution {
	if pkg == nil {
		return nil
	}
	name := strings.TrimSpace(acp.SanitizeLabel(pkg.Package))
	if name == "" || len(name) > maxPackageLen || strings.Contains(name, "..") {
		return nil
	}
	if !packageRule.MatchString(name) {
		return nil
	}
	return &PackageDistribution{Package: name, Args: sanitizeArgs(pkg.Args)}
}

func sanitizeArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, min(len(args), maxArgs))
	for _, arg := range args {
		if clean := acp.SanitizeLabel(arg); clean != "" {
			out = append(out, clean)
			if len(out) == maxArgs {
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, min(len(env), maxEnvEntries))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if !envKeyRule.MatchString(key) {
			continue
		}
		out[key] = acp.SanitizeLabel(value)
		if len(out) == maxEnvEntries {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
