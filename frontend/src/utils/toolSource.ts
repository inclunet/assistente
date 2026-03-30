const MCP_PREFIX = 'mcp_';
const MCP_SEPARATOR = '__';

export interface ToolSource {
  type: 'local' | 'mcp';
  serverSlug?: string;
}

export function parseToolSource(toolName: string): ToolSource {
  if (!toolName.startsWith(MCP_PREFIX)) return { type: 'local' };
  const rest = toolName.slice(MCP_PREFIX.length);
  const sepIdx = rest.indexOf(MCP_SEPARATOR);
  if (sepIdx < 0) return { type: 'local' };
  return { type: 'mcp', serverSlug: rest.slice(0, sepIdx) };
}

export interface McpServerEntry {
  slug: string;
  name: string;
}

/**
 * Extrai servidores MCP únicos a partir dos nomes das tools.
 * Se `servers` for fornecido, usa o `name` amigável; caso contrário, usa o slug.
 */
export function extractMcpServers(
  toolNames: string[],
  servers?: McpServerEntry[],
): McpServerEntry[] {
  const slugs = new Set<string>();
  for (const name of toolNames) {
    const src = parseToolSource(name);
    if (src.type === 'mcp' && src.serverSlug) slugs.add(src.serverSlug);
  }

  const serverMap = new Map(servers?.map((s) => [s.slug, s.name]));
  return Array.from(slugs)
    .sort()
    .map((slug) => ({ slug, name: serverMap.get(slug) ?? slug }));
}
