import { describe, expect, it } from 'vitest';
import { terminal } from '../../wailsjs/go/models';
import { terminalChatHistory } from './terminalHistory';
import limits from './terminalContextLimits.json';

describe('contexto recente do terminal', () => {
  const entry = (source: string, command: string, output: string) =>
    terminal.HistoryEntry.createFrom({ source, command, output });
  const project = (entries: terminal.HistoryEntry[]) => terminalChatHistory(entries, {
    sessionOutput: 'Sessão', blankInput: 'Entrada em branco', truncated: '… truncado',
  });

  it('ignora comandos em branco antes de aplicar o limite por origem', () => {
    const empty = entry('user-raw', ' \t\n ', '');
    expect(project([empty])).toMatchObject({ text: '', items: [], truncated: false });
    const meaningful = entry('llm', 'pwd', '/casa');
    expect(project([meaningful, ...Array(limits.itemsPerKind + 1).fill(empty)]))
      .toEqual(project([meaningful]));
  });

  it('mantém a resposta a uma entrada em branco como saída do comando', () => {
    const result = project([
      entry('user-raw', ' \t', ''),
      entry('user-raw', '\n', 'Opção padrão aceita\n'),
    ]);
    expect(result).toEqual({
      text: '[Entrada em branco]\nOpção padrão aceita',
      items: [{ kind: 'command', header: '[Entrada em branco]', output: 'Opção padrão aceita' }],
      truncated: false,
    });
  });

  it('preserva o comando recente quando um bloco anterior ocupa todo o orçamento', () => {
    const result = project([entry('session-output', '', '🟣'.repeat(25000)), entry('llm', 'pwd', '/casa')]);
    expect(result.text).toMatch(/^… truncado\n\[Sessão\]\n…🟣+\n---\n\$ pwd\n\/casa$/u);
    expect(Array.from(result.text)).toHaveLength(limits.textFieldCodePoints);
    expect(result.truncated).toBe(true);
  });

  it('aplica o limite exato em pontos de código e informa quando o excede', () => {
    const output = '🟣'.repeat(limits.textFieldCodePoints - '$ cmd\n'.length);
    expect(project([entry('llm', 'cmd', output)])).toEqual({
      text: '$ cmd\n' + output, items: [{ kind: 'command', header: '$ cmd', output }], truncated: false,
    });
    const result = project([entry('llm', 'cmd', output + '🟣')]);
    expect(result.text).toMatch(/^… truncado\n\$ cmd\n…🟣+$/u);
    expect(Array.from(result.text)).toHaveLength(limits.textFieldCodePoints);
    expect(result.truncated).toBe(true);
  });

  it('preserva o erro final disponível com um cabeçalho que excede o orçamento', () => {
    const result = project([entry('llm', 'npm test ' + 'x'.repeat(2000), 'teste ok\n'.repeat(300) + 'ERRO: módulo ausente')]);
    expect(result.text).toMatch(/^… truncado\n\$ npm test x+…\n…/);
    expect(result.text).toMatch(/\nERRO: módulo ausente$/);
    expect(Array.from(result.text)).toHaveLength(limits.textFieldCodePoints);
  });

  it('preserva a marca original quando o histórico conserva apenas o prefixo', () => {
    const marker = '[TRUNCADO: output original tinha 70000 bytes]';
    const result = project([entry('llm', 'npm test', 'x'.repeat(50 * 1024) + '\n\n' + marker)]);
    expect(result.text).toMatch(/^… truncado\n\$ npm test\n…x+/);
    expect(result.text.endsWith('\n\n' + marker)).toBe(true);
    const crowded = project([entry('llm', 'antigo', 'x'.repeat(50 * 1024) + '\n\n' + marker),
      entry('llm', 'novo', 'x'.repeat(limits.textFieldCodePoints - 60))]);
    expect(crowded.text).toMatch(/^… truncado\n\$ novo\nx+$/);
  });

  it('omite o comando anterior se o espaço restante não identifica seu cabeçalho', () => {
    const result = project([entry('llm', 'anterior', 'erro'), entry('llm', 'atual', 'x'.repeat(876))]);
    expect(result.text).toMatch(/^… truncado\n\$ atual\nx+$/);
  });
});
