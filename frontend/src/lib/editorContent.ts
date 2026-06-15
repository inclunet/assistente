/** Extrai uma mensagem de erro legível de um valor desconhecido. */
export function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error ?? '');
}

/**
 * Normaliza o retorno de leituras do backend que podem vir como string crua
 * ou como objeto `{ content }`.
 */
export function getMaybeContent(res: unknown): string {
  if (typeof res === 'string') return res;
  if (res && typeof res === 'object' && 'content' in res) {
    const value = (res as { content?: string }).content;
    return typeof value === 'string' ? value : String(value ?? '');
  }
  return '';
}
