package jobs

import (
	"os"
	"strconv"
	"strings"
)

// JobExecutionMaxResultSizeBytes é o budget máximo de resultado para execução de tools
// no caminho de jobs/testes de catálogo. A persistência em tool_invocations pode truncar
// separadamente; este limite existe para evitar truncar JSON durante o processamento de jobs.
//
// Observação: outputs grandes podem gerar pressão de memória (strings + unmarshal).
// Para ajustar em runtime, use a env var ASSISTENTE_JOB_EXECUTION_MAX_RESULT_BYTES.
var JobExecutionMaxResultSizeBytes = resolveJobExecutionMaxResultSizeBytes()

func resolveJobExecutionMaxResultSizeBytes() int {
	const defaultBytes = 100 * 1024 * 1024
	raw := os.Getenv("ASSISTENTE_JOB_EXECUTION_MAX_RESULT_BYTES")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultBytes
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultBytes
	}
	return v
}
