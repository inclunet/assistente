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

export interface EditorDocumentReadResult {
  path: string;
  content: string;
  projected: boolean;
  format: string;
  readOnly: boolean;
  pages?: number;
  warnings: string[];
  warningCode: string;
}

/** Normaliza bindings novos e retornos string legados durante a migração. */
export function normalizeEditorDocumentResult(res: unknown, fallbackPath = ''): EditorDocumentReadResult {
  if (typeof res === 'string') {
    return {
      path: fallbackPath,
      content: res,
      projected: false,
      format: '',
      readOnly: false,
      warnings: [],
      warningCode: '',
    };
  }
  const value = res && typeof res === 'object' ? res as Record<string, unknown> : {};
  const projected = value.projected === true;
  return {
    path: String(value.path ?? fallbackPath),
    content: String(value.content ?? ''),
    projected,
    format: String(value.format ?? ''),
    readOnly: value.readOnly === true || projected,
    pages: Number(value.pages) > 0 ? Number(value.pages) : undefined,
    warnings: Array.isArray(value.warnings) ? value.warnings.map(String) : [],
    warningCode: String(value.warningCode ?? ''),
  };
}
