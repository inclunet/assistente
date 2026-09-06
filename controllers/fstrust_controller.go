package controllers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/apidto"
	"assistente/internal/fstrust"
	"assistente/internal/profiles"
	"assistente/internal/tools/invocationctx"
)

// PathAllowlistView — alias estável durante a migração Strangler (AEP-0088 D5).
type PathAllowlistView = apidto.PathAllowlistView

// FSTrustControllerConfig agrupa dependências do FSTrustController.
type FSTrustControllerConfig struct {
	FSTrustMgr *fstrust.Manager
	ProfileMgr *profiles.Manager
}

// FSTrustController expõe a gestão da allowlist de paths fora do sandbox (AEP-0092).
type FSTrustController struct {
	fsTrustMgr *fstrust.Manager
	profileMgr *profiles.Manager
}

// NewFSTrustController cria um FSTrustController com as dependências fornecidas.
func NewFSTrustController(cfg FSTrustControllerConfig) *FSTrustController {
	return &FSTrustController{
		fsTrustMgr: cfg.FSTrustMgr,
		profileMgr: cfg.ProfileMgr,
	}
}

// managementContext injeta a identidade (perfil ativo) no ctx para que os
// escopos de perfil sejam considerados nas operações de gestão chamadas pela UI.
func (c *FSTrustController) managementContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.profileMgr != nil {
		if slug := c.profileMgr.GetActiveSlug(); slug != "" {
			ctx = invocationctx.With(ctx, invocationctx.InvocationContext{ProfileSlug: slug})
		}
	}
	return ctx
}

// GetPathAllowlist lista as entradas de allowlist de path (workspace, global e
// perfil ativo). Escopos efêmeros (once/session) não aparecem — a API de gestão
// não injeta ConversationID.
func (c *FSTrustController) GetPathAllowlist(ctx context.Context) []apidto.PathAllowlistView {
	if c.fsTrustMgr == nil {
		return nil
	}
	entries := c.fsTrustMgr.List(c.managementContext(ctx))
	views := make([]apidto.PathAllowlistView, 0, len(entries))
	for _, e := range entries {
		views = append(views, apidto.PathAllowlistView{
			Path:      e.Path,
			Kind:      string(e.Kind),
			Operation: e.Operation,
			Effect:    string(fstrust.NormalizedEffect(e.Effect)),
			Scope:     string(e.Scope),
			CreatedBy: e.CreatedBy,
			CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Reason:    e.Reason,
		})
	}
	return views
}

// AddPathAllowlistEntry cria uma regra persistente pela UI de gestão. Allow e
// deny usam o mesmo contrato; o efeito é dado explícito, não uma API paralela.
func (c *FSTrustController) AddPathAllowlistEntry(ctx context.Context, path, kind, operation, effect, scope, reason string) error {
	if c.fsTrustMgr == nil {
		return fmt.Errorf("gerenciador de allowlist de path não inicializado")
	}
	eff := fstrust.Effect(strings.TrimSpace(effect))
	if eff == "" || !fstrust.ValidEffect(eff) {
		return fmt.Errorf("effect inválido para regra de path: %q (use allow ou deny)", effect)
	}
	s := fstrust.Scope(scope)
	if !s.IsPersistent() {
		return fmt.Errorf("escopo inválido para regra de path: %q (use workspace, profile ou global)", scope)
	}
	k := fstrust.Kind(kind)
	if !fstrust.ValidKind(k) {
		return fmt.Errorf("kind inválido: %q (use file ou dir)", kind)
	}
	// Normaliza antes de persistir: gravar com espaços nas bordas geraria
	// entradas que não casam como o usuário espera e ficam difíceis de remover.
	path = strings.TrimSpace(path)
	// Operação casa por igualdade e o código sempre usa minúsculas (read, write,
	// copy_from, ...). Sem normalizar, "Read"/"WRITE" persistiria e nunca casaria.
	operation = strings.ToLower(strings.TrimSpace(operation))
	reason = strings.TrimSpace(reason)
	if operation == "" {
		return fmt.Errorf("operation vazia")
	}
	if path == "" {
		return fmt.Errorf("path vazio")
	}
	// O path é digitado à mão: expandir "~" para o home, senão persistiríamos
	// uma regra literal "~/..." que nunca casa o caminho real.
	path, err := expandUserHome(path)
	if err != nil {
		return err
	}
	// Exige path absoluto: um relativo seria resolvido contra o cwd do processo,
	// criando um deny em lugar inesperado (e difícil de remover).
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path deve ser absoluto (ou começar com ~): %q", path)
	}
	// Persiste o destino real para que allow e deny casem o mesmo alvo e um
	// alias/symlink não amplie nem contorne a regra escolhida.
	resolved, err := fstrust.ResolvePath(path)
	if err != nil {
		return fmt.Errorf("não foi possível resolver o path %q para a regra: %w", path, err)
	}
	return c.fsTrustMgr.Add(c.managementContext(ctx), fstrust.AllowlistEntry{
		Path:      resolved,
		Kind:      k,
		Operation: operation,
		Effect:    fstrust.NormalizedEffect(eff),
		Scope:     s,
		CreatedBy: "user",
		Reason:    reason,
	})
}

// RemovePathAllowlistEntry remove uma entrada persistida por (scope, path, kind, operation, effect).
// Aceita apenas escopos PERSISTIDOS (workspace/profile/global): once nunca é
// persistido e session vive em memória por conversa, sem relação com esta API de
// gestão — passar esses valores retorna erro de escopo inválido direto.
func (c *FSTrustController) RemovePathAllowlistEntry(ctx context.Context, scope, path, kind, operation, effect string) error {
	if c.fsTrustMgr == nil {
		return fmt.Errorf("gerenciador de allowlist de path não inicializado")
	}
	s := fstrust.Scope(scope)
	if !s.IsPersistent() {
		return fmt.Errorf("escopo inválido para remoção: %q (use workspace, profile ou global)", scope)
	}
	k := fstrust.Kind(kind)
	if !fstrust.ValidKind(k) {
		return fmt.Errorf("kind inválido para remoção: %q (use file ou dir)", kind)
	}
	// Esta API de gestão exige effect explícito. ValidEffect aceita ""
	// (normalizado para allow), então rejeitamos vazio aqui para não tornar a
	// remoção ambígua e contradizer a mensagem ("use allow ou deny").
	eff := fstrust.Effect(strings.TrimSpace(effect))
	if eff == "" || !fstrust.ValidEffect(eff) {
		return fmt.Errorf("effect inválido para remoção: %q (use allow ou deny)", effect)
	}
	return c.fsTrustMgr.Remove(c.managementContext(ctx), s, path, k, operation, fstrust.NormalizedEffect(eff))
}

// expandUserHome troca um "~" inicial pelo diretório home do usuário. filepath.Abs
// trataria "~" como componente literal relativo, persistindo uma regra que nunca
// casaria o caminho real digitado (ex.: "~/.env").
func expandUserHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("não foi possível resolver \"~\" para o diretório home: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	// Remove separadores à esquerda do sufixo: "~//x" deixaria path[2:] começando
	// com separador e poderia alterar a semântica do Join em algumas plataformas.
	suffix := strings.TrimLeft(path[2:], "/\\")
	return filepath.Join(home, suffix), nil
}
