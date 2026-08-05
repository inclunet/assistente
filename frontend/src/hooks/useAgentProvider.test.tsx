import { describe, expect, it, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useAgentProvider } from './useAgentProvider';

const providersSpy = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetLLMProvidersWithStatus: () => providersSpy(),
}));

beforeEach(() => {
  providersSpy.mockReset();
  providersSpy.mockResolvedValue([
    { id: 'openai', api_format: 'openai', is_default: true },
    { id: 'cursor', api_format: 'acp' },
  ]);
});

describe('useAgentProvider', () => {
  it('reconhece o provedor de agente', async () => {
    const { result } = renderHook(() => useAgentProvider('cursor'));

    await waitFor(() => expect(result.current).toBe(true));
  });

  it('provedor http não é agente', async () => {
    const { result } = renderHook(() => useAgentProvider('openai'));

    await waitFor(() => expect(providersSpy).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  // O perfil pode apontar para "o padrão", e quem é o padrão pode ser um
  // agente.
  it('resolve o provedor padrão do app', async () => {
    providersSpy.mockResolvedValue([
      { id: 'openai', api_format: 'openai' },
      { id: 'cursor', api_format: 'acp', is_default: true },
    ]);

    const { result } = renderHook(() => useAgentProvider('$default'));

    await waitFor(() => expect(result.current).toBe(true));
  });

  it('provedor que não está na lista não é agente', async () => {
    const { result } = renderHook(() => useAgentProvider('sumiu'));

    await waitFor(() => expect(providersSpy).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it('sem provedor escolhido não consulta nada', () => {
    const { result } = renderHook(() => useAgentProvider(''));

    expect(providersSpy).not.toHaveBeenCalled();
    expect(result.current).toBe(false);
  });

  // Esconder guia por causa de uma consulta que falhou tiraria da pessoa
  // configuração que ela tem.
  it('consulta que falha responde que não é agente', async () => {
    providersSpy.mockRejectedValue(new Error('sem resposta'));

    const { result } = renderHook(() => useAgentProvider('cursor'));

    await waitFor(() => expect(providersSpy).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it('trocar de provedor troca a resposta', async () => {
    const { result, rerender } = renderHook(({ id }) => useAgentProvider(id), {
      initialProps: { id: 'cursor' },
    });
    await waitFor(() => expect(result.current).toBe(true));

    rerender({ id: 'openai' });

    await waitFor(() => expect(result.current).toBe(false));
  });
});
