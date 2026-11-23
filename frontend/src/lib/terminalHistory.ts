import type { terminal } from '../../wailsjs/go/models';

// Nó local de saída do PTY, sem comando, duração ou status de processo.
export const sessionOutputSource = 'session-output';

export function hasTerminalEndTime(entry: terminal.HistoryEntry): boolean {
  return Boolean(entry.endedAt && entry.endedAt !== '0001-01-01T00:00:00Z');
}

export function isManagedCommandRunning(entry: terminal.HistoryEntry): boolean {
  return entry.source !== 'user-raw' && entry.source !== sessionOutputSource && entry.exitCode === -999 && !hasTerminalEndTime(entry);
}
