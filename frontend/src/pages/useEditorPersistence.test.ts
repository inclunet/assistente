import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { hashStringFNV1a32, type DiskInfo } from '../lib/editorMergeUtils';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { EditorGetFileInfo, EditorReadFile, EditorWatchFile, EditorWriteFile } from '@wailsjs/go/app/App';
import type { EditorFileChangedEvent } from './editorTypes';
import type { TabDiskState } from './editorReconciler';
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
  const diskStateByTabRef: { current: Record<string, TabDiskState> } = {
    current: {
      'tab-1': { info: initialInfo, baselineHash: hashStringFNV1a32(initialContent), baselineContent: initialContent },
    },
  };
  const externalConflictLockedByTab: Record<string, boolean> = {};

  const ensureDiskState = (tabId: string): TabDiskState => {
    if (!diskStateByTabRef.current[tabId]) {
      diskStateByTabRef.current[tabId] = { info: null, baselineHash: null, baselineContent: null };
    }
    return diskStateByTabRef.current[tabId];
  };

  return {
    mergeStateRevision: 0,
    latestMarkdownByTabRef,
    diskStateByTabRef,
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
    getDiskStateForTab: vi.fn(
      (tabId: string) => diskStateByTabRef.current[tabId] ?? { info: null, baselineHash: null, baselineContent: null }
    ),
    setDiskInfoForTab: vi.fn((tabId: string, info: DiskInfo | null) => {
      ensureDiskState(tabId).info = info;
    }),
    refreshDiskInfoForTab: vi.fn(async (tab: EditorDocument) => {
      const info = makeDiskInfoFromBackend(await EditorGetFileInfo(String(tab.filePath)));
      ensureDiskState(tab.id).info = info;
      return info;
    }),
    setDiskBaselineForTab: vi.fn((tabId: string, content: string) => {
      const state = ensureDiskState(tabId);
      state.baselineHash = hashStringFNV1a32(content);
      state.baselineContent = content;
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

  it('evento selfWrite do editor apenas atualiza baseline de disco, sem reload nem prompt', async () => {
    const doc = makeDoc({ markdown: 'conteudo local', isDirty: true });
    const merge = makeMerge('conteudo local', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('conteudo em disco' as never);

    renderPersistence(doc, merge);
    await waitFor(() => expect(EditorWatchFile).toHaveBeenCalledWith('C:/tmp/doc.md'));

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md', origin: 'editor_ui', selfWrite: true });
    });

    // Não recarrega conteúdo, não reconcilia e não abre prompt.
    expect(EditorReadFile).not.toHaveBeenCalled();
    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('conteudo local');
    expect(useEditorStore.getState().documents['tab-1'].isDirty).toBe(true);
    expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();

    // Apenas o baseline de disco (size+mtime) da aba é atualizado.
    expect(merge.refreshDiskInfoForTab).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(merge.diskStateByTabRef.current['tab-1'].info).toEqual(makeDiskInfo(20, 2000)));
  });

  it('trata origin editor_ui sem flag selfWrite da mesma forma', async () => {
    const doc = makeDoc({ markdown: 'conteudo local', isDirty: false });
    const merge = makeMerge('conteudo local', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('conteudo em disco' as never);

    renderPersistence(doc, merge);
    await waitFor(() => expect(EditorWatchFile).toHaveBeenCalledWith('C:/tmp/doc.md'));

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md', origin: 'editor_ui' });
    });

    expect(EditorReadFile).not.toHaveBeenCalled();
    expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
    expect(merge.refreshDiskInfoForTab).toHaveBeenCalledTimes(1);
  });

  it('evento sem origin ainda usa o fallback isProbablySelfWrite', async () => {
    const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
    const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
    vi.mocked(merge.isProbablySelfWrite).mockReturnValue(true);
    vi.mocked(EditorReadFile).mockResolvedValue('mudanca externa' as never);

    renderPersistence(doc, merge);

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md' });
    });

    // Suprimido pelo fallback: nada é lido nem reconciliado.
    expect(EditorReadFile).not.toHaveBeenCalled();
    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('minha edicao local');
    expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
  });

  it('atualiza o editor aberto com escrita assistida de outra aba quando não há divergência local', async () => {
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
    expect(merge.latestMarkdownByTabRef.current['tab-1']).toBe('depois da tool');
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
    merge.diskStateByTabRef.current['tab-2'] = {
      info: null,
      baselineHash: hashStringFNV1a32('antes da tool'),
      baselineContent: 'antes da tool',
    };
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
    merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('baseline antes da tool');
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    renderPersistence(doc, merge);

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md', origin: 'assistant_tool', assisted: true });
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('minha edicao local');
    expect(useEditorStore.getState().documents['tab-1'].isDirty).toBe(true);
    // O prompt carrega a causa assistida: a mensagem exibida é a de "alteração
    // do assistente", não a de "arquivo mudou fora do Assistente".
    expect(promptResolveExternalChangeForTab).toHaveBeenCalledWith('tab-1', 'C:/tmp/doc.md', {
      diskContent: 'depois da tool',
      diskReadError: '',
      cause: 'assisted',
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
      cause: 'external',
    });
  });

  describe('pré-autosave (persistTabContentNow)', () => {
    it('mtime tocado sem mudança de conteúdo não abre prompt nem trava autosave, e grava normalmente', async () => {
      // Digitação local em andamento: local diverge do baseline, mas o disco
      // ainda tem o baseline — só o mtime/size foi tocado (ex.: OneDrive).
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('conteudo original');
      merge.diskStateByTabRef.current['tab-1'].baselineContent = 'conteudo original';
      // beforeEach: EditorGetFileInfo devolve size 20/mtime 2000 → metadados divergem.
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo original' as never);

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
      expect(merge.setExternalConflictLocked).not.toHaveBeenCalledWith('tab-1', true);
      expect(addToast).not.toHaveBeenCalled();
      expect(EditorWriteFile).toHaveBeenCalledWith('C:/tmp/doc.md', 'minha edicao local');
    });

    it('disco já igual ao local: grava sem prompt e adota o local como baseline', async () => {
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('conteudo original');
      merge.diskStateByTabRef.current['tab-1'].baselineContent = 'conteudo original';
      vi.mocked(EditorReadFile).mockResolvedValue('minha edicao local' as never);

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
      expect(merge.setDiskBaselineForTab).toHaveBeenCalledWith('tab-1', 'minha edicao local');
      expect(EditorWriteFile).toHaveBeenCalledWith('C:/tmp/doc.md', 'minha edicao local');
    });

    it('divergência real de conteúdo abre prompt, trava e não grava', async () => {
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('conteudo original');
      merge.diskStateByTabRef.current['tab-1'].baselineContent = 'conteudo original';
      vi.mocked(EditorReadFile).mockResolvedValue('mudanca externa no disco' as never);

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(merge.setExternalConflictLocked).toHaveBeenCalledWith('tab-1', true);
      expect(addToast).toHaveBeenCalledWith('editor.toast.fileModified', 'warning');
      // Sem diskContent: o prompt relê o disco ao abrir (silent-resolve se o
      // arquivo convergiu de volta nesse meio tempo).
      expect(promptResolveExternalChangeForTab).toHaveBeenCalledWith('tab-1', 'C:/tmp/doc.md', undefined);
      expect(EditorWriteFile).not.toHaveBeenCalled();
    });

    it('lock adquirido durante a leitura do defer_read aborta a gravação', async () => {
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('conteudo original');
      merge.diskStateByTabRef.current['tab-1'].baselineContent = 'conteudo original';
      // O conteúdo lido seria benigno (== baseline), mas durante o await um
      // editor:fileChanged trava a aba por conflito real.
      vi.mocked(EditorReadFile).mockImplementation(async () => {
        merge.setExternalConflictLocked('tab-1', true);
        return 'conteudo original' as never;
      });

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(EditorWriteFile).not.toHaveBeenCalled();
      expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
    });

    it('revalidação pré-gravação aborta quando o conteúdo do disco muda entre a leitura e a gravação', async () => {
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('conteudo original');
      merge.diskStateByTabRef.current['tab-1'].baselineContent = 'conteudo original';
      // Primeira leitura vê o baseline; a re-leitura da revalidação encontra
      // conteúdo novo gravado por fora durante a janela do defer_read.
      vi.mocked(EditorReadFile)
        .mockResolvedValueOnce('conteudo original' as never)
        .mockResolvedValueOnce('mudanca externa gravada no meio' as never);
      // Stat pré-decisão e re-stat pré-gravação divergem.
      vi.mocked(EditorGetFileInfo)
        .mockResolvedValueOnce({ exists: true, isDir: false, size: 20, modTimeMs: 2000 } as never)
        .mockResolvedValueOnce({ exists: true, isDir: false, size: 30, modTimeMs: 3000 } as never);

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(EditorWriteFile).not.toHaveBeenCalled();
      expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
      // O abort acontece ANTES de mutar o estado: baseline/info antigos ficam
      // intactos para o próximo autosave re-comparar por conteúdo.
      expect(merge.setDiskInfoForTab).not.toHaveBeenCalled();
      expect(merge.setDiskBaselineForTab).not.toHaveBeenCalled();
    });

    it('revalidação com mtime flutuando mas conteúdo igual segue gravando e adota o stat novo', async () => {
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('conteudo original');
      merge.diskStateByTabRef.current['tab-1'].baselineContent = 'conteudo original';
      // O re-stat diverge (OneDrive tocou o mtime de novo), mas a re-leitura
      // confirma que o conteúdo é o mesmo já avaliado: não pode abortar em
      // loop, a gravação prossegue.
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo original' as never);
      vi.mocked(EditorGetFileInfo)
        .mockResolvedValueOnce({ exists: true, isDir: false, size: 20, modTimeMs: 2000 } as never)
        .mockResolvedValueOnce({ exists: true, isDir: false, size: 20, modTimeMs: 3000 } as never);

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
      expect(EditorWriteFile).toHaveBeenCalledWith('C:/tmp/doc.md', 'minha edicao local');
      expect(merge.setDiskInfoForTab).toHaveBeenCalledWith('tab-1', {
        exists: true,
        isDir: false,
        size: 20,
        modTimeMs: 3000,
      });
    });

    it('sem lastDisk conhecido, divergência real ainda é detectada por conteúdo (unknown != no_change)', async () => {
      // Primeiro autosave antes do refresh inicial de info completar: o estado
      // de disco da aba ainda não tem `info`, mas o arquivo existe no disco.
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      merge.diskStateByTabRef.current['tab-1'].info = null;
      merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('conteudo original');
      merge.diskStateByTabRef.current['tab-1'].baselineContent = 'conteudo original';
      vi.mocked(EditorReadFile).mockResolvedValue('mudanca externa no disco' as never);

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(promptResolveExternalChangeForTab).toHaveBeenCalledWith('tab-1', 'C:/tmp/doc.md', undefined);
      expect(EditorWriteFile).not.toHaveBeenCalled();
    });

    it('sem lastDisk conhecido e disco igual ao baseline, grava normalmente', async () => {
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      merge.diskStateByTabRef.current['tab-1'].info = null;
      merge.diskStateByTabRef.current['tab-1'].baselineHash = hashStringFNV1a32('conteudo original');
      merge.diskStateByTabRef.current['tab-1'].baselineContent = 'conteudo original';
      vi.mocked(EditorReadFile).mockResolvedValue('conteudo original' as never);

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
      expect(EditorWriteFile).toHaveBeenCalledWith('C:/tmp/doc.md', 'minha edicao local');
    });

    it('metadados iguais gravam sem ler o conteúdo do disco', async () => {
      const doc = makeDoc({ markdown: 'minha edicao local', isDirty: true });
      const merge = makeMerge('minha edicao local', makeDiskInfo(5, 1000));
      // Metadados idênticos ao último estado conhecido → nenhuma leitura extra.
      vi.mocked(EditorGetFileInfo).mockResolvedValue({
        exists: true,
        isDir: false,
        size: 5,
        modTimeMs: 1000,
      } as never);

      const { result } = renderPersistence(doc, merge);

      await act(async () => {
        await result.current.persistTabContentNow('tab-1');
      });

      expect(EditorReadFile).not.toHaveBeenCalled();
      expect(promptResolveExternalChangeForTab).not.toHaveBeenCalled();
      expect(EditorWriteFile).toHaveBeenCalledWith('C:/tmp/doc.md', 'minha edicao local');
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
      cause: 'external',
    });
  });

  it('anuncia reload assistido com toast específico de alteração do assistente', async () => {
    const doc = makeDoc({ markdown: 'antes da tool', isDirty: false });
    const merge = makeMerge('antes da tool', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    renderPersistence(doc, merge);

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md', origin: 'assistant_tool', assisted: true });
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('depois da tool');
    expect(addToast).toHaveBeenCalledWith('editor.toast.assistedReloaded', 'info');
  });

  it('anuncia reload de mudança externa com o toast de mudança externa', async () => {
    const doc = makeDoc({ markdown: 'conteudo original', isDirty: false });
    const merge = makeMerge('conteudo original', makeDiskInfo(5, 1000));
    vi.mocked(EditorReadFile).mockResolvedValue('mudanca externa' as never);

    renderPersistence(doc, merge);

    await act(async () => {
      await fileChangedHandler?.({ path: 'C:/tmp/doc.md' });
    });

    expect(useEditorStore.getState().documents['tab-1'].markdown).toBe('mudanca externa');
    expect(addToast).toHaveBeenCalledWith('editor.toast.externalReloaded', 'info');
  });
});
