import type { llm } from '../../wailsjs/go/models';

type WorkspaceSurfaceTab = {
  id?: string;
  type: string;
  title?: string;
  contentId?: string;
  state?: Record<string, unknown>;
};

export type SurfaceType = 'editor' | 'tasklist' | 'terminal' | (string & {});

export type SurfaceRange = {
  startLine?: number;
  startColumn?: number;
  endLine?: number;
  endColumn?: number;
  startOffset?: number;
  endOffset?: number;
};

export type SurfaceSelection = {
  kind: string;
  text?: string;
  markdown?: string;
  range?: SurfaceRange;
  isEmpty?: boolean;
  explicit?: boolean;
  items?: Array<Record<string, unknown>>;
};

export type SurfaceFocus = {
  kind: string;
  label?: string;
  text?: string;
  range?: SurfaceRange;
  cursor?: {
    line?: number;
    column?: number;
    offset?: number;
  };
  entity?: Record<string, unknown>;
};

export type SurfaceContent = {
  kind: string;
  text?: string;
  markdown?: string;
  summary?: string;
  recentOutput?: string;
  currentInput?: string;
  truncated?: boolean;
};

export type SurfaceContext = {
  surfaceType: SurfaceType;
  surfaceId: string;
  title?: string;
  mode?: string;
  selection?: SurfaceSelection;
  focus?: SurfaceFocus;
  content?: SurfaceContent;
  metadata?: Record<string, unknown>;
  snapshotVersion: string;
  capturedAt?: string;
  staleAfterMs?: number;
};

export type ChatSurfaceContext = SurfaceContext | Record<string, unknown>;

function serializeRecord(value: Record<string, unknown> | undefined): string | undefined;
function serializeRecord(value: SurfaceContext | undefined): string | undefined;
function serializeRecord(value: Record<string, unknown> | SurfaceContext | undefined): string | undefined {
  if (!value) return undefined;
  if (Object.keys(value).length === 0) return undefined;
  return JSON.stringify(value);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function stringValue(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
}

export function hashSurfaceValue(value: string): string {
  let hash = 2166136261;
  for (let i = 0; i < value.length; i += 1) {
    hash ^= value.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(36);
}

export function boundedSurfaceSnapshotValue(value: string, maxLength = 512): string {
  if (value.length <= maxLength) return value;
  return `${value.slice(0, maxLength)}:len=${value.length}`;
}

export function createSurfaceSnapshotVersion(
  surfaceType: string,
  surfaceId: string,
  value: string,
): string {
  return `${surfaceType}:${surfaceId}:${hashSurfaceValue(value)}`;
}

function fallbackSurfaceId(tab: WorkspaceSurfaceTab | null | undefined): string {
  const state = tab?.state;
  return (
    stringValue(tab?.id) ||
    stringValue(tab?.contentId) ||
    stringValue(state?.sessionId) ||
    stringValue(state?.tasklistId) ||
    stringValue(state?.draftId) ||
    stringValue(state?.filePath) ||
    stringValue(tab?.type) ||
    'surface'
  );
}

function hasSurfaceEnvelope(value: Record<string, unknown>): value is SurfaceContext {
  return !!(
    stringValue(value.surfaceType) &&
    stringValue(value.surfaceId) &&
    stringValue(value.snapshotVersion)
  );
}

export function normalizeSurfaceContext(
  tab: WorkspaceSurfaceTab | null | undefined,
  context: ChatSurfaceContext | undefined,
): SurfaceContext | undefined {
  if (!context || !isRecord(context) || Object.keys(context).length === 0) return undefined;

  if (hasSurfaceEnvelope(context)) {
    return {
      ...(context as SurfaceContext),
      capturedAt: stringValue(context.capturedAt) || new Date().toISOString(),
    };
  }

  const state = tab?.state;
  const surfaceType = (stringValue(tab?.type) || 'surface') as SurfaceType;
  const surfaceId = fallbackSurfaceId(tab);
  const mode = stringValue(context.mode) || stringValue(state?.mode);
  const selectedText = stringValue(context.selectedText);
  const selectedMarkdown = stringValue(context.selectedMarkdown);
  const cursorContext = stringValue(context.cursorContext);
  const historyPreview = stringValue(context.historyPreview);
  const tasksPreview = stringValue(context.tasksPreview);

  const selection = selectedText || selectedMarkdown
    ? {
        kind: 'text',
        text: selectedText,
        markdown: selectedMarkdown,
        isEmpty: Boolean(context.selectionIsEmpty),
        explicit: !context.selectionIsEmpty,
      }
    : undefined;

  const content = historyPreview
    ? { kind: 'terminal_output', recentOutput: historyPreview }
    : tasksPreview
      ? { kind: 'tasklist_summary', summary: tasksPreview }
      : cursorContext
        ? { kind: 'cursor_context', text: cursorContext }
        : undefined;

  return {
    surfaceType,
    surfaceId,
    title: stringValue(tab?.title),
    mode,
    selection,
    focus: cursorContext ? { kind: 'cursor', text: cursorContext } : undefined,
    content,
    metadata: {
      ...('projectId' in context ? { projectId: context.projectId } : {}),
      legacySurfaceContext: true,
    },
    snapshotVersion: createSurfaceSnapshotVersion(surfaceType, surfaceId, JSON.stringify({ state, context })),
    capturedAt: new Date().toISOString(),
  };
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
    surfaceContextJson: serializeRecord(normalizeSurfaceContext(tab, opts?.context)),
  };
}
