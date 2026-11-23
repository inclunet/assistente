import type { terminal } from '../../wailsjs/go/models';

export function hasTerminalEndTime(entry: terminal.HistoryEntry): boolean {
  return Boolean(entry.endedAt && entry.endedAt !== '0001-01-01T00:00:00Z');
}

export function isManagedCommandRunning(entry: terminal.HistoryEntry): boolean {
  return entry.source !== 'user-raw' && entry.exitCode === -999 && !hasTerminalEndTime(entry);
}
