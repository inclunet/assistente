import { isSafeLinkHref } from './safeLink';

function firstToken(text: string): string {
  const parts = String(text ?? '')
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  return parts[0] ?? '';
}

export function normalizePastedLinkHref(pastedText: string): string | null {
  const token = firstToken(pastedText);
  if (!token) return null;

  const lower = token.toLowerCase();
  const likelyUrl =
    lower.startsWith('http://') ||
    lower.startsWith('https://') ||
    lower.startsWith('mailto:') ||
    lower.startsWith('#') ||
    lower.startsWith('/') ||
    lower.startsWith('./') ||
    lower.startsWith('../') ||
    lower.startsWith('www.');

  if (!likelyUrl) return null;

  const normalized = lower.startsWith('www.') ? `https://${token}` : token;
  if (!isSafeLinkHref(normalized)) return null;
  return normalized;
}

export function escapeMarkdownLinkText(text: string): string {
  return String(text ?? '')
    .replace(/\r?\n/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\\/g, '\\\\')
    .replace(/\[/g, '\\[')
    .replace(/\]/g, '\\]');
}

export function buildMarkdownLinkFromSelection(params: {
  selectedText: string;
  href: string;
}): string {
  const href = String(params.href ?? '').trim();
  const selectedText = escapeMarkdownLinkText(params.selectedText);

  // Usa <...> no destino para reduzir problemas com parênteses/espaços.
  const safeHref = `<${href}>`;
  const label = selectedText || href;
  return `[${label}](${safeHref})`;
}
