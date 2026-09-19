import { describe, expect, it } from 'vitest';
import { formatToolPresentation, presentTool } from './toolPresentation';

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

  it('não expõe o termo técnico MCP quando o provedor não tem nome público', () => {
    expect(presentTool('crm_consultar_cliente', 'mcp_native')).toEqual({ labelKey: 'chat.toolMcpIntegration' });
  });

  it('aceita somente URLs http(s) como alvo acionável', () => {
    expect(presentTool('web_fetch', 'builtin', undefined, '{"url":"https://docs.exemplo.com/guia"}').target)
      .toEqual({ kind: 'url', url: 'https://docs.exemplo.com/guia', label: 'docs.exemplo.com' });
    expect(presentTool('web_fetch', 'builtin', undefined, '{"url":"file:///segredo"}').target).toBeUndefined();
  });

  it('não transforma escopos de busca e listagem em links de arquivo', () => {
    const args = '{"path":"C:\\\\repo\\\\src"}';

    expect(presentTool('search_files', 'builtin', undefined, args).target).toBeUndefined();
    expect(presentTool('list_directory', 'builtin', undefined, args).target).toBeUndefined();
  });

  it('formata a mesma frase amigável usada pelo card e pelo leitor de telas', () => {
    const presentation = presentTool('read_file', 'builtin', undefined, '{"path":"C:\\\\repo\\\\segredo\\\\arquivo.txt"}');
    const translated = formatToolPresentation(presentation, (key) => key === 'chat.toolReadFile' ? 'Lendo arquivo' : key);

    expect(translated).toBe('Lendo arquivo: arquivo.txt');
    expect(translated).not.toContain('C:\\repo');
  });

  it('não expõe o nome técnico de uma ferramenta desconhecida', () => {
    const presentation = presentTool('crm_internal_lookup_v2', 'builtin');

    expect(presentation).toEqual({ labelKey: 'chat.toolGeneric' });
    expect(formatToolPresentation(presentation, () => 'Executando ferramenta')).toBe('Executando ferramenta');
  });
});
