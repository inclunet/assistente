import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { EditorGetFileInfo, EditorReadFile, EditorWriteDraft } from '@wailsjs/go/app/App';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const addToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (s: { addToast: typeof addToast }) => unknown) => selector({ addToast }),
}));

const requestQuestionnaire = vi.fn();
vi.mock('../store/questionnaireUIStore', () => ({
  useQuestionnaireUIStore: (selector: (s: { request: typeof requestQuestionnaire }) => unknown) =>
    selector({ request: requestQuestionnaire }),
}));

const editorStoreState = {
  documents: {} as Record<string, unknown>,
  setDocMarkdown: vi.fn(),
  setDocFilePath: vi.fn(),
  setDocDraftId: vi.fn(),
  setDocDirty: vi.fn(),
  renameDocument: vi.fn(),
};
vi.mock('../store/editorStore', () => ({
  useEditorStore: Object.assign(
    (selector: (s: typeof editorStoreState) => unknown) => selector(editorStoreState),
    { getState: () => editorStoreState }
  ),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  EditorDeleteDraft: vi.fn().mockResolvedValue(undefined),
  EditorGetFileInfo: vi.fn(),
  EditorReadFile: vi.fn(),
  EditorSaveFileDialog: vi.fn(),
  EditorWriteDraft: vi.fn().mockResolvedValue(undefined),
  EditorWriteFile: vi.fn().mockResolvedValue(undefined),
}));

import { useEditorMerge } from './useEditorMerge';

function setup() {
  return renderHook(() => useEditorMerge());
}

const tab = { id: 't1', title: 'Doc', markdown: 'conteúdo original', mode: 'markdown' as const };

describe('useEditorMerge', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('marca e detecta auto-escritas recentes por caminho', () => {
    const { result } = setup();
    expect(result.current.markSelfWrite).toBeTypeOf('function');

    result.current.markSelfWrite('/tmp/doc.md');
    expect(result.current.isProbablySelfWrite('/tmp/doc.md')).toBe(true);
    expect(result.current.isProbablySelfWrite('/tmp/outro.md')).toBe(false);
  });

  it('controla o lock de conflito externo por aba', () => {
    const { result } = setup();
    expect(result.current.isExternalConflictLocked('t1')).toBe(false);

    act(() => {
      result.current.setExternalConflictLocked('t1', true);
    });
    expect(result.current.isExternalConflictLocked('t1')).toBe(true);

    act(() => {
      result.current.setExternalConflictLocked('t1', false);
    });
    expect(result.current.isExternalConflictLocked('t1')).toBe(false);
  });

  it('controla o flag de prompt em andamento por aba', () => {
    const { result } = setup();
    expect(result.current.isExternalPromptInFlight('t1')).toBe(false);
    result.current.setExternalPromptInFlight('t1', true);
    expect(result.current.isExternalPromptInFlight('t1')).toBe(true);
  });

  it('usa o markdown da aba como fallback e o cache quando atualizado', () => {
    const { result } = setup();
    expect(result.current.getCachedMarkdownForTab(tab)).toBe('conteúdo original');

    result.current.updateLatestMarkdownForTab('t1', 'conteúdo novo');
    expect(result.current.getCachedMarkdownForTab(tab)).toBe('conteúdo novo');
  });

  it('refreshDiskInfoForTab normaliza o retorno do backend', async () => {
    vi.mocked(EditorGetFileInfo).mockResolvedValue({
      path: '/tmp/doc.md',
      exists: true,
      isDir: false,
      size: 42,
      modTimeMs: 1000,
    } as never);

    const { result } = setup();
    const info = await result.current.refreshDiskInfoForTab({ ...tab, filePath: '/tmp/doc.md' });
    expect(info).toEqual({ exists: true, isDir: false, size: 42, modTimeMs: 1000 });
  });

  it('refresh atrasado não sobrescreve info mais nova (guarda monotônica)', async () => {
    const { result } = setup();
    const tabWithPath = { ...tab, filePath: '/tmp/doc.md' };

    let resolveStale: (v: unknown) => void = () => {};
    const staleStat = new Promise((r) => {
      resolveStale = r;
    });
    vi.mocked(EditorGetFileInfo)
      .mockImplementationOnce(() => staleStat as never)
      .mockResolvedValueOnce({ exists: true, isDir: false, size: 2, modTimeMs: 2000 } as never);

    // O refresh antigo fica pendente enquanto um mais novo completa primeiro.
    const stale = result.current.refreshDiskInfoForTab(tabWithPath);
    const fresh = result.current.refreshDiskInfoForTab(tabWithPath);
    await fresh;
    resolveStale({ exists: true, isDir: false, size: 1, modTimeMs: 1000 });
    const staleResult = await stale;

    const freshInfo = { exists: true, isDir: false, size: 2, modTimeMs: 2000 };
    expect(result.current.getDiskStateForTab('t1').info).toEqual(freshInfo);
    // O refresh descartado também devolve o info mais novo já aplicado, não o
    // stat antigo que perdeu a corrida.
    expect(staleResult).toEqual(freshInfo);
  });

  it('setDiskInfoForTab invalida refresh em voo iniciado antes', async () => {
    const { result } = setup();
    const tabWithPath = { ...tab, filePath: '/tmp/doc.md' };

    let resolveStale: (v: unknown) => void = () => {};
    const staleStat = new Promise((r) => {
      resolveStale = r;
    });
    vi.mocked(EditorGetFileInfo).mockImplementationOnce(() => staleStat as never);

    const stale = result.current.refreshDiskInfoForTab(tabWithPath);
    const fresh = { exists: true, isDir: false, size: 9, modTimeMs: 9000 };
    result.current.setDiskInfoForTab('t1', fresh);
    resolveStale({ exists: true, isDir: false, size: 1, modTimeMs: 1000 });
    const staleResult = await stale;

    expect(result.current.getDiskStateForTab('t1').info).toEqual(fresh);
    expect(staleResult).toEqual(fresh);
  });

  it('startMergeSessionForTab grava os três drafts e registra a sessão', async () => {
    const { result } = setup();
    await act(async () => {
      await result.current.startMergeSessionForTab('t1', '/tmp/doc.md', 'disco', 'minha');
    });

    expect(EditorWriteDraft).toHaveBeenCalledTimes(3);
    expect(result.current.getMergeSession('t1')).not.toBeNull();
    expect(result.current.isExternalConflictLocked('t1')).toBe(true);
    expect(editorStoreState.setDocMarkdown).toHaveBeenCalled();
  });

  it('mergeStateRevision muda quando o lock externo muda (e não muda se o valor for igual)', () => {
    const { result } = setup();
    const initial = result.current.mergeStateRevision;

    act(() => {
      result.current.setExternalConflictLocked('t1', true);
    });
    expect(result.current.mergeStateRevision).not.toBe(initial);

    const afterLock = result.current.mergeStateRevision;
    act(() => {
      result.current.setExternalConflictLocked('t1', true);
    });
    expect(result.current.mergeStateRevision).toBe(afterLock);

    act(() => {
      result.current.setExternalConflictLocked('t1', false);
    });
    expect(result.current.mergeStateRevision).not.toBe(afterLock);
  });

  describe('promptResolveExternalChangeForTab', () => {
    const docWithPath = { id: 't1', title: 'Doc', markdown: 'local', mode: 'markdown', filePath: '/tmp/doc.md' };

    beforeEach(() => {
      editorStoreState.documents = { t1: docWithPath };
      vi.mocked(EditorGetFileInfo).mockResolvedValue({
        exists: true,
        isDir: false,
        size: 10,
        modTimeMs: 1000,
      } as never);
    });

    it('usa "Manter minha versão" como opção padrão do questionário', async () => {
      requestQuestionnaire.mockResolvedValue({ cancelled: true });
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo externo' as never);

      const { result } = setup();
      await act(async () => {
        await result.current.promptResolveExternalChangeForTab('t1', '/tmp/doc.md', {
          diskContent: 'conteudo externo',
        });
      });

      const questions = requestQuestionnaire.mock.calls[0][0].questions as Array<{ id: string; default?: string }>;
      const choice = questions.find((q) => q.id === 'choice');
      expect(choice?.default).toBe('editor.options.useMine');
    });

    it('ao cancelar com disco ainda divergente, mantém o lock e avisa que o autosave está pausado', async () => {
      requestQuestionnaire.mockResolvedValue({ cancelled: true });
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo externo' as never);

      const { result } = setup();
      await act(async () => {
        await result.current.promptResolveExternalChangeForTab('t1', '/tmp/doc.md', {
          diskContent: 'conteudo externo',
        });
      });

      expect(result.current.isExternalConflictLocked('t1')).toBe(true);
      expect(addToast).toHaveBeenCalledWith('editor.toast.externalChange', 'warning');
    });

    it('ao cancelar com disco já igual ao local, desfaz o lock em vez de matar o autosave', async () => {
      requestQuestionnaire.mockResolvedValue({ cancelled: true });
      const { result } = setup();
      act(() => {
        result.current.updateLatestMarkdownForTab('t1', 'conteudo convergido');
      });
      // O prompt abre com divergência, mas na re-checagem do cancelamento o
      // disco já convergiu com o local.
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo convergido' as never);

      await act(async () => {
        await result.current.promptResolveExternalChangeForTab('t1', '/tmp/doc.md', {
          diskContent: 'conteudo externo',
        });
      });

      expect(result.current.isExternalConflictLocked('t1')).toBe(false);
      expect(editorStoreState.setDocDirty).toHaveBeenCalledWith('t1', false);
      expect(addToast).not.toHaveBeenCalledWith('editor.toast.externalChange', 'warning');
    });
  });

  it('mergeStateRevision muda ao iniciar e ao limpar a merge session', async () => {
    const { result } = setup();
    const initial = result.current.mergeStateRevision;

    await act(async () => {
      await result.current.startMergeSessionForTab('t1', '/tmp/doc.md', 'disco', 'minha');
    });
    const afterStart = result.current.mergeStateRevision;
    expect(afterStart).not.toBe(initial);

    await act(async () => {
      await result.current.cleanupMergeSessionForTab('t1');
    });
    expect(result.current.mergeStateRevision).not.toBe(afterStart);
  });
});
