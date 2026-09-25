const MAX_UINT64 = '18446744073709551615';

export interface WorkspaceSnapshotPayload {
  id: string;
  name: string;
  profile?: string;
  snapshot_epoch?: unknown;
  snapshot_sequence?: unknown;
  tabs?: {
    items?: unknown;
    active?: unknown;
  };
  [key: string]: unknown;
}

export interface ParsedWorkspaceSnapshot {
  payload: WorkspaceSnapshotPayload;
  epoch: string;
  sequence: string;
}

export type SnapshotOrder = 'newer' | 'same' | 'older';

function isCanonicalPositiveDecimal(value: unknown): value is string {
  return typeof value === 'string'
    && value.length <= 20
    && /^[1-9]\d*$/.test(value)
    && (value.length < MAX_UINT64.length
      || (value.length === MAX_UINT64.length && value <= MAX_UINT64));
}

function isWorkspacePayload(value: unknown): value is WorkspaceSnapshotPayload {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Record<string, unknown>;
  if (typeof candidate.id !== 'string' || candidate.id.length === 0
    || typeof candidate.name !== 'string') return false;
  if (candidate.tabs === null || candidate.tabs === undefined || typeof candidate.tabs !== 'object') return false;
  const tabs = candidate.tabs as { items?: unknown; active?: unknown };
  const items = tabs.items;
  if (items === null) {
    if (tabs.active !== '') return false;
  } else if (!Array.isArray(items)) {
    return false;
  }
  const tabItems = Array.isArray(items) ? items : [];
  const ids = new Set<string>();
  for (const item of tabItems) {
    if (!item || typeof item !== 'object') return false;
    const tab = item as Record<string, unknown>;
    if (typeof tab.id !== 'string' || tab.id.length === 0 || ids.has(tab.id)
      || typeof tab.type !== 'string' || tab.type.length === 0
      || (tab.title !== undefined && typeof tab.title !== 'string')
      || typeof tab.position !== 'number' || !Number.isInteger(tab.position) || tab.position < 0) {
      return false;
    }
    ids.add(tab.id);
  }
  if (tabs.active !== undefined && typeof tabs.active !== 'string') return false;
  if (tabs.active === undefined && tabItems.length > 0) return false;
  if (tabs.active === '' && tabItems.length > 0) return false;
  return tabs.active === undefined || tabs.active === '' || ids.has(tabs.active);
}

export function parseWorkspaceSnapshot(value: unknown): ParsedWorkspaceSnapshot | null {
  if (!isWorkspacePayload(value)) return null;
  const epoch = value.snapshot_epoch;
  const sequence = value.snapshot_sequence;
  if (typeof epoch !== 'string' || epoch.trim().length === 0 || epoch.length > 256
    || epoch !== epoch.trim() || !isCanonicalPositiveDecimal(sequence)) {
    return null;
  }
  return { payload: value, epoch, sequence };
}

export function compareWorkspaceSnapshots(
  current: Pick<ParsedWorkspaceSnapshot, 'epoch' | 'sequence'>,
  incoming: Pick<ParsedWorkspaceSnapshot, 'epoch' | 'sequence'>,
): SnapshotOrder | 'different-epoch' {
  if (current.epoch !== incoming.epoch) return 'different-epoch';
  if (current.sequence === incoming.sequence) return 'same';
  if (current.sequence.length !== incoming.sequence.length) {
    return incoming.sequence.length > current.sequence.length ? 'newer' : 'older';
  }
  return incoming.sequence > current.sequence ? 'newer' : 'older';
}
