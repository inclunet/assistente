// Package commandledger inicia o ledger durável da AEP-0103. A API atual
// suporta somente leituras diretas local_session, sem argumentos ou contexto
// de surface/workspace. Não autentica, autoriza, executa ou purga registros.
package commandledger

import (
	"errors"
	"time"
)

type Status string

const (
	Evaluating     Status = "evaluating"
	Queued         Status = "queued"
	Running        Status = "running"
	Succeeded      Status = "succeeded"
	Failed         Status = "failed"
	Denied         Status = "denied"
	Cancelled      Status = "cancelled"
	CancelledStale Status = "cancelled_stale"
	TimedOut       Status = "timed_out"
	OutcomeUnknown Status = "outcome_unknown"
)

var (
	ErrInvalidRequest    = errors.New("solicitação de ledger inválida")
	ErrConflict          = errors.New("identidade ou fingerprint conflitante")
	ErrNotFound          = errors.New("invocação não encontrada no escopo")
	ErrExpired           = errors.New("janela de reentrega encerrada")
	ErrInvalidTransition = errors.New("transição de invocação inválida")
	ErrInconsistent      = errors.New("ledger e auditoria inconsistentes")
)

// Owner é derivado da sessão autenticada pelo serviço chamador, nunca do payload.
type Owner struct {
	UserID        string
	AuthContextID string
}

// LocalReadRequest é fornecida somente pelo executor confiável após validar
// comando read/none, sem argumentos nem alvo mutável. Os fingerprints são HMACs
// já calculados pelo serviço; este repository não calcula nem autentica HMAC.
// Origem suportada: palette, ui.action ou cli (execução direta, sem evento).
type LocalReadRequest struct {
	InvocationID              string
	Owner                     Owner
	AuthGeneration            string
	SecurityGeneration        string
	RegistryVersion           string
	GlobalConfigGeneration    string
	ActiveLayersGeneration    string
	CommandID                 string
	SourceType                string
	ArgumentsFingerprint      string
	RequestFingerprintVersion string
	RequestFingerprint        string
	CorrelationID             string
	ReceivedAt                time.Time
	ExpiresAt                 time.Time
}

// Record é uma cópia do ledger, não prova de autorização. A auditoria pode
// ser compactada no futuro sem remover esta chave de idempotência.
type Record struct {
	ID                        string
	Key                       string
	InvocationID              string
	Owner                     Owner
	SourceType                string
	RequestFingerprintVersion string
	RequestFingerprint        string
	Status                    Status
	ReceivedAt                time.Time
	ExpiresAt                 time.Time
}

type Reservation struct {
	Record  Record
	Created bool
}
