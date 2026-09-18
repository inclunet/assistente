export type SearchResultTarget =
  | { kind: 'file'; path: string }
  | { kind: 'url'; url: string };

export interface SearchResultItem {
  kind: 'file' | 'url' | 'text';
  title: string;
  snippet?: string;
  target?: SearchResultTarget;
}

export interface SearchResultPresentation {
  total: number;
  truncated: boolean;
  items: SearchResultItem[];
}

const MAX_ITEMS = 100;
const MAX_TEXT_LENGTH = 4_096;

function text(value: unknown): string | undefined {
  return typeof value === 'string' && value.length > 0 && value.length <= MAX_TEXT_LENGTH ? value : undefined;
}

function target(value: unknown): SearchResultTarget | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const source = value as Record<string, unknown>;
  if (source.kind === 'file') {
    const path = text(source.path);
    return path ? { kind: 'file', path } : undefined;
  }
  if (source.kind === 'url') {
    const url = text(source.url);
    if (!url) return undefined;
    try {
      const parsed = new URL(url);
      return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? { kind: 'url', url: parsed.toString() } : undefined;
    } catch { return undefined; }
  }
  return undefined;
}

/** Lê exclusivamente o contrato versionado e emitido pelo backend nativo. */
export function parseSearchResultPresentation(metadata: string | undefined): SearchResultPresentation | undefined {
  try {
    const parsed: unknown = JSON.parse(metadata ?? '');
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return undefined;
    const source = (parsed as Record<string, unknown>).search_result_presentation;
    if (!source || typeof source !== 'object' || Array.isArray(source)) return undefined;
    const value = source as Record<string, unknown>;
    if (value.version !== 1 || !Array.isArray(value.items) || !Number.isSafeInteger(value.total) || (value.total as number) < 0) return undefined;
    const items: SearchResultItem[] = [];
    for (const rawItem of value.items.slice(0, MAX_ITEMS)) {
      if (!rawItem || typeof rawItem !== 'object' || Array.isArray(rawItem)) continue;
      const item = rawItem as Record<string, unknown>;
      const kind = item.kind;
      const title = text(item.title);
      if ((kind !== 'file' && kind !== 'url' && kind !== 'text') || !title) continue;
      const snippet = text(item.snippet);
      const itemTarget = target(item.target);
      items.push({ kind, title, ...(snippet ? { snippet } : {}), ...(itemTarget ? { target: itemTarget } : {}) });
    }
    return { total: value.total as number, truncated: value.truncated === true, items };
  } catch { return undefined; }
}
