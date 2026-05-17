package jobs

// JobExecutionMaxResultSizeBytes é o budget máximo de resultado para execução de tools
// no caminho de jobs/testes de catálogo. A persistência em tool_invocations pode truncar
// separadamente; este limite existe para evitar truncar JSON durante o processamento de jobs.
const JobExecutionMaxResultSizeBytes = 100 * 1024 * 1024
