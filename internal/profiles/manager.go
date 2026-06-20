package profiles

import (
	"assistente/internal/configdir"
	"assistente/internal/slug"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

// Manager gerencia perfis de conversa armazenados em arquivos JSON.
// Usa configdir.Resolver para resolução multi-diretório.
type Manager struct {
	resolver *configdir.Resolver
}

// NewManager cria um novo gerenciador de perfis
func NewManager() *Manager {
	return &Manager{
		resolver: configdir.NewResolver("profiles"),
	}
}

// List retorna todos os perfis resolvidos (sem duplicatas, maior prioridade ganha)
func (m *Manager) List() ([]ProfileInfo, error) {
	files, err := m.resolver.List()
	if err != nil {
		return nil, err
	}

	infos := make([]ProfileInfo, 0, len(files))
	for _, f := range files {
		// Só arquivos .json
		if !strings.HasSuffix(f.Filename, ".json") {
			continue
		}

		// Tenta ler o perfil para extrair metadados
		data, _, err := m.resolver.Read(f.Filename)
		if err != nil {
			continue
		}

		var profile Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			continue
		}

		infos = append(infos, ProfileInfo{
			Name:        profile.Name,
			Slug:        f.Name,
			Description: profile.Description,
			Icon:        profile.Icon,
			Source:      string(f.Source),
		})
	}

	// Ordena por nome
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})

	return infos, nil
}

// Get carrega um perfil pelo slug (nome do arquivo sem extensão).
//
// Aplica a normalização de routing fields (`normalizeRoutingFields`)
// imediatamente após o decode: profiles legacy com
// `Chat.LLMProvider`/`Chat.Model`/`Voice.Assistant.LLMProviderID`/
// `Input.LLMProviderID` vazios passam a expor `$default` para o resto
// do app. Isso elimina a ambiguidade "vazio quer dizer o quê?" no
// callsite — para `providers.Service.ResolveProfileDefaults` o
// significado de `$default` já é explícito e auditável.
func (m *Manager) Get(slug string) (*Profile, error) {
	filename := slug + ".json"

	data, _, err := m.resolver.Read(filename)
	if err != nil {
		return nil, fmt.Errorf("profile not found: %s", slug)
	}

	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse profile %s: %w", slug, err)
	}

	normalizeRoutingFields(&profile)
	return &profile, nil
}

// normalizeRoutingFields garante que campos de routing nunca sejam
// vazios em profiles em memória. A semântica é simples: "campo vazio
// num profile salvo é o equivalente legacy de `$default`". Profiles
// novos (criados via wizard) já vêm com `$default` explicitamente
// (ver DefaultProfile em types.go).
func normalizeRoutingFields(p *Profile) {
	if p == nil {
		return
	}
	if strings.TrimSpace(p.Chat.LLMProvider) == "" {
		p.Chat.LLMProvider = DefaultProviderSentinel
	}
	if strings.TrimSpace(p.Chat.Model) == "" {
		p.Chat.Model = DefaultProviderSentinel
	}
	if strings.TrimSpace(p.Voice.Assistant.LLMProviderID) == "" {
		p.Voice.Assistant.LLMProviderID = DefaultProviderSentinel
	}
	if strings.TrimSpace(p.Input.LLMProviderID) == "" {
		p.Input.LLMProviderID = DefaultProviderSentinel
	}
}

// Create cria um novo perfil no diretório home (~/.assistente/profiles/)
func (m *Manager) Create(profile *Profile) (string, error) {
	if err := profile.Validate(); err != nil {
		return "", err
	}

	slug := Slugify(profile.Name)
	filename := slug + ".json"

	// Verifica se já existe
	if m.resolver.Exists(filename) {
		return "", fmt.Errorf("profile already exists: %s", slug)
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "", err
	}

	if err := m.resolver.Create(filename, data); err != nil {
		return "", err
	}

	return slug, nil
}

// Duplicate cria uma copia de um perfil existente no diretorio home.
func (m *Manager) Duplicate(slug string) (string, error) {
	profile, err := m.Get(slug)
	if err != nil {
		return "", err
	}

	newProfile := *profile
	newProfile.Name = m.nextCopyName(profile.Name)
	newProfile.Active = false
	newProfile.BuiltinVersion = ""

	return m.Create(&newProfile)
}

// Update atualiza o perfil no arquivo válido (maior prioridade).
//
// Invariante de unicidade do Active: apenas UM perfil pode ter `active: true`
// no disco. Se o caller passar `profile.Active = true`, este método grava o
// arquivo destino e em seguida desativa explicitamente todos os outros
// perfis. Ou seja, `Update(slug, p)` com p.Active=true é equivalente a
// `Update + SetActive(slug)` num único call.
//
// Sem essa garantia, qualquer caller (UI de edição, importação, migração)
// que acidentalmente envie active=true introduz um segundo "ativo" no disco
// e o `GetActive` passa a depender da ordem alfabética do filesystem para
// escolher entre eles — comportamento não-determinístico já observado em
// produção (perfis embedded com active=true gravados duas vezes).
func (m *Manager) Update(slug string, profile *Profile) error {
	if err := profile.Validate(); err != nil {
		return err
	}

	filename := slug + ".json"

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}

	if err := m.resolver.Write(filename, data); err != nil {
		return err
	}

	if profile.Active {
		if err := m.deactivateOthers(slug); err != nil {
			log.Printf("[Profiles] Update(%q) marcou Active=true mas falhou ao desativar outros: %v", slug, err)
		}
	}

	return nil
}

// deactivateOthers desativa todos os perfis exceto `keepSlug`.
// Idempotente: perfis já inativos não são reescritos.
func (m *Manager) deactivateOthers(keepSlug string) error {
	files, err := m.resolver.List()
	if err != nil {
		return err
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Filename, ".json") {
			continue
		}
		otherSlug := strings.TrimSuffix(f.Filename, ".json")
		if otherSlug == keepSlug {
			continue
		}
		other, err := m.Get(otherSlug)
		if err != nil || !other.Active {
			continue
		}
		other.Active = false
		filename := otherSlug + ".json"
		data, mErr := json.MarshalIndent(other, "", "  ")
		if mErr != nil {
			log.Printf("[Profiles] erro ao serializar %q durante deactivate: %v", otherSlug, mErr)
			continue
		}
		if wErr := m.resolver.Write(filename, data); wErr != nil {
			log.Printf("[Profiles] erro ao gravar %q desativado: %v", otherSlug, wErr)
		}
	}
	return nil
}

// Delete remove o perfil válido (maior prioridade)
func (m *Manager) Delete(slug string) error {
	filename := slug + ".json"
	return m.resolver.Delete(filename)
}

// GetActive retorna o perfil marcado como Active: true em seu JSON.
//
// Auto-cura: se mais de um perfil tiver Active=true, escolhe o mais
// recentemente modificado (mtime do arquivo) e desativa os demais
// gravando-os no disco. Sem essa auto-cura o "perfil ativo" passa a
// depender da ordem alfabética do filesystem (já vimos `padrao` ser
// silenciosamente escolhido sobre `programacao` porque vinha antes na
// listagem). Aceitar a primeira ocorrência seria estável mas
// invisivelmente errada para o user — que viu o picker mostrar `X`
// mas o app continuar usando `Y`.
//
// Fallback (nenhum Active=true): prefere "padrao" sobre o primeiro perfil
// arbitrário (a ordem de iteração de filesystem não é determinística).
func (m *Manager) GetActive() (*Profile, error) {
	profile, _, err := m.resolveActive()
	return profile, err
}

// GetActiveAndSlug retorna o perfil ativo e seu slug numa única resolução
// (ver resolveActive). Para operações de ESCRITA no perfil ativo, prefira este
// método em vez de combinar GetActive + GetActiveSlug: ele propaga o erro de
// resolução e garante que perfil e slug vêm da mesma passada, evitando gravar no
// slug errado caso uma segunda resolução tolerante caísse silenciosamente em
// "padrao".
func (m *Manager) GetActiveAndSlug() (*Profile, string, error) {
	return m.resolveActive()
}

// resolveActive é a resolução canônica do perfil ativo: retorna o perfil e o
// slug correspondente, usando UMA única regra (active=true → auto-cura por mtime
// → "padrao" → primeiro perfil legível → DefaultProfile).
//
// O slug normalmente é o do arquivo de onde o perfil veio. EXCEÇÃO: no fallback
// final (nenhum perfil legível no disco) retorna DefaultProfile() com slug
// "padrao", que NÃO corresponde a um arquivo existente — gravar nele criaria
// padrao.json.
//
// EFEITO COLATERAL: NÃO é read-only. Quando detecta múltiplos perfis com
// active=true, escolhe o vencedor (mtime) e REGRAVA os demais no disco com
// active=false (auto-cura), gerando I/O e logs. Em estado saudável (0 ou 1
// ativo) é apenas leitura.
//
// GetActive e GetActiveSlug delegam para cá para aplicarem a MESMA regra de
// resolução: o slug retornado é o do arquivo de onde o perfil veio. São chamadas
// independentes (não atômicas entre si), então sob alteração concorrente do
// filesystem ainda podem observar estados diferentes; o objetivo aqui é eliminar
// a divergência de *regra* — antes cada uma desempatava de um jeito (auto-cura
// por mtime vs. ordem de listagem), o que fazia gravar/ler atingir slugs
// diferentes mesmo sem concorrência, quando havia múltiplos active=true ou
// arquivos corrompidos.
func (m *Manager) resolveActive() (*Profile, string, error) {
	files, err := m.resolver.List()
	if err != nil {
		return nil, "", fmt.Errorf("erro ao listar perfis: %w", err)
	}

	var firstProfile *Profile
	var firstSlug string
	var padraoProfile *Profile
	var actives []activeCandidate

	for _, f := range files {
		if !strings.HasSuffix(f.Filename, ".json") {
			continue
		}

		slug := strings.TrimSuffix(f.Filename, ".json")
		profile, err := m.Get(slug)
		if err != nil {
			continue
		}

		if firstProfile == nil {
			firstProfile = profile
			firstSlug = slug
		}

		if profile.Active {
			actives = append(actives, activeCandidate{slug: slug, profile: profile, path: f.Path})
		}

		if slug == "padrao" {
			padraoProfile = profile
		}
	}

	if len(actives) == 1 {
		return actives[0].profile, actives[0].slug, nil
	}
	if len(actives) > 1 {
		winner := pickMostRecentActive(actives)
		log.Printf("[Profiles] %d perfis com active=true detectados; mantendo %q (mais recente) e desativando demais", len(actives), winner.slug)
		for _, c := range actives {
			if c.slug == winner.slug {
				continue
			}
			c.profile.Active = false
			filename := c.slug + ".json"
			data, err := json.MarshalIndent(c.profile, "", "  ")
			if err != nil {
				log.Printf("[Profiles] auto-cura: erro ao serializar %q: %v", c.slug, err)
				continue
			}
			if err := m.resolver.Write(filename, data); err != nil {
				log.Printf("[Profiles] auto-cura: erro ao desativar %q: %v", c.slug, err)
			}
		}
		return winner.profile, winner.slug, nil
	}

	if padraoProfile != nil {
		return padraoProfile, "padrao", nil
	}
	if firstProfile != nil {
		return firstProfile, firstSlug, nil
	}

	return DefaultProfile(), "padrao", nil
}

// activeCandidate descreve um perfil candidato a "ativo" durante a
// auto-cura de múltiplos active=true.
type activeCandidate struct {
	slug    string
	profile *Profile
	path    string
}

// pickMostRecentActive escolhe o candidato com mtime mais recente.
// Em empate (ou erro de Stat), desempata pelo slug em ordem alfabética
// para ser determinístico entre execuções.
func pickMostRecentActive(actives []activeCandidate) activeCandidate {
	if len(actives) == 0 {
		return activeCandidate{}
	}
	best := actives[0]
	bestTime, _ := statMTime(best.path)
	for _, c := range actives[1:] {
		t, _ := statMTime(c.path)
		if t.After(bestTime) || (t.Equal(bestTime) && c.slug < best.slug) {
			best = c
			bestTime = t
		}
	}
	return best
}

func statMTime(path string) (time.Time, error) {
	if path == "" {
		return time.Time{}, fmt.Errorf("empty path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// SetActive marca um perfil como Active: true e desativa os outros
// NOTA: Migrado para usar Profile.Active em vez de config.json
func (m *Manager) SetActive(slug string) error {
	// Verifica se o perfil existe
	profile, err := m.Get(slug)
	if err != nil {
		return fmt.Errorf("profile not found: %s", slug)
	}

	// Marca como ativo
	profile.Active = true
	if err := m.Update(slug, profile); err != nil {
		return err
	}

	// Desativa os outros
	files, err := m.resolver.List()
	if err != nil {
		return nil // Não é erro crítico
	}

	for _, f := range files {
		if !strings.HasSuffix(f.Filename, ".json") {
			continue
		}
		otherSlug := strings.TrimSuffix(f.Filename, ".json")
		if otherSlug == slug {
			continue
		}

		other, err := m.Get(otherSlug)
		if err != nil {
			continue
		}

		if other.Active {
			other.Active = false
			if updateErr := m.Update(otherSlug, other); updateErr != nil {
				return fmt.Errorf("failed to deactivate profile %s: %w", otherSlug, updateErr)
			}
		}
	}

	return nil
}

// GetActiveSlug retorna o slug do perfil ativo aplicando a MESMA regra de
// resolução de GetActive (ver resolveActive). Como são chamadas independentes
// (não atômicas entre si), uma alteração concorrente do filesystem ainda pode
// fazer com que observem estados diferentes; o que garantimos é a regra de
// resolução comum, não atomicidade.
//
// ATENÇÃO: apesar do nome de getter, NÃO é estritamente read-only — delega para
// resolveActive, que pode regravar perfis no disco para auto-curar múltiplos
// active=true (ver o efeito colateral documentado lá). Em estado saudável é só
// leitura, mas callers em caminhos quentes devem estar cientes do I/O eventual.
func (m *Manager) GetActiveSlug() string {
	_, slug, err := m.resolveActive()
	if err != nil || slug == "" {
		return "padrao"
	}
	return slug
}

// GetSearchPaths retorna os caminhos de busca do resolver
func (m *Manager) GetSearchPaths() []string {
	return m.resolver.GetSearchPaths()
}

// EnsureDefaults ensures the profiles home directory exists.
// Builtin profiles are now installed by App.installBuiltinProfiles() from embedded JSON files.
func (m *Manager) EnsureDefaults() error {
	return m.resolver.EnsureHomeDir()
}

// Slugify converte um nome em slug seguro para nome de arquivo.
// Ex: "Padrão" -> "padrao", "Modelo Local" -> "modelo-local"
// Delega ao pacote canônico internal/slug, usando "perfil" como fallback.
func Slugify(name string) string {
	return slug.Slugify(name, "perfil")
}

func (m *Manager) nextCopyName(baseName string) string {
	if baseName == "" {
		baseName = "Perfil"
	}

	if candidate := baseName + " (Copia)"; !m.resolver.Exists(Slugify(candidate)+".json") {
		return candidate
	}

	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (Copia %d)", baseName, i)
		if !m.resolver.Exists(Slugify(candidate) + ".json") {
			return candidate
		}
	}
}
