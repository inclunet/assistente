import { codePointPrefix } from '../utils/format';
import limits from './terminalContextLimits.json';
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

/** O provedor lê recentOutput com um limite de pontos de código antes do orçamento do perfil. */
export function terminalChatHistory(history: terminal.HistoryEntry[], sessionOutputLabel: string, truncatedLabel: string) {
  const recent = recentTerminalHistory(history, 40);
  const separator = '\n---\n';
  // Seleciona primeiro os comandos mais recentes, depois preenche com blocos da sessão.
  const newest = recent.map((entry, index) => ({ entry, index })).reverse();
  const priority = [
    ...newest.filter(({ entry }) => entry.source !== sessionOutputSource),
    ...newest.filter(({ entry }) => entry.source === sessionOutputSource),
  ];

  function project(budget: number) {
    const parts: string[] = [];
    let truncated = history.length > recent.length;
    let kept = 0;
    for (const { entry, index } of priority) {
      const session = entry.source === sessionOutputSource;
      const command = (entry.command ?? '').trim();
      let header = session ? `[${sessionOutputLabel}]` : `$ ${command}`;
      const raw = entry.output ?? '';
      const output = session ? raw : raw.trimEnd();
      const available = budget - (kept ? separator.length : 0);
      if (available <= 0) {
        truncated = true;
        break;
      }
      const headerLength = Array.from(codePointPrefix(header, available + 1)).length;
      if (!session && !command) {
        truncated = true;
        continue;
      }
      if (headerLength + (output ? 2 : 0) > available) {
        truncated = true;
        // Identifica até um comando mais longo que o orçamento inteiro.
        // Quando só sobra uma fração após outro item, pule o cabeçalho incompleto.
        if (session || kept) continue;
        const headerBudget = output ? Math.floor(available / 2) : available;
        header = codePointPrefix(header, headerBudget - 1) + '…';
      }
      let excerpt = header;
      if (output) {
        const room = available - Array.from(header).length - 1;
        const fits = codePointPrefix(output, room).length === output.length;
        if (!fits && room < 2) {
          truncated = true;
          continue;
        }
        // Duas unidades UTF-16 por ponto bastam para ler apenas a cauda necessária.
        const tailSize = room - 1;
        const tail = fits ? output : '…' + Array.from(output.slice(-2 * tailSize)).slice(-tailSize).join('');
        // Um item anterior só entra se couber sua última linha, que pode ser
        // o diagnóstico ou a indicação de corte acrescentada pelo histórico.
        if (kept && !fits) {
          const lastBreak = output.lastIndexOf('\n');
          if (lastBreak >= 0 && output.length - lastBreak - 1 > tail.length - 1) {
            truncated = true;
            continue;
          }
        }
        excerpt += '\n' + tail;
        if (!fits) truncated = true;
      }
      parts[index] = excerpt;
      kept++;
      budget = available - Array.from(excerpt).length;
    }
    return { text: parts.filter(Boolean).join(separator), truncated };
  }

  const full = project(limits.textFieldCodePoints);
  if (!full.truncated) return full;
  const notice = truncatedLabel + '\n';
  const excerpt = project(limits.textFieldCodePoints - Array.from(notice).length);
  return { text: notice + excerpt.text, truncated: true };
}

export function hasTerminalEndTime(entry: terminal.HistoryEntry): boolean {
  return Boolean(entry.endedAt && entry.endedAt !== '0001-01-01T00:00:00Z');
}

export function isManagedCommandRunning(entry: terminal.HistoryEntry): boolean {
  return entry.source !== 'user-raw' && entry.source !== sessionOutputSource && entry.exitCode === -999 && !hasTerminalEndTime(entry);
}
