/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
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
    useMCPStore.setState({ servers: [], isLoading: false, activeServerSlug: null, editingConfig: null });
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
