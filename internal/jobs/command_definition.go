package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"assistente/internal/commandjson"
	"github.com/google/uuid"
)

const jobDefinitionFingerprintDomain = "assistente.job-definition.v1"

var ErrInvalidJobDefinition = errors.New("definição de job inválida")

// jobDefinitionProjection contém somente o conteúdo persistido que altera a
// identidade/efeito do job. Não inclui triggers, tags, descrição, metadata ou
// qualquer estado derivado de execução.
type jobDefinitionProjection struct {
	DatabaseID      string         `json:"database_id"`
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Enabled         bool           `json:"enabled"`
	Pipeline        string         `json:"pipeline"`
	PipelineEnabled bool           `json:"pipeline_enabled"`
	Tool            string         `json:"tool"`
	Inputs          map[string]any `json:"inputs"`
	Output          OutputConfig   `json:"output"`
	Events          EventsConfig   `json:"events"`
	ErrorPolicy     ErrorPolicy    `json:"error_policy"`
	MaxRunsPerHour  int            `json:"max_runs_per_hour"`
	DryRun          DryRunConfig   `json:"dry_run"`
}

// DefinitionFingerprint retorna a versão de conteúdo da definição persistida
// do job. Não é autorização, grant, credencial ou prova de identidade.
func DefinitionFingerprint(job *Job) (string, error) {
	if job == nil || !validJobDefinitionDatabaseID(job.DatabaseID) ||
		strings.TrimSpace(job.ID) == "" || strings.TrimSpace(job.ID) != job.ID {
		return "", ErrInvalidJobDefinition
	}

	projection := jobDefinitionProjection{
		DatabaseID:      job.DatabaseID,
		ID:              job.ID,
		Name:            job.Name,
		Enabled:         job.Enabled,
		Pipeline:        job.Pipeline,
		PipelineEnabled: job.PipelineEnabled,
		Tool:            job.Tool,
		Inputs:          job.Inputs,
		Output:          job.Output,
		Events:          job.Events,
		ErrorPolicy:     job.ErrorPolicy,
		MaxRunsPerHour:  job.MaxRunsPerHour,
		DryRun:          job.DryRun,
	}
	canonical, err := commandjson.Marshal(projection)
	if err != nil {
		return "", ErrInvalidJobDefinition
	}
	framed := make([]byte, 0, len(jobDefinitionFingerprintDomain)+1+len(canonical))
	framed = append(framed, jobDefinitionFingerprintDomain...)
	framed = append(framed, 0)
	framed = append(framed, canonical...)
	digest := sha256.Sum256(framed)
	return hex.EncodeToString(digest[:]), nil
}

func validJobDefinitionDatabaseID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
