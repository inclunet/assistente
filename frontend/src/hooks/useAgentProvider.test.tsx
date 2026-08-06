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

    await waitFor(() => expect(result.current.isAgent).toBe(true));
    expect(result.current.resolved).toBe(true);
  });

  it('provedor http não é agente', async () => {
    const { result } = renderHook(() => useAgentProvider('openai'));

    await waitFor(() => expect(result.current.resolved).toBe(true));
    expect(result.current.isAgent).toBe(false);
  });

  // Antes da lista chegar a resposta é "não é agente" por não se saber, e
  // `resolved` é o que separa isso de uma resposta.
  it('enquanto a lista não chega, nada está resolvido', () => {
    providersSpy.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useAgentProvider('cursor'));

    expect(result.current).toEqual({ isAgent: false, resolved: false });
  });

  // O perfil pode apontar para "o padrão", e quem é o padrão pode ser um
  // agente.
  it('resolve o provedor padrão do app', async () => {
    providersSpy.mockResolvedValue([
      { id: 'openai', api_format: 'openai' },
      { id: 'cursor', api_format: 'acp', is_default: true },
    ]);

    const { result } = renderHook(() => useAgentProvider('$default'));

    await waitFor(() => expect(result.current.isAgent).toBe(true));
  });

  it('provedor que não está na lista não é agente', async () => {
    const { result } = renderHook(() => useAgentProvider('sumiu'));

    await waitFor(() => expect(result.current.resolved).toBe(true));
    expect(result.current.isAgent).toBe(false);
  });

  it('sem provedor escolhido não é agente', async () => {
    const { result } = renderHook(() => useAgentProvider(''));

    await waitFor(() => expect(result.current.resolved).toBe(true));
    expect(result.current.isAgent).toBe(false);
  });

  // Esconder guia por causa de uma consulta que falhou tiraria da pessoa
  // configuração que ela tem.
  it('consulta que falha responde que não é agente', async () => {
    providersSpy.mockRejectedValue(new Error('sem resposta'));

    const { result } = renderHook(() => useAgentProvider('cursor'));

    await waitFor(() => expect(result.current.resolved).toBe(true));
    expect(result.current.isAgent).toBe(false);
  });

  // Quem é o padrão se decide em outra tela. O editor é um diálogo que some ao
  // fechar, então a consulta de cada abertura já traz a lista de agora.
  it('abrir de novo pergunta de novo quem é o padrão', async () => {
    const primeiro = renderHook(() => useAgentProvider('$default'));
    await waitFor(() => expect(primeiro.result.current.resolved).toBe(true));
    expect(primeiro.result.current.isAgent).toBe(false);
    primeiro.unmount();

    providersSpy.mockResolvedValue([
      { id: 'openai', api_format: 'openai' },
      { id: 'cursor', api_format: 'acp', is_default: true },
    ]);

    const segundo = renderHook(() => useAgentProvider('$default'));

    await waitFor(() => expect(segundo.result.current.isAgent).toBe(true));
  });

  // Trocar de provedor no formulário responde na hora, com a lista que já veio:
  // não há instante em que a tela ainda esconde o que o provedor novo usa.
  it('trocar de provedor troca a resposta no mesmo render', async () => {
    const { result, rerender } = renderHook(({ id }) => useAgentProvider(id), {
      initialProps: { id: 'cursor' },
    });
    await waitFor(() => expect(result.current.isAgent).toBe(true));
    providersSpy.mockClear();

    rerender({ id: 'openai' });

    expect(result.current.isAgent).toBe(false);
    expect(providersSpy).not.toHaveBeenCalled();
  });
});
