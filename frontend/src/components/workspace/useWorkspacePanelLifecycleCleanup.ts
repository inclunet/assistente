/**
 * Mantido como fronteira explícita do ciclo de vida dos painéis.
 *
 * Sessões PTY são recursos independentes (AEP-0089): remover uma aba ou trocar
 * de workspace apenas desconecta a visualização. Encerramento acontece somente
 * por uma ação explícita na toolbar ou por uma tool do chat.
 */
export function useWorkspacePanelLifecycleCleanup() {
  // Sem cleanup de domínio.
}
