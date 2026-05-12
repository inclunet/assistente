import type { llm } from '../../wailsjs/go/models';

type WorkspaceSurfaceTab = {
  type: string;
  title?: string;
  state?: Record<string, unknown>;
};

export type ChatSurfaceContext = Record<string, unknown>;

function serializeRecord(value: Record<string, unknown> | undefined): string | undefined {
  if (!value) return undefined;
  if (Object.keys(value).length === 0) return undefined;
  return JSON.stringify(value);
}

export function buildChatSurfaceParams(
  tab: WorkspaceSurfaceTab | null | undefined,
  opts?: {
    profileSlug?: string;
    context?: ChatSurfaceContext;
  },
): Partial<llm.ChatParams> {
  const state = tab?.state;
  const activeFilePath = typeof state?.filePath === 'string' && state.filePath.trim()
    ? state.filePath.trim()
    : undefined;

  return {
    profileSlug: opts?.profileSlug,
    tabType: tab?.type || undefined,
    activeFilePath,
    surfaceStateJson: serializeRecord(state),
    surfaceContextJson: serializeRecord(opts?.context),
  };
}
