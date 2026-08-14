import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import {
  dirName,
  pendingRecreate,
  useAgentConversationWorkDir,
  type AgentWorkDirState,
} from './useAgentConversationWorkDir';

const getWorkDir = vi.fn();
const setWorkDir = vi.fn();

vi.mock('@wailsjs/go/wailsapi/ACPWorkDir', () => ({
  GetAgentConversationWorkDir: (id: string) => getWorkDir(id),
  SetAgentConversationWorkDir: (id: string, dir: string) => setWorkDir(id, dir),
}));

const estado = (over: Partial<AgentWorkDirState> = {}): AgentWorkDirState => ({
  conversationId: 'conversa-1',
  available: true,
  dir: '/casa/ana/projeto',
  workspaceDir: '/casa/ana/projeto',
  pinned: false,
  ...over,
});

beforeEach(() => {
  getWorkDir.mockReset();
  setWorkDir.mockReset();
});

describe('useAgentConversationWorkDir', () => {
  it('lê o diretório da conversa que está na tela', async () => {
    getWorkDir.mockResolvedValue(estado());
    const { result } = renderHook(() => useAgentConversationWorkDir('conversa-1'));

    await waitFor(() => expect(result.current.state?.dir).toBe('/casa/ana/projeto'));
    expect(getWorkDir).toHaveBeenCalledWith('conversa-1');
  });

  // Trocar de conversa troca o diretório mostrado: a escolha é de cada conversa,
  // e deixar o anterior na barra diria que o agente age numa árvore que não é a
  // dela.
  it('troca o diretório quando a conversa muda', async () => {
    getWorkDir.mockImplementation((id: string) => Promise.resolve(
      estado({ conversationId: id, dir: id === 'conversa-1' ? '/um' : '/dois' }),
    ));
    const { result, rerender } = renderHook(
      ({ id }: { id: string }) => useAgentConversationWorkDir(id),
      { initialProps: { id: 'conversa-1' } },
    );

    await waitFor(() => expect(result.current.state?.dir).toBe('/um'));
    rerender({ id: 'conversa-2' });
    await waitFor(() => expect(result.current.state?.dir).toBe('/dois'));
  });

  // Sem conversa não há diretório a mostrar, e perguntar por uma que não existe
  // só geraria erro no console de quem ainda não abriu conversa nenhuma.
  it('não pergunta nada sem conversa', async () => {
    const { result } = renderHook(() => useAgentConversationWorkDir(null));

    await waitFor(() => expect(result.current.state).toBeNull());
    expect(getWorkDir).not.toHaveBeenCalled();
  });
});

describe('pendingRecreate', () => {
  // Sem sessão de pé não há o que recriar: a conversa que ainda não falou com o
  // agente não perde memória nenhuma ao escolher diretório.
  it('não há recriação pendente sem sessão de pé', () => {
    expect(pendingRecreate(estado({ sessionDir: '' }))).toBe(false);
    expect(pendingRecreate(null)).toBe(false);
  });

  it('há recriação pendente quando a sessão de pé é de outro diretório', () => {
    expect(pendingRecreate(estado({ dir: '/novo', sessionDir: '/velho' }))).toBe(true);
  });

  // A barra no fim do caminho não é troca de diretório, e anunciá-la como uma
  // prometeria uma perda de memória que não vai acontecer.
  it('a barra no fim não conta como diretório diferente', () => {
    expect(pendingRecreate(estado({ dir: '/casa/ana/projeto', sessionDir: '/casa/ana/projeto/' }))).toBe(false);
  });

  // No Windows a caixa das letras não distingue diretórios, e o backend compara
  // do mesmo jeito: divergir aqui faria a tela e a sessão discordarem.
  it('a caixa das letras não distingue caminhos do Windows', () => {
    expect(pendingRecreate(estado({ dir: 'C:\\Casa\\Ana', sessionDir: 'c:\\casa\\ana' }))).toBe(false);
    expect(pendingRecreate(estado({ dir: '/casa/Ana', sessionDir: '/casa/ana' }))).toBe(true);
  });
});

describe('dirName', () => {
  it('mostra a última pasta do caminho', () => {
    expect(dirName('/casa/ana/projeto')).toBe('projeto');
    expect(dirName('C:\\casa\\ana\\projeto\\')).toBe('projeto');
    expect(dirName('')).toBe('');
  });
});
