/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { mcp } from '@wailsjs/go/models';
import { ConnectMCPServer, SaveMCPServer, SaveMCPServerWithOAuthSecret, SaveMCPServerWithCredential } from '@wailsjs/go/wailsapi/MCP';
import { useMCPStore } from './mcpStore';

const mockListMCPServers = vi.fn();
const mockReauthorizeMCPServer = vi.fn();

vi.mock('@wailsjs/go/wailsapi/MCP', () => ({
  ListMCPServers: () => mockListMCPServers(),
  ConnectMCPServer: vi.fn(),
  DisconnectMCPServer: vi.fn(),
  ReconnectMCPServer: vi.fn(),
  ReauthorizeMCPServer: (slug: string) => mockReauthorizeMCPServer(slug),
  SaveMCPServer: vi.fn(),
  SaveMCPServerWithOAuthSecret: vi.fn(),
  SaveMCPServerWithCredential: vi.fn(),
  DeleteMCPServer: vi.fn(),
  GetMCPServerTools: vi.fn(),
  GetMCPServerConfig: vi.fn(),
}));

// Registro dos handlers de evento para simular emissões do backend.
const eventHandlers: Record<string, Array<(...args: unknown[]) => void>> = {};

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, handler: (...args: unknown[]) => void) => {
    (eventHandlers[event] ??= []).push(handler);
    return () => {
      eventHandlers[event] = (eventHandlers[event] || []).filter((h) => h !== handler);
    };
  },
}));

function emit(event: string) {
  (eventHandlers[event] || []).forEach((h) => h());
}

describe('mcpStore reautorização', () => {
  beforeEach(() => {
    mockListMCPServers.mockReset();
    mockReauthorizeMCPServer.mockReset();
    for (const key of Object.keys(eventHandlers)) delete eventHandlers[key];
    mockListMCPServers.mockResolvedValue([]);
    useMCPStore.setState({ servers: [], isLoading: false, loadRevision: 0, activeServerSlug: null, editingConfig: null });
  });

  it('reautoriza um servidor chamando o binding e recarregando a lista', async () => {
    mockReauthorizeMCPServer.mockResolvedValue(undefined);

    await useMCPStore.getState().reauthorize('atlassian');

    expect(mockReauthorizeMCPServer).toHaveBeenCalledWith('atlassian');
    expect(mockListMCPServers).toHaveBeenCalled();
  });

  it('propaga o erro e ainda recarrega a lista quando a reautorização falha', async () => {
    mockReauthorizeMCPServer.mockRejectedValue(new Error('fluxo cancelado'));

    await expect(useMCPStore.getState().reauthorize('atlassian')).rejects.toThrow('fluxo cancelado');
    expect(mockListMCPServers).toHaveBeenCalled();
  });

  it('recarrega os servidores quando o backend sinaliza needs_reauth e reauthorized', () => {
    const cleanup = useMCPStore.getState().setupEventListeners();

    mockListMCPServers.mockClear();
    emit('mcp:server_needs_reauth');
    emit('mcp:server_reauthorized');

    expect(mockListMCPServers).toHaveBeenCalledTimes(2);
    cleanup();
  });
});

it('salva configuração e segredo gerenciados pela mesma operação e propaga rollback', async () => {
 const config = new mcp.ServerConfig({oauth_managed: true});
 vi.mocked(SaveMCPServer).mockClear();
 vi.mocked(SaveMCPServerWithOAuthSecret).mockResolvedValueOnce(undefined);
 await useMCPStore.getState().save('managed', config, 'secret');
 expect(SaveMCPServerWithOAuthSecret).toHaveBeenCalledWith('managed',config,'secret');
 expect(SaveMCPServer).not.toHaveBeenCalled();
 vi.mocked(SaveMCPServerWithOAuthSecret).mockRejectedValueOnce(new Error('atomic failure'));
 await expect(useMCPStore.getState().save('managed',config,'new')).rejects.toThrow('atomic failure');
 expect(SaveMCPServer).not.toHaveBeenCalled();
});

it('propaga recusa OAuth ao handler e recarrega o estado após conectar', async () => {
 mockListMCPServers.mockResolvedValue([]);
 vi.mocked(ConnectMCPServer).mockRejectedValueOnce(new Error('oauth_consent_declined'));
 await expect(useMCPStore.getState().connect('managed')).rejects.toThrow('oauth_consent_declined');
 expect(mockListMCPServers).toHaveBeenCalled();
});

it('commits an edited credential and MCP config through one operation without fallback',async()=>{
 const cfg={slug:'test',auth_type:'bearer',transport:'streamable',url:'https://example.com'} as Parameters<typeof SaveMCPServer>[1];
 const input={pattern:'example.com',source:'static',type:'bearer',token:'secret'} as Parameters<typeof SaveMCPServerWithCredential>[2];
 vi.mocked(SaveMCPServer).mockClear();
 vi.mocked(SaveMCPServerWithCredential).mockResolvedValueOnce(undefined);
 await useMCPStore.getState().save('test',cfg,undefined,input);
 expect(SaveMCPServerWithCredential).toHaveBeenCalledWith('test',cfg,input);
 expect(SaveMCPServer).not.toHaveBeenCalled();
 vi.mocked(SaveMCPServerWithCredential).mockRejectedValueOnce(new Error('vault unavailable'));
 await expect(useMCPStore.getState().save('test',cfg,undefined,input)).rejects.toThrow('vault unavailable');
 expect(SaveMCPServer).not.toHaveBeenCalled();
});


it('só publica revisão de carga após sucesso e mantém falha recuperável por retry', async () => {
  useMCPStore.setState({ servers: [], isLoading: false, loadRevision: 0 });
  mockListMCPServers.mockRejectedValueOnce(new Error('list unavailable'));
  await useMCPStore.getState().loadServers();
  expect(useMCPStore.getState().loadRevision).toBe(0);
  expect(useMCPStore.getState().isLoading).toBe(false);
  mockListMCPServers.mockResolvedValueOnce([]);
  await useMCPStore.getState().loadServers();
  expect(useMCPStore.getState().loadRevision).toBe(1);
  expect(useMCPStore.getState().servers).toEqual([]);
});


it.each(['success', 'failure'])('uma carga antiga com %s não libera prontidão enquanto a nova está pendente', async (outcome) => {
  useMCPStore.setState({ servers: [], isLoading: false, loadRevision: 0 });
  let resolveOld!: (servers: mcp.ServerInfo[]) => void;
  let rejectOld!: (reason: Error) => void;
  let resolveCurrent!: (servers: mcp.ServerInfo[]) => void;
  mockListMCPServers.mockReturnValueOnce(new Promise<mcp.ServerInfo[]>((resolve, reject) => { resolveOld = resolve; rejectOld = reject; }));
  mockListMCPServers.mockReturnValueOnce(new Promise<mcp.ServerInfo[]>((resolve) => { resolveCurrent = resolve; }));
  const oldLoad = useMCPStore.getState().loadServers();
  const currentLoad = useMCPStore.getState().loadServers();
  if (outcome === 'success') resolveOld([]); else rejectOld(new Error('old request'));
  await oldLoad;
  expect(useMCPStore.getState().isLoading).toBe(true);
  expect(useMCPStore.getState().loadRevision).toBe(0);
  const current = [new mcp.ServerInfo({ slug: 'current', name: 'Current' })];
  resolveCurrent(current);
  await currentLoad;
  expect(useMCPStore.getState().servers).toEqual(current);
  expect(useMCPStore.getState().isLoading).toBe(false);
  expect(useMCPStore.getState().loadRevision).toBe(1);
});

it('resposta antiga após o sucesso atual não sobrescreve servidores nem publica outra revisão', async () => {
  useMCPStore.setState({ servers: [], isLoading: false, loadRevision: 0 });
  let finishOld!: (servers: mcp.ServerInfo[]) => void;
  mockListMCPServers.mockReturnValueOnce(new Promise<mcp.ServerInfo[]>((resolve) => { finishOld = resolve; }));
  const current = [new mcp.ServerInfo({ slug: 'current', name: 'Current' })];
  mockListMCPServers.mockResolvedValueOnce(current);
  const oldLoad = useMCPStore.getState().loadServers();
  await useMCPStore.getState().loadServers();
  finishOld([]);
  await oldLoad;
  expect(useMCPStore.getState().servers).toEqual(current);
  expect(useMCPStore.getState().loadRevision).toBe(1);
});
