/**
 * Formata duração em milissegundos para exibição legível.
 * Usado por ToolCallsSection, TerminalEntry e outros componentes.
 */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(Math.floor(ms / 100) / 10).toFixed(1)}s`;
  const min = Math.floor(ms / 60000);
  const sec = Math.floor((ms % 60000) / 1000);
  return `${min}m ${sec}s`;
}

/** Lê apenas o prefixo solicitado, preservando pares substitutos UTF-16. */
export function codePointPrefix(text: string, limit: number): string {
  if (limit <= 0) return '';
  if (text.length <= limit) return text;
  let end = 0;
  let count = 0;
  for (const character of text) {
    end += character.length;
    if (++count >= limit) break;
  }
  return text.slice(0, end);
}
