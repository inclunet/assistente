import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { EditorGetFileInfo, EditorWriteDraft } from '@wailsjs/go/app/App';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const addToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (s: { addToast: typeof addToast }) => unknown) => selector({ addToast }),
}));

vi.mock('../store/questionnaireUIStore', () => ({
  useQuestionnaireUIStore: (selector: (s: { request: () => void }) => unknown) => selector({ request: vi.fn() }),
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
    await stale;

    expect(result.current.getDiskStateForTab('t1').info).toEqual({
      exists: true,
      isDir: false,
      size: 2,
      modTimeMs: 2000,
    });
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
    await stale;

    expect(result.current.getDiskStateForTab('t1').info).toEqual(fresh);
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
