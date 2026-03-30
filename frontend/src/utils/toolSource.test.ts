import { describe, it, expect } from 'vitest';
import { parseToolSource, extractMcpServers } from './toolSource';

describe('parseToolSource', () => {
  it('identifica ferramenta local', () => {
    expect(parseToolSource('read_file')).toEqual({ type: 'local' });
  });

  it('identifica ferramenta MCP com serverSlug', () => {
    expect(parseToolSource('mcp_atlassian__search')).toEqual({ type: 'mcp', serverSlug: 'atlassian' });
  });

  it('identifica MCP com slug contendo hifens', () => {
    expect(parseToolSource('mcp_nu-mcp__get_issues')).toEqual({ type: 'mcp', serverSlug: 'nu-mcp' });
  });

  it('retorna local se prefixo mcp_ sem separador __', () => {
    expect(parseToolSource('mcp_broken')).toEqual({ type: 'local' });
  });

  it('retorna local para string vazia', () => {
    expect(parseToolSource('')).toEqual({ type: 'local' });
  });
});

describe('extractMcpServers', () => {
  it('extrai servidores MCP únicos dos nomes de tools', () => {
    const tools = ['local_tool', 'mcp_atlassian__search', 'mcp_atlassian__create', 'mcp_slack__send'];
    const result = extractMcpServers(tools);
    expect(result).toEqual([
      { slug: 'atlassian', name: 'atlassian' },
      { slug: 'slack', name: 'slack' },
    ]);
  });

  it('usa nomes amigáveis quando servers é fornecido', () => {
    const tools = ['mcp_atlassian__search', 'mcp_slack__send'];
    const servers = [
      { slug: 'atlassian', name: 'Atlassian Cloud' },
      { slug: 'slack', name: 'Slack MCP' },
    ];
    const result = extractMcpServers(tools, servers);
    expect(result).toEqual([
      { slug: 'atlassian', name: 'Atlassian Cloud' },
      { slug: 'slack', name: 'Slack MCP' },
    ]);
  });

  it('retorna array vazio sem ferramentas MCP', () => {
    expect(extractMcpServers(['tool1', 'tool2'])).toEqual([]);
  });

  it('ordena por slug', () => {
    const tools = ['mcp_zzz__a', 'mcp_aaa__b'];
    const result = extractMcpServers(tools);
    expect(result[0].slug).toBe('aaa');
    expect(result[1].slug).toBe('zzz');
  });
});
