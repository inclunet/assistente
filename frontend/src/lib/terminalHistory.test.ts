import { describe, expect, it } from 'vitest';
import { terminal } from '../../wailsjs/go/models';
import { terminalChatHistory } from './terminalHistory';
import limits from './terminalContextLimits.json';

describe('contexto recente do terminal', () => {
  const entry = (source: string, command: string, output: string) =>
    terminal.HistoryEntry.createFrom({ source, command, output });
  const project = (entries: terminal.HistoryEntry[]) => terminalChatHistory(entries, 'Sessão', '… truncado');

  it('preserva o comando recente quando um bloco anterior ocupa todo o orçamento', () => {
    const result = project([entry('session-output', '', '🟣'.repeat(25000)), entry('llm', 'pwd', '/casa')]);
    expect(result.text).toMatch(/^… truncado\n\[Sessão\]\n🟣+…\n---\n\$ pwd\n\/casa$/u);
    expect(Array.from(result.text)).toHaveLength(limits.textFieldCodePoints);
    expect(result.truncated).toBe(true);
  });

  it('aplica o limite exato em pontos de código e informa quando o excede', () => {
    const output = '🟣'.repeat(limits.textFieldCodePoints - '$ cmd\n'.length);
    expect(project([entry('llm', 'cmd', output)])).toEqual({ text: '$ cmd\n' + output, truncated: false });
    const result = project([entry('llm', 'cmd', output + '🟣')]);
    expect(result.text).toMatch(/^… truncado\n\$ cmd\n🟣+…$/u);
    expect(Array.from(result.text)).toHaveLength(limits.textFieldCodePoints);
    expect(result.truncated).toBe(true);
  });
});
