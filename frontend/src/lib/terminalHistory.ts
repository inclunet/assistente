import type { terminal } from '../../wailsjs/go/models';

// Nó local de saída do PTY, sem comando, duração ou status de processo.
export const sessionOutputSource = 'session-output';

/** Preserva a ordem temporal e limita separadamente os comandos e os blocos da sessão. */
export function recentTerminalHistory(entries: terminal.HistoryEntry[], limitPerKind: number): terminal.HistoryEntry[] {
  if (entries.length <= limitPerKind) return entries;
  let commands = 0;
  let blocks = 0;
  const kept: terminal.HistoryEntry[] = [];
  for (let i = entries.length - 1; i >= 0; i--) {
    const entry = entries[i];
    const count = entry.source === sessionOutputSource ? ++blocks : ++commands;
    if (count <= limitPerKind) kept.push(entry);
    if (commands >= limitPerKind && blocks >= limitPerKind) break;
  }
  return kept.reverse();
}

export function hasTerminalEndTime(entry: terminal.HistoryEntry): boolean {
  return Boolean(entry.endedAt && entry.endedAt !== '0001-01-01T00:00:00Z');
}

export function isManagedCommandRunning(entry: terminal.HistoryEntry): boolean {
  return entry.source !== 'user-raw' && entry.source !== sessionOutputSource && entry.exitCode === -999 && !hasTerminalEndTime(entry);
}
