import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { useAgentSessionCommands, type AgentSessionCommandsEvent } from './useAgentSessionCommands';

const getCommandsSpy = vi.fn();
const listeners = new Map<string, (data: unknown) => void>();

vi.mock('@wailsjs/go/wailsapi/ACPCommands', () => ({
  GetAgentSessionCommands: (conversationId: string) => getCommandsSpy(conversationId),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, handler: (data: unknown) => void) => {
    listeners.set(event, handler);
    return () => listeners.delete(event);
  },
}));

function anuncia(event: AgentSessionCommandsEvent) {
  act(() => {
    listeners.get('chat:agent_commands')?.(event);
  });
}

describe('useAgentSessionCommands', () => {
  beforeEach(() => {
    getCommandsSpy.mockReset();
    getCommandsSpy.mockResolvedValue({ conversationId: 'conversa-1', commands: [] });
    listeners.clear();
  });

  it('lê os comandos ao abrir, porque o agente os anuncia antes de alguém digitar a barra', async () => {
    getCommandsSpy.mockResolvedValue({
      conversationId: 'conversa-1',
      commands: [{ name: 'plan', description: 'Monta um plano', acceptsInput: true }],
    });

    const { result } = renderHook(() => useAgentSessionCommands('conversa-1'));

    await waitFor(() => expect(result.current).toHaveLength(1));
    expect(result.current[0].name).toBe('plan');
  });

  it('ignora o anúncio de outra conversa, que fala com outro agente', async () => {
    const { result } = renderHook(() => useAgentSessionCommands('conversa-1'));
    await waitFor(() => expect(getCommandsSpy).toHaveBeenCalledWith('conversa-1'));

    anuncia({ conversationId: 'conversa-2', commands: [{ name: 'plan', acceptsInput: false }] });

    expect(result.current).toHaveLength(0);
  });

  it('a lista nova substitui a anterior, inclusive quando vem vazia', async () => {
    getCommandsSpy.mockResolvedValue({
      conversationId: 'conversa-1',
      commands: [{ name: 'plan', acceptsInput: true }],
    });
    const { result } = renderHook(() => useAgentSessionCommands('conversa-1'));
    await waitFor(() => expect(result.current).toHaveLength(1));

    anuncia({ conversationId: 'conversa-1', commands: [] });

    expect(result.current).toHaveLength(0);
  });

  it('conversa que não fala com agente fica sem comandos em vez de quebrar', async () => {
    getCommandsSpy.mockRejectedValue(new Error('sem agente'));

    const { result } = renderHook(() => useAgentSessionCommands('conversa-1'));

    await waitFor(() => expect(getCommandsSpy).toHaveBeenCalled());
    expect(result.current).toHaveLength(0);
  });

  it('trocar de conversa não carrega os comandos da anterior', async () => {
    getCommandsSpy.mockResolvedValueOnce({
      conversationId: 'conversa-1',
      commands: [{ name: 'plan', acceptsInput: true }],
    });
    getCommandsSpy.mockResolvedValueOnce({ conversationId: 'conversa-2', commands: [] });

    const { result, rerender } = renderHook(
      ({ id }: { id: string }) => useAgentSessionCommands(id),
      { initialProps: { id: 'conversa-1' } },
    );
    await waitFor(() => expect(result.current).toHaveLength(1));

    rerender({ id: 'conversa-2' });

    await waitFor(() => expect(result.current).toHaveLength(0));
  });
});
