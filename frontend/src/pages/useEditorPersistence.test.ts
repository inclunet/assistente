import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { hashStringFNV1a32, type DiskInfo } from '../lib/editorMergeUtils';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { EditorGetFileInfo, EditorReadFile, EditorWatchFile } from '@wailsjs/go/app/App';
import type { EditorFileChangedEvent } from './editorTypes';
import { useEditorPersistence } from './useEditorPersistence';
import type { UseEditorMergeResult } from './useEditorMerge';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const addToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (s: { addToast: typeof addToast }) => unknown) => selector({ addToast }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  EditorGetFileInfo: vi.fn(),
  EditorReadFile: vi.fn(),
  EditorUnwatchFile: vi.fn().mockResolvedValue(undefined),
  EditorWatchFile: vi.fn().mockResolvedValue(undefined),
  EditorWriteDraft: vi.fn().mockResolvedValue(undefined),
  EditorWriteFile: vi.fn().mockResolvedValue(undefined),
}));

let fileChangedHandler: ((data: EditorFileChangedEvent) => void | Promise<void>) | null = null;
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((name: string, handler: (data: EditorFileChangedEvent) => void | Promise<void>) => {
    if (name === 'editor:fileChanged') {
      fileChangedHandler = handler;
    }
    return vi.fn();
  }),
}));

const promptResolveExternalChangeForTab = vi.fn();

function makeDoc(patch: Partial<EditorDocument> = {}): EditorDocument {
  return {
    id: 'tab-1',
    title: 'Doc',
    markdown: 'conteudo original',
    mode: 'markdown',
    filePath: 'C:/tmp/doc.md',
    draftId: null,
    isDirty: false,
    ...patch,
  };
}

function makeDiskInfo(size: number, modTimeMs: number): DiskInfo {
  return { exists: true, isDir: false, size, modTimeMs };
}

function makeMerge(initialContent: string, initialInfo: DiskInfo): UseEditorMergeResult {
  const latestMarkdownByTabRef: { current: Record<string, string> } = { current: { 'tab-1': initialContent } };
  const diskInfoByTabRef: { current: Record<string, DiskInfo> } = { current: { 'tab-1': initialInfo } };
  const diskContentHashByTabRef: { current: Record<string, number> } = {
    current: { 'tab-1': hashStringFNV1a32(initialContent) },
  };
  const externalConflictLockedByTab: Record<string, boolean> = {};

  return {
    mergeStateRevision: 0,
    latestMarkdownByTabRef,
    diskInfoByTabRef,
    diskContentHashByTabRef,
    mergeSessionByTabRef: { current: {} },
    getMergeSession: vi.fn(() => null),
    markSelfWrite: vi.fn(),
    isProbablySelfWrite: vi.fn(() => false),
    updateLatestMarkdownForTab: vi.fn((tabId: string, markdown: string) => {
      latestMarkdownByTabRef.current[tabId] = markdown;
    }),
    getCachedMarkdownForTab: vi.fn((tab: EditorDocument) => latestMarkdownByTabRef.current[tab.id] ?? tab.markdown),
    setExternalConflictLocked: vi.fn((tabId: string, locked: boolean) => {
      externalConflictLockedByTab[tabId] = locked;
    }),
    isExternalConflictLocked: vi.fn((tabId: string) => !!externalConflictLockedByTab[tabId]),
    isExternalPromptInFlight: vi.fn(() => false),
    setExternalPromptInFlight: vi.fn(),
    refreshDiskInfoForTab: vi.fn(async (tab: EditorDocument) => {
      const info = makeDiskInfoFromBackend(await EditorGetFileInfo(String(tab.filePath)));
      diskInfoByTabRef.current[tab.id] = info;
      return info;
    }),
    setDiskBaselineForTab: vi.fn((tabId: string, content: string) => {
      diskContentHashByTabRef.current[tabId] = hashStringFNV1a32(content);
    }),
    startMergeSessionForTab: vi.fn(),
    cleanupMergeSessionForTab: vi.fn(),
    promptResolveExternalChangeForTab,
  } as unknown as UseEditorMergeResult;
}

function makeDiskInfoFromBackend(info: unknown): DiskInfo {
  const raw = info as { exists?: boolean; isDir?: boolean; size?: number; modTimeMs?: number };
  return {
    exists: !!raw?.exists,
    isDir: !!raw?.isDir,
    size: Number(raw?.size ?? 0),
    modTimeMs: Number(raw?.modTimeMs ?? 0),
  };
}

function renderPersistence(
  doc: EditorDocument,
  merge: UseEditorMergeResult,
  flushActiveRichMarkdownNow = vi.fn(),
  opts: { allDocs?: EditorDocument[]; currentDocumentId?: string } = {}
) {
  const allDocs = opts.allDocs ?? [doc];
  useEditorStore.getState().hydrate({ documents: Object.fromEntries(allDocs.map((d) => [d.id, d])) });

  return renderHook(() =>
    useEditorPersistence({
      merge,
      sessionLoaded: true,
      currentDocumentId: opts.currentDocumentId ?? doc.id,
      allDocs,
      flushActiveRichMarkdownNow,
      saveEditorState: vi.fn(),
    })
  );
}

describe('useEditorPersistence', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    fileChangedHandler = null;
    useEditorStore.getState().hydrate({ documents: {} });
    vi.mocked(EditorGetFileInfo).mockResolvedValue({
      exists: true,
      isDir: false,
      size: 20,
      modTimeMs: 2000,
    } as never);
  });

  it('sincroniza escrita assistida via editor:fileChanged sem abrir conflito falso', async () => {
    const doc = makeDoc({ markdown: 'antes', isDirty: false });
    const merge = makeMerge('antes', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('depois' as never);

    renderPersistence(doc, merge);
    await waitFor(() => expect(EditorWatchFile).toHaveBeenCalledWith('C:/tmp/doc.md'));
    expect(fileChangedHandler).toBeTruthy();

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md' });
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('depois');
    expect(useEditorStore.getState().documents['tab-1'].isDirty).toBe(false);
    expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
  });

  it('sincroniza tool edit_file assistida mesmo quando a aba está dirty mas ainda está no baseline', async () => {
    const doc = makeDoc({ markdown: 'antes da tool', isDirty: true });
    const merge = makeMerge('antes da tool', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    renderPersistence(doc, merge);
    await waitFor(() => expect(EditorWatchFile).toHaveBeenCalledWith('C:/tmp/doc.md'));

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md', origin: 'assistant_tool', assisted: true });
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('depois da tool');
    expect(useEditorStore.getState().documents['tab-1'].isDirty).toBe(false);
    expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
  });

  it('sincroniza escrita assistida explicitamente antes do watcher tardio', async () => {
    const doc = makeDoc({ markdown: 'antes da tool', isDirty: true });
    const merge = makeMerge('antes da tool', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    const { result } = renderPersistence(doc, merge);
    await waitFor(() => expect(EditorWatchFile).toHaveBeenCalledWith('C:/tmp/doc.md'));

    await act(async () => {
      await result.current.syncAssistedChangeForTab('tab-1');
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('depois da tool');
    expect(useEditorStore.getState().documents['tab-1'].isDirty).toBe(false);
    expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
  });

  it('não descarta evento assistido dentro da janela de self-write do editor', async () => {
    const doc = makeDoc({ markdown: 'antes da tool', isDirty: true });
    const merge = makeMerge('antes da tool', makeDiskInfo(5, 1000));
    vi.mocked(merge.isProbablySelfWrite).mockReturnValue(true);
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    renderPersistence(doc, merge);

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md', origin: 'assistant_tool', assisted: true });
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('depois da tool');
    expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
  });

  it('faz flush do editor rico antes de processar evento assistido', async () => {
    const doc = makeDoc({ markdown: 'antes da tool', mode: 'rich', isDirty: true });
    const merge = makeMerge('antes da tool', makeDiskInfo(5, 1000));
    const flushActiveRichMarkdownNow = vi.fn();
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    renderPersistence(doc, merge, flushActiveRichMarkdownNow);

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md', origin: 'assistant_tool', assisted: true });
    });

    expect(flushActiveRichMarkdownNow).toHaveBeenCalled();
    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('depois da tool');
  });

  it('não faz flush do editor rico ativo quando evento assistido é de outro arquivo', async () => {
    const activeDoc = makeDoc({ id: 'tab-1', filePath: 'C:/tmp/active.md', mode: 'rich', markdown: 'ativo' });
    const affectedDoc = makeDoc({ id: 'tab-2', filePath: 'C:/tmp/other.md', markdown: 'antes da tool', isDirty: false });
    const merge = makeMerge('ativo', makeDiskInfo(5, 1000));
    merge.latestMarkdownByTabRef.current['tab-2'] = 'antes da tool';
    merge.diskContentHashByTabRef.current['tab-2'] = hashStringFNV1a32('antes da tool');
    const flushActiveRichMarkdownNow = vi.fn();
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    renderPersistence(activeDoc, merge, flushActiveRichMarkdownNow, {
      allDocs: [activeDoc, affectedDoc],
      currentDocumentId: 'tab-1',
    });

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/other.md', origin: 'assistant_tool', assisted: true });
    });

    expect(flushActiveRichMarkdownNow).not.toHaveBeenCalled();
    expect(useEditorStore.getState().documents['tab-2'].markdown).toBe('depois da tool');
  });

  it('mantém prompt para tool assistida quando há edição local divergente do baseline', async () => {
    const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
    const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
    merge.diskContentHashByTabRef.current['tab-1'] = hashStringFNV1a32('baseline antes da tool');
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    renderPersistence(doc, merge);

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md', origin: 'assistant_tool', assisted: true });
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('minha edicao local');
    expect(useEditorStore.getState().documents['tab-1'].isDirty).toBe(true);
    expect(promptResolveExternalChangeForTab).toHaveBeenCalledWith('tab-1', 'C:/tmp/doc.md', {
      diskContent: 'depois da tool',
      diskReadError: '',
    });
  });

  it('não pergunta conflito no foco quando disco e editor já têm o mesmo conteúdo', async () => {
    const doc = makeDoc({ markdown: 'conteudo assistido', isDirty: true });
    const merge = makeMerge('conteudo assistido', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('conteudo assistido' as never);

    renderPersistence(doc, merge);

    await act(async () => {
      window.dispatchEvent(new Event('focus'));
    });

    await waitFor(() => expect(EditorReadFile).toHaveBeenCalledWith('C:/tmp/doc.md'));
    expect(useEditorStore.getState().documents['tab-1'].isDirty).toBe(false);
    expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
  });

  it('não sobrescreve conteúdo local divergente no recheck de foco', async () => {
    const doc = makeDoc({ markdown: 'edicao local ainda nao salva', isDirty: false });
    const merge = makeMerge('edicao local ainda nao salva', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('mudanca externa no disco' as never);

    renderPersistence(doc, merge);

    await act(async () => {
      window.dispatchEvent(new Event('focus'));
    });

    await waitFor(() => expect(promptResolveExternalChangeForTab).toHaveBeenCalled());
    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('edicao local ainda nao salva');
    expect(promptResolveExternalChangeForTab).toHaveBeenCalledWith('tab-1', 'C:/tmp/doc.md', {
      diskContent: 'mudanca externa no disco',
      diskReadError: '',
    });
  });

  it('mantém prompt quando mudança externa diverge de edição local dirty', async () => {
    const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
    const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('mudanca externa' as never);

    renderPersistence(doc, merge);
    expect(fileChangedHandler).toBeTruthy();

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md' });
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('minha edicao local');
    expect(useEditorStore.getState().documents['tab-1'].isDirty).toBe(true);
    expect(promptResolveExternalChangeForTab).toHaveBeenCalledWith('tab-1', 'C:/tmp/doc.md', {
      diskContent: 'mudanca externa',
      diskReadError: '',
    });
  });
});
