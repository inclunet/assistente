/**
 * Remove caracteres markdown de um texto para uso em leitores de tela
 * e anúncios ARIA (aria-label, aria-live).
 *
 * @param codeBlockLabel rótulo falado no lugar de fences ``` (i18n no announcer)
 */
export const stripMarkdown = (
  text: string,
  options?: { codeBlockLabel?: string },
): string => {
  if (!text) return text;

  const codeBlockLabel = options?.codeBlockLabel ?? 'bloco de código';

  return text
    // Remove blocos de código
    .replace(/```[\s\S]*?```/g, codeBlockLabel)
    .replace(/`([^`]+)`/g, '$1')
    // Remove negrito e itálico
    .replace(/\*\*\*(.+?)\*\*\*/g, '$1')
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/\*(.+?)\*/g, '$1')
    .replace(/___(.+?)___/g, '$1')
    .replace(/__(.+?)__/g, '$1')
    .replace(/_(.+?)_/g, '$1')
    // Imagens ![alt](url) → alt (antes de links)
    .replace(/!\[([^\]]*)\]\([^\)]+\)/g, '$1')
    // Remove links mas mantém o texto
    .replace(/\[([^\]]+)\]\([^\)]+\)/g, '$1')
    // Remove cabeçalhos
    .replace(/^#{1,6}\s+/gm, '')
    // Remove listas
    .replace(/^\s*[-*+]\s+/gm, '')
    .replace(/^\s*\d+\.\s+/gm, '')
    // Remove citações
    .replace(/^\s*>\s+/gm, '')
    // Remove linha horizontal
    .replace(/^[\-*_]{3,}$/gm, '')
    // Remove múltiplas quebras de linha
    .replace(/\n{3,}/g, '\n\n')
    // Trim
    .trim();
};

/** Sufixo novo entre previous e plain (LCP) — evita reler tudo se o strip reescrever o prefixo. */
export function plainSpeechDelta(previous: string, plain: string): string {
  if (!previous) return plain;
  if (plain.startsWith(previous)) return plain.slice(previous.length);
  let i = 0;
  const n = Math.min(previous.length, plain.length);
  while (i < n && previous[i] === plain[i]) i += 1;
  return plain.slice(i);
}
