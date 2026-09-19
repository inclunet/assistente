import type { ToolOrigin } from '../types/chat';

export type ToolTarget =
  | { kind: 'file'; path: string; label: string }
  | { kind: 'url'; url: string; label: string };

export interface ToolPresentation {
  /** Chave i18n; a UI é responsável por localizar a frase. */
  labelKey: string;
  labelValues?: Record<string, string>;
  target?: ToolTarget;
}

type TranslateToolPresentation = (
  key: string,
  values?: Record<string, string>,
) => string;

/** Fonte única do texto curto usado no card e no anúncio acessível. */
export function formatToolPresentation(
  presentation: ToolPresentation,
  translate: TranslateToolPresentation,
): string {
  const action = translate(presentation.labelKey, presentation.labelValues);
  return presentation.target ? `${action}: ${presentation.target.label}` : action;
}

function parseArguments(raw?: string): Record<string, unknown> | undefined {
  if (!raw) return undefined;
  try {
    const parsed: unknown = JSON.parse(raw);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed as Record<string, unknown>
      : undefined;
  } catch {
    return undefined;
  }
}

function textArgument(args: Record<string, unknown> | undefined, ...names: string[]): string | undefined {
  for (const name of names) {
    const value = args?.[name];
    if (typeof value === 'string' && value.trim()) return value.trim();
  }
  return undefined;
}

function fileLabel(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).pop() || path;
}

function urlTarget(value: string): ToolTarget | undefined {
  try {
    const url = new URL(value);
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return undefined;
    return { kind: 'url', url: url.toString(), label: url.hostname };
  } catch {
    return undefined;
  }
}

/**
 * Traduz somente ferramentas nativas que conhecemos. Integrações MCP mantêm
 * uma descrição genérica pelo provedor: não inferimos intenção de schemas ou
 * nomes arbitrários de terceiros (AEP-0107 D3).
 */
export function presentTool(name: string, origin?: ToolOrigin, serverLabel?: string, rawArgs?: string): ToolPresentation {
  if (origin === 'mcp_bridge' || origin === 'mcp_native') {
    return serverLabel?.trim()
      ? { labelKey: 'chat.toolMcpProvider', labelValues: { provider: serverLabel.trim() } }
      : { labelKey: 'chat.toolMcpIntegration' };
  }

  if (origin !== undefined && origin !== 'builtin') {
    return { labelKey: 'chat.toolGeneric' };
  }

  const args = parseArguments(rawArgs);
  const path = textArgument(args, 'path', 'filePath', 'file');
  const url = textArgument(args, 'url', 'uri', 'href');
  const fileTarget = path ? { kind: 'file' as const, path, label: fileLabel(path) } : undefined;
  const webTarget = url ? urlTarget(url) : undefined;

  switch (name) {
    case 'read_file': return { labelKey: 'chat.toolReadFile', target: fileTarget };
    case 'write_file':
    case 'edit_file':
    case 'apply_patch': return { labelKey: 'chat.toolEditFile', target: fileTarget };
    case 'list_directory': return { labelKey: 'chat.toolListDirectory' };
    case 'search_files':
    case 'grep_search': return { labelKey: 'chat.toolSearchFiles' };
    case 'web_search':
    case 'search_web': return { labelKey: 'chat.toolSearchWeb', target: webTarget };
    case 'web_fetch':
    case 'http_request': return { labelKey: 'chat.toolAccessUrl', target: webTarget };
    default: return { labelKey: 'chat.toolGeneric' };
  }
}
