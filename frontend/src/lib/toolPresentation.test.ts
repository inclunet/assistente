import { describe, expect, it } from 'vitest';
import { presentTool } from './toolPresentation';

describe('presentTool', () => {
  it('apresenta uma ferramenta nativa de arquivo sem expor o caminho inteiro', () => {
    const presentation = presentTool('edit_file', 'builtin', undefined, '{"path":"C:\\\\projetos\\\\demo\\\\arquivo.txt"}');

    expect(presentation.labelKey).toBe('chat.toolEditFile');
    expect(presentation.target).toEqual({ kind: 'file', path: 'C:\\projetos\\demo\\arquivo.txt', label: 'arquivo.txt' });
  });

  it('mantém MCP no fallback por provedor, sem inferir o nome da ferramenta', () => {
    const presentation = presentTool('crm_consultar_cliente', 'mcp_native', 'CRM Exemplo', '{"cliente":"Ana"}');

    expect(presentation).toEqual({ labelKey: 'chat.toolMcpProvider', labelValues: { provider: 'CRM Exemplo' } });
  });

  it('aceita somente URLs http(s) como alvo acionável', () => {
    expect(presentTool('web_fetch', 'builtin', undefined, '{"url":"https://docs.exemplo.com/guia"}').target)
      .toEqual({ kind: 'url', url: 'https://docs.exemplo.com/guia', label: 'docs.exemplo.com' });
    expect(presentTool('web_fetch', 'builtin', undefined, '{"url":"file:///segredo"}').target).toBeUndefined();
  });
});
