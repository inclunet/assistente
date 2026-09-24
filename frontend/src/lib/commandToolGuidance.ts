import type { apidto } from '@wailsjs/go/models';
import { GetRuntimeToolCatalog } from '@wailsjs/go/wailsapi/Tools';

export interface RuntimeCommandToolGuidance {
  readonly available: boolean;
  readonly displayName: string;
  readonly description?: string;
  readonly schema?: unknown;
}

const PAGE_SIZE = 50;
// Guidance is best-effort: bound lookup work even if the catalog keeps returning full pages.
// The runtime catalog is capped at 4096 entries: ceil(4096 / 50) = 82 pages.
const MAX_PAGES = 82;

export function runtimeToolIDFromCommandID(commandID: string): string | null {
  const match = /^tool\.execute\.t_([0-9a-f]{32})$/.exec(commandID);
  if (!match || match[1][12] !== '7' || !'89ab'.includes(match[1][16])) return null;
  const compact = match[1];
  return `${compact.slice(0, 8)}-${compact.slice(8, 12)}-${compact.slice(12, 16)}-${compact.slice(16, 20)}-${compact.slice(20)}`;
}

function decodeSchema(schema: unknown): unknown {
  if (Array.isArray(schema) && schema.every((byte) => Number.isInteger(byte) && byte >= 0 && byte <= 255)) {
    try { return JSON.parse(new TextDecoder().decode(new Uint8Array(schema))) as unknown; }
    catch { return undefined; }
  }
  if (typeof schema === 'string') {
    try { return JSON.parse(schema) as unknown; }
    catch { return undefined; }
  }
  return schema;
}

/** Reads owner-scoped runtime metadata for display only; execution authorization stays in ExecutePaletteCommand. */
export async function getRuntimeCommandToolGuidance(
  commandID: string,
  isCurrent: () => boolean = () => true,
): Promise<RuntimeCommandToolGuidance> {
  const id = runtimeToolIDFromCommandID(commandID);
  if (!id) return { available: false, displayName: commandID };
  try {
    for (let pageNumber = 0; pageNumber < MAX_PAGES && isCurrent(); pageNumber += 1) {
      const page = await GetRuntimeToolCatalog({ availabilityStatus: 'available', limit: PAGE_SIZE, offset: pageNumber * PAGE_SIZE } as apidto.RuntimeToolCatalogFilter);
      if (!isCurrent()) return { available: false, displayName: commandID };
      if (!Array.isArray(page)) break;
      const entry = page.find((candidate) => candidate.id.toLowerCase() === id);
      if (entry) return {
        available: true,
        displayName: entry.displayName || entry.name || commandID,
        description: entry.description || undefined,
        schema: decodeSchema(entry.schema),
      };
      if (page.length < PAGE_SIZE) break;
    }
  } catch {
    // Guidance is optional and never changes whether the backend accepts execution.
  }
  return { available: false, displayName: commandID };
}
