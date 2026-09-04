import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { EditorGetFileInfo, EditorReadFile, EditorWriteDraft } from '@wailsjs/go/wailsapi/Editor';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const addToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (s: { addToast: typeof addToast }) => unknown) => selector({ addToast }),
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

vi.mock('@wailsjs/go/wailsapi/Editor', () => ({
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

  it('refresh com erro devolve o info atual quando a sequência foi invalidada durante o await', async () => {
    const { result } = setup();
    const tabWithPath = { ...tab, filePath: '/tmp/doc.md' };

    let rejectStale: (e: unknown) => void = () => {};
    const staleStat = new Promise((_resolve, reject) => {
      rejectStale = reject;
    });
    vi.mocked(EditorGetFileInfo).mockImplementationOnce(() => staleStat as never);

    const stale = result.current.refreshDiskInfoForTab(tabWithPath);
    const fresh = { exists: true, isDir: false, size: 9, modTimeMs: 9000 };
    result.current.setDiskInfoForTab('t1', fresh);
    rejectStale(new Error('stat falhou'));

    // Há uma verdade mais nova em memória: o chamador não deve abortar à toa.
    expect(await stale).toEqual(fresh);
    expect(result.current.getDiskStateForTab('t1').info).toEqual(fresh);
  });

  it('refresh com erro e sequência intacta continua devolvendo null', async () => {
    const { result } = setup();
    vi.mocked(EditorGetFileInfo).mockRejectedValueOnce(new Error('stat falhou'));

    expect(await result.current.refreshDiskInfoForTab({ ...tab, filePath: '/tmp/doc.md' })).toBeNull();
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

    const openDecision = async (
      result: ReturnType<typeof setup>['result'],
      opts?: { diskContent?: string; cause?: 'external' | 'assisted' },
    ) => {
      let pending!: Promise<void>;
      act(() => {
        pending = result.current.promptResolveExternalChangeForTab(
          't1',
          '/tmp/doc.md',
          opts,
        );
      });
      await waitFor(() => expect(result.current.externalChangeDecision).not.toBeNull());
      return { pending };
    };

    const choose = async (
      result: ReturnType<typeof setup>['result'],
      pending: Promise<void>,
      action: Parameters<typeof result.current.resolveExternalChangeDecision>[0],
    ) => {
      await act(async () => {
        result.current.resolveExternalChangeDecision(action);
        await pending;
      });
    };

    it('expõe opções diretas e mantém minha versão como foco seguro no host', async () => {
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo externo' as never);
      const { result } = setup();
      const { pending } = await openDecision(result, { diskContent: 'conteudo externo' });

      expect(result.current.externalChangeDecision).toMatchObject({
        title: 'editor.questionnaire.externalChangeTitle',
        description: 'editor.questionnaire.externalChangeDesc',
        labels: {
          useDisk: 'editor.options.useDisk',
          resolveMerge: 'editor.options.resolveMerge',
          useMine: 'editor.options.useMine',
          saveAs: 'editor.options.saveAs',
          notNow: 'editor.buttons.notNow',
        },
      });
      await choose(result, pending, 'not-now');
    });

    it('ao decidir depois com disco divergente, mantém lock e avisa sobre autosave', async () => {
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo externo' as never);
      const { result } = setup();
      const { pending } = await openDecision(result, { diskContent: 'conteudo externo' });
      await choose(result, pending, 'not-now');

      expect(result.current.isExternalConflictLocked('t1')).toBe(true);
      expect(addToast).toHaveBeenCalledWith('editor.toast.externalChange', 'warning');
    });

    it('não mostra mensagem crua quando a leitura do disco falha', async () => {
      vi.mocked(EditorReadFile).mockRejectedValue(new Error('falha ao acessar arquivo'));
      const { result } = setup();
      const { pending } = await openDecision(result);

      expect(result.current.externalChangeDecision?.diskPreview).toBe(
        'editor.errors.diskReadFailed',
      );
      expect(result.current.externalChangeDecision?.diskPreview).not.toContain(
        'falha ao acessar arquivo',
      );
      await choose(result, pending, 'not-now');
    });

    it('preserva causa assistida no diálogo e no aviso', async () => {
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo da tool' as never);
      const { result } = setup();
      const { pending } = await openDecision(result, {
        diskContent: 'conteudo da tool',
        cause: 'assisted',
      });

      expect(result.current.externalChangeDecision?.title).toBe(
        'editor.questionnaire.assistedChangeTitle',
      );
      await choose(result, pending, 'not-now');
      expect(addToast).toHaveBeenCalledWith('editor.toast.assistedChange', 'warning');
    });

    it('usar disco só substitui conteúdo após ação explícita', async () => {
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo externo' as never);
      const { result } = setup();
      const { pending } = await openDecision(result, { diskContent: 'conteudo externo' });

      expect(editorStoreState.setDocMarkdown).not.toHaveBeenCalled();
      await choose(result, pending, 'use-disk');
      expect(editorStoreState.setDocMarkdown).toHaveBeenCalledWith(
        't1',
        'conteudo externo',
      );
      expect(result.current.isExternalConflictLocked('t1')).toBe(false);
    });

    it('chamada com aba inexistente não abre decisão nem vaza causa', async () => {
      const { result } = setup();
      editorStoreState.documents = {};
      await act(async () => {
        await result.current.promptResolveExternalChangeForTab(
          't1',
          '/tmp/doc.md',
          { cause: 'assisted' },
        );
      });
      expect(result.current.externalChangeDecision).toBeNull();

      editorStoreState.documents = { t1: docWithPath };
      const { pending } = await openDecision(result, { diskContent: 'conteudo externo' });
      expect(result.current.externalChangeDecision?.title).toBe(
        'editor.questionnaire.externalChangeTitle',
      );
      await choose(result, pending, 'not-now');
    });

    it('ao decidir depois com disco convergido, destrava o autosave', async () => {
      const { result } = setup();
      act(() => {
        result.current.updateLatestMarkdownForTab('t1', 'conteudo convergido');
      });
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo convergido' as never);
      const { pending } = await openDecision(result, { diskContent: 'conteudo externo' });
      await choose(result, pending, 'not-now');

      expect(result.current.isExternalConflictLocked('t1')).toBe(false);
      expect(editorStoreState.setDocDirty).toHaveBeenCalledWith('t1', false);
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
