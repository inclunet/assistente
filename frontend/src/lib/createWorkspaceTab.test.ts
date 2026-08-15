import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createWorkspaceTab } from './createWorkspaceTab';

const mocks = vi.hoisted(() => ({
  addTab: vi.fn(),
  createSession: vi.fn(),
  loadSessions: vi.fn(),
  closeSession: vi.fn(),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => ({ addTab: mocks.addTab }),
  },
}));

vi.mock('../store/terminalStore', () => ({
  useTerminalStore: {
    getState: () => ({
      createSession: mocks.createSession,
      loadSessions: mocks.loadSessions,
      closeSession: mocks.closeSession,
    }),
  },
}));

describe('createWorkspaceTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.addTab.mockResolvedValue('tab-1');
    mocks.createSession.mockResolvedValue('session-1');
    mocks.loadSessions.mockResolvedValue(true);
    mocks.closeSession.mockResolvedValue(true);
  });

  it('cria e conecta uma sessão ao abrir nova aba de terminal', async () => {
    await expect(createWorkspaceTab('terminal', 'Terminal')).resolves.toBe('tab-1');

    expect(mocks.createSession).toHaveBeenCalledOnce();
    expect(mocks.loadSessions).toHaveBeenCalledOnce();
    expect(mocks.addTab).toHaveBeenCalledWith('terminal', 'Terminal', {
      sessionId: 'session-1',
    });
  });

  it('não cria aba desconectada quando a sessão não pode ser confirmada', async () => {
    mocks.loadSessions.mockResolvedValue(false);

    await expect(createWorkspaceTab('terminal', 'Terminal')).rejects.toThrow();

    expect(mocks.addTab).not.toHaveBeenCalled();
    expect(mocks.closeSession).toHaveBeenCalledWith('session-1');
  });

  it('encerra a sessão se a persistência da aba falhar', async () => {
    mocks.addTab.mockRejectedValue(new Error('falha ao salvar aba'));

    await expect(createWorkspaceTab('terminal', 'Terminal')).rejects.toThrow(
      'falha ao salvar aba',
    );

    expect(mocks.closeSession).toHaveBeenCalledWith('session-1');
  });

  it('mantém criação simples para outros tipos de aba', async () => {
    await createWorkspaceTab('editor', 'Editor');

    expect(mocks.createSession).not.toHaveBeenCalled();
    expect(mocks.addTab).toHaveBeenCalledWith('editor', 'Editor');
  });
});
