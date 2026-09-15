package commandconfig

import (
	"context"
	"encoding/json"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ConfirmedBindingEnabledChange não pode ser fabricada a partir de resposta da
// UI. Guarda a proposta privada, o serviço e os vínculos exatos da confirmação.
// Continua não sendo autorização: o host reautentica e verifica epochs sob gate.
type ConfirmedBindingEnabledChange struct {
	store    *Store
	change   *BindingEnabledChange
	receipts *commanddecision.Store
	epoch    commandsecurity.EpochSnapshot
	request  commanddecision.Request
}

// ConfirmBindingEnabled assina o snapshot exato e espera a decisão FORA do
// gate. keys, epoch, render e receipts são portas do host, nunca do payload.
// render recebe cópias; seu resultado só é mostrado, nunca persistido no receipt.
// Nenhuma chave é provisionada e nenhuma alteração do binding ocorre aqui.
func (s *Store) ConfirmBindingEnabled(ctx context.Context, change *BindingEnabledChange,
	epoch commandsecurity.EpochSnapshot, receipts *commanddecision.Store,
	version string, keys commandledger.FingerprintKeyProvider, expiresAt time.Time,
	render func(Binding, Binding) (string, error),
) (*ConfirmedBindingEnabledChange, error) {
	if s == nil || s.db == nil || ctx == nil || change == nil || change.store != s ||
		change.baseline == nil || change.baseline.store != s || len(change.baseline.generations) != 1 ||
		change.baseline.scope.WorkspaceID != nil || epoch.UserID != change.before.UserID ||
		!validID(change.mutationID) || receipts == nil || render == nil || !expiresAt.After(time.Now()) {
		return nil, ErrInvalid
	}
	beforeJSON, err := json.Marshal(change.before)
	if err != nil {
		return nil, err
	}
	afterJSON, err := json.Marshal(change.after)
	if err != nil {
		return nil, err
	}
	generation := change.baseline.generations[0]
	fingerprint, err := commandledger.SignConfigurationMutation(ctx, commandledger.ConfigurationMutationRequest{
		MutationID: change.mutationID, UserID: epoch.UserID, SessionID: epoch.SessionID,
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		GenerationID: generation.ID, Generation: generation.Generation,
		BeforeDocument: string(beforeJSON), AfterDocument: string(afterJSON),
	}, version, keys)
	if err != nil {
		return nil, err
	}
	before, after := change.Diff()
	body, err := render(before, after)
	if err != nil {
		return nil, err
	}
	decisionID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	request := commanddecision.Request{DecisionID: decisionID.String(), MutationID: change.mutationID,
		UserID: epoch.UserID, SessionID: epoch.SessionID, Fingerprint: fingerprint,
		AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration,
		ExpiresAt: time.UnixMilli(expiresAt.UnixMilli()).UTC(), Body: body}
	status, err := receipts.Decide(ctx, request)
	if err != nil {
		return nil, err
	}
	if status != commanddecision.Accepted {
		return nil, commanddecision.ErrStale
	}
	request.Body = "" // Não reter o texto do diálogo no comprovante interno.
	return &ConfirmedBindingEnabledChange{store: s, change: change, receipts: receipts, epoch: epoch, request: request}, nil
}

// CommitConfirmedBindingEnabled consome receipt, registra seu evento, faz CAS
// da geração e altera enabled em UMA transação no mesmo banco. Falha em qualquer
// etapa reverte tudo. Exige DispatchGate exclusivo, autenticação/política
// revalidadas, epoch atual e mapa invalidado; não chama cofre, UI ou rede.
// O histórico do receipt não substitui o futuro ledger de comandos write.
func (s *Store) CommitConfirmedBindingEnabled(ctx context.Context, confirmed *ConfirmedBindingEnabledChange, epoch commandsecurity.EpochSnapshot) error {
	if s == nil || confirmed == nil || confirmed.store != s || confirmed.receipts == nil || epoch != confirmed.epoch {
		return ErrInvalid
	}
	return s.commitBindingEnabled(ctx, confirmed.change, func(apply func(*gorm.DB) error) error {
		return confirmed.receipts.ConsumeForDatabase(ctx, s.db, confirmed.request, apply)
	})
}
