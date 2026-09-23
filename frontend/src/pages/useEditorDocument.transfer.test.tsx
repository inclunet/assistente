import { cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useEditorDocument } from './useEditorDocument';
import type { UseEditorMergeResult } from './useEditorMerge';
import { useEditorStore } from '../store/editorStore';
import { useWorkspaceStore, type WorkspaceData } from '../store/workspaceStore';

const storage = vi.hoisted(() => ({ read: vi.fn(), load: vi.fn(), save: vi.fn(), file: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Editor', () => ({
  EditorLoadState: storage.load, EditorReadDraft: storage.read, EditorReadFile: storage.file,
  EditorSaveState: storage.save, EditorDeleteDraft: vi.fn(async () => {}), EditorGetDraftPath: vi.fn(async () => 'draft.md'),
}));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: () => () => {} }));
beforeEach(() => {
  storage.read.mockReset().mockResolvedValue(''); storage.load.mockReset().mockResolvedValue({});
  storage.save.mockReset().mockResolvedValue(undefined); storage.file.mockReset();
  useEditorStore.setState({ ownerUserId: 'owner', documents: {} });
  useWorkspaceStore.setState({ workspace: { id: 'workspace', name: 'Workspace', activeTabId: 'editor', tabs: [{ id: 'editor', type: 'editor', title: 'From chat', position: 0, state: { draftId: 'draft' } }] } as WorkspaceData });
});
afterEach(cleanup);
function mount() {
  const merge = {
    updateLatestMarkdownForTab: vi.fn(), setDiskBaselineForTab: vi.fn(), refreshDiskInfoForTab: vi.fn(),
    setExternalConflictLocked: vi.fn(), mergeSessionByTabRef: { current: {} },
  } as unknown as UseEditorMergeResult;
  return renderHook(() => useEditorDocument({ merge, isWsInitialized: true, currentDocumentId: 'editor', activeTab: null, allDocs: [], documents: {} }));
}
describe('hidratação dos rascunhos criados pela transferência chat → editor', () => {
  it.each(['', '**saved content**'])('restaura draft sem filePath e preserva conteúdo exato %j', async content => {
    storage.read.mockResolvedValue(content);
    const { result } = mount(); await waitFor(() => expect(result.current.sessionLoaded).toBe(true));
    expect(storage.read).toHaveBeenCalledExactlyOnceWith('draft'); expect(storage.file).not.toHaveBeenCalled();
    expect(useEditorStore.getState().documents.editor).toMatchObject({ markdown: content, draftId: 'draft', filePath: null, sessionHydrated: true, readOnly: false });
  });
  it('draft reservado ainda sem arquivo inicia vazio, sem texto de exemplo', async () => {
    storage.read.mockRejectedValue('draft não encontrado');
    const { result } = mount(); await waitFor(() => expect(result.current.sessionLoaded).toBe(true));
    expect(useEditorStore.getState().documents.editor).toMatchObject({ markdown: '', loadError: false, readOnly: false });
  });
  it('erro real de leitura não permite sobrescrever um draft como se fosse novo', async () => {
    storage.read.mockRejectedValue(new Error('access denied'));
    const { result } = mount(); await waitFor(() => expect(result.current.sessionLoaded).toBe(true));
    expect(useEditorStore.getState().documents.editor).toMatchObject({ markdown: '', loadError: true, readOnly: true, mode: 'view' });
  });
  it('hidratação não substitui edição viva enquanto a leitura aguarda', async () => {
    let resolve!: (value: string) => void;
    storage.read.mockImplementation(() => new Promise<string>(done => { resolve = done; }));
    const { result } = mount(); await waitFor(() => expect(storage.read).toHaveBeenCalledOnce());
    useEditorStore.getState().createDocument({ id: 'editor', markdown: 'newer edit', draftId: 'draft' });
    resolve('older disk'); await waitFor(() => expect(result.current.sessionLoaded).toBe(true));
    expect(useEditorStore.getState().documents.editor.markdown).toBe('newer edit');
  });
});
