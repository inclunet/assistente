import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useEditorFileActions } from './useEditorFileActions';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceData, type WorkspaceTab } from '../store/workspaceStore';
import type { UseEditorMergeResult } from './useEditorMerge';

const wails = vi.hoisted(() => ({ open: vi.fn(), write: vi.fn(), saveDialog: vi.fn(), deleteDraft: vi.fn(), readDraft: vi.fn(), addWorkspaceTab: vi.fn(), setActiveWorkspaceTab: vi.fn(), toast: vi.fn(), confirm: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../store/uiStore', () => ({ useUIStore: (selector: (s: { addToast: typeof wails.toast }) => unknown) => selector({ addToast: wails.toast }) }));
vi.mock('../store/confirmStore', () => ({ requestConfirm: (...args: unknown[]) => wails.confirm(...args) }));
vi.mock('@wailsjs/go/wailsapi/Editor', () => ({ EditorOpenFile: (...a: unknown[]) => wails.open(...a), EditorWriteFile: (...a: unknown[]) => wails.write(...a), EditorSaveFileDialog: (...a: unknown[]) => wails.saveDialog(...a), EditorDeleteDraft: (...a: unknown[]) => wails.deleteDraft(...a), EditorReadDraft: (...a: unknown[]) => wails.readDraft(...a) }));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn(), ListWorkspaces: vi.fn(), CreateWorkspace: vi.fn(), SwitchWorkspace: vi.fn(), RenameWorkspace: vi.fn(), DeleteWorkspace: vi.fn(), SetWorkspaceProfile: vi.fn(), AddWorkspaceTab: (...a: unknown[]) => wails.addWorkspaceTab(...a), RemoveWorkspaceTab: vi.fn(), UpdateWorkspaceTab: vi.fn(), ReorderWorkspaceTabs: vi.fn(), MoveWorkspaceTabTo: vi.fn(), ExportWorkspace: vi.fn(), ImportWorkspace: vi.fn() }));
vi.mock('@wailsjs/go/app/App', () => ({ CreateAdminUser: vi.fn(), GetAuthStatus: vi.fn(), Login: vi.fn(), Logout: vi.fn(), RefreshAuth: vi.fn(), SetupVault: vi.fn(), UnlockVault: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn(() => vi.fn()) }));
vi.mock('../lib/workspaceNavigationWails', () => ({ setActiveWorkspaceTabForWorkspace: (...a: unknown[]) => wails.setActiveWorkspaceTab(...a) }));
vi.mock('../../wailsjs/go/models', () => ({ workspace: { Tab: class { constructor(data: Record<string, unknown>) { Object.assign(this, data); } } } }));

function mergeFixture(cache?: (doc: EditorDocument) => string) {
  return {
    getMergeSession: vi.fn(() => null), getCachedMarkdownForTab: vi.fn(cache || ((doc: EditorDocument) => doc.markdown)),
    updateLatestMarkdownForTab: vi.fn(), markSelfWrite: vi.fn(), isExternalConflictLocked: vi.fn(() => false),
    setExternalConflictLocked: vi.fn(), setDiskBaselineForTab: vi.fn(), refreshDiskInfoForTab: vi.fn(),
    cleanupMergeSessionForTab: vi.fn(async () => undefined), promptResolveExternalChangeForTab: vi.fn(),
  } as unknown as UseEditorMergeResult;
}
const baseDoc = (p: Partial<EditorDocument> = {}): EditorDocument => ({ id: 'doc-a', title: 'Doc', markdown: 'original', mode: 'markdown', filePath: 'C:/doc.md', draftId: 'draft-a', isDirty: true, readOnly: false, ...p });
const tab = (id: string, title = id): WorkspaceTab => ({ id, type: 'editor', title, position: 0 });
const baseWorkspace = (activeTabId = 'doc-a'): WorkspaceData => ({ id: 'ws-a', name: 'Workspace', activeTabId, tabs: [tab('doc-a')] });

function installState(documents: Record<string, EditorDocument> = { 'doc-a': baseDoc() }, ws = baseWorkspace()) {
  useAuthStore.setState({ user: { userId: 'owner-a', sessionId: 'session-a', role: 'user' }, isAuthenticated: true, status: null, error: null, isLoading: false });
  useEditorStore.setState({ ownerUserId: 'owner-a', documents });
  useWorkspaceStore.setState({ workspace: ws, workspaces: [], isInitialized: true });
  wails.addWorkspaceTab.mockImplementation(async (bt: { id: string; title: string; type: string; position: number }) => {
    const current = useWorkspaceStore.getState().workspace!;
    useWorkspaceStore.setState({ workspace: { ...current, tabs: [...current.tabs, { id: bt.id, type: bt.type as WorkspaceTab['type'], title: bt.title, position: bt.position }], activeTabId: bt.id } });
    return undefined;
  });
}
function renderActions(merge = mergeFixture(), focusEditorSoon = vi.fn(), flushActiveRichMarkdownNow = vi.fn()) {
  return renderHook(() => {
    const activeTabId = useWorkspaceStore((s) => s.workspace?.activeTabId);
    const documents = useEditorStore((s) => s.documents);
    const activeTab = activeTabId ? documents[activeTabId] || null : null;
    return useEditorFileActions({ merge, activeTab, flushActiveRichMarkdownNow, focusEditorSoon });
  });
}

beforeEach(() => {
  installState();
  wails.open.mockReset(); wails.write.mockReset(); wails.saveDialog.mockReset(); wails.deleteDraft.mockReset(); wails.readDraft.mockReset(); wails.toast.mockReset(); wails.confirm.mockReset(); wails.setActiveWorkspaceTab.mockResolvedValue(undefined);
  wails.open.mockResolvedValue({ path: 'C:/opened.md', content: 'opened', readOnly: false, projected: false }); wails.write.mockResolvedValue(undefined); wails.saveDialog.mockResolvedValue(''); wails.deleteDraft.mockResolvedValue(undefined);
  wails.confirm.mockResolvedValue(true);
  document.body.innerHTML = '<button id="editor-focus">editor</button>'; (document.getElementById('editor-focus') as HTMLElement).focus();
});
afterEach(() => { document.body.innerHTML = ''; vi.clearAllMocks(); });

describe('useEditorFileActions com stores Zustand reais', () => {
  it('prepara bloqueio externo sem sessão sem abrir segundo diálogo', async () => {
    const merge = mergeFixture();
    (merge.isExternalConflictLocked as unknown as ReturnType<typeof vi.fn>).mockReturnValue(true);
    const { result } = renderActions(merge);
    let prepared;
    await act(async () => { prepared = await result.current.prepareFileCommand('editor.file.save'); });
    expect(prepared).toBeUndefined();
    expect(wails.confirm).not.toHaveBeenCalled();
    expect(wails.toast).toHaveBeenCalledWith('editor.toast.saveLockedExternal', 'warning');
  });
  it('usa confirmação renderer para overwrite', async () => {
    const { result } = renderActions();
    await expect(result.current.confirmOverwrite('C:/target.md')).resolves.toBe(true);
    expect(wails.confirm).toHaveBeenCalledWith(expect.objectContaining({
      message: 'app.questionnaire.editConfirmation.overwriteDescription',
      confirmText: 'app.questionnaire.editConfirmation.overwriteConfirm',
      cancelText: 'app.questionnaire.editConfirmation.overwriteCancel',
    }));
  });
  it('não limpa dirty nem substitui documento se resultado chegar após mudança', async () => {
    const merge = mergeFixture();
    const { result } = renderActions(merge);
    await act(async () => { await result.current.prepareFileCommand('editor.file.save'); });
    act(() => { useEditorStore.getState().setDocMarkdown('doc-a', 'newer'); });
    act(() => { result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/doc.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a']).toMatchObject({ markdown: 'newer', isDirty: true });
    expect(merge.setDiskBaselineForTab).toHaveBeenCalledWith('doc-a', 'original');
    expect(useEditorStore.getState().documents['doc-a'].draftId).toBe('draft-a');
    expect(wails.deleteDraft).not.toHaveBeenCalled();
  });
  it('não aplica conteúdo open em documento vivo', async () => {
    const { result } = renderActions();
    await act(async () => { await result.current.prepareFileCommand('editor.file.open'); });
    act(() => { result.current.applyCommittedFileCommand('editor.file.open', { tabId: 'doc-a', path: 'C:/other.md', written: true, opened: { path: 'C:/other.md', content: 'remote' } }); });
    expect(useEditorStore.getState().documents['doc-a'].markdown).toBe('original');
  });
  it('save_copy não altera a origem após commit', async () => {
    const merge = mergeFixture();
    const { result } = renderActions(merge);
    await act(async () => { await result.current.prepareFileCommand('editor.file.save_copy'); });
    act(() => { result.current.applyCommittedFileCommand('editor.file.save_copy', { tabId: 'doc-a', path: 'C:/copy.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a']).toMatchObject({ filePath: 'C:/doc.md', markdown: 'original', isDirty: true, draftId: 'draft-a' });
    expect(merge.setDiskBaselineForTab).not.toHaveBeenCalled();
    expect(merge.setExternalConflictLocked).not.toHaveBeenCalled();
    expect(wails.deleteDraft).not.toHaveBeenCalled();
  });
  it('save unchanged limpa dirty e draft somente após resultado escrito', async () => {
    const { result } = renderActions();
    await act(async () => { await result.current.prepareFileCommand('editor.file.save'); });
    act(() => { result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/doc.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a']).toMatchObject({ isDirty: false, draftId: null });
    expect(wails.deleteDraft).toHaveBeenCalledWith('draft-a');
    expect(wails.toast).toHaveBeenCalledWith('editor.toast.fileSaved', 'success');
  });
  it('primeiro save associa o path retornado e remove draft quando não houve edição', async () => {
    installState({ 'doc-a': baseDoc({ filePath: null }) });
    const { result } = renderActions();
    await act(async () => { await result.current.prepareFileCommand('editor.file.save'); });
    act(() => { result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/first.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a']).toMatchObject({ filePath: 'C:/first.md', title: 'first.md', isDirty: false, draftId: null });
  });
  it('aceita path já aplicado pelo evento antes do resultado', async () => {
    installState({ 'doc-a': baseDoc({ filePath: null }) });
    const { result } = renderActions();
    await act(async () => { await result.current.prepareFileCommand('editor.file.save'); });
    act(() => { useEditorStore.getState().setDocFilePath('doc-a', 'C:/first.md'); });
    act(() => { result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/first.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a']).toMatchObject({ filePath: 'C:/first.md', isDirty: false });
  });
  it('recusa preparação de save para documento readonly', async () => {
    installState({ 'doc-a': baseDoc({ readOnly: true }) });
    const { result } = renderActions();
    await expect(result.current.prepareFileCommand('editor.file.save')).resolves.toBeUndefined();
  });
  it('faz flush rich e envia o cache atual, não o markdown atrasado do store', async () => {
    const flush = vi.fn();
    const merge = mergeFixture(() => 'rich-cache');
    installState({ 'doc-a': baseDoc({ mode: 'rich', markdown: 'store-old' }) });
    const { result } = renderActions(merge, vi.fn(), flush);
    const request = await result.current.prepareFileCommand('editor.file.save');
    expect(flush).toHaveBeenCalledTimes(1);
    expect(request?.content).toBe('rich-cache');
  });
  it('não aplica resultado depois de logout ou troca de workspace', async () => {
    const { result } = renderActions();
    await act(async () => { await result.current.prepareFileCommand('editor.file.save'); });
    useAuthStore.setState({ isAuthenticated: false, user: null });
    act(() => { result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/doc.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a'].isDirty).toBe(true);

    installState();
    const second = renderActions();
    await act(async () => { await second.result.current.prepareFileCommand('editor.file.save'); });
    useWorkspaceStore.setState({ workspace: { ...baseWorkspace(), id: 'ws-other' } });
    act(() => { second.result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/doc.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a'].isDirty).toBe(true);
  });
  it('recusa markers de merge antes de abrir confirmação', async () => {
    const merge = mergeFixture(() => '<<<<<<< ours\nconflict\n>>>>>>> theirs');
    const { result } = renderActions(merge);
    await expect(result.current.prepareFileCommand('editor.file.save')).resolves.toBeUndefined();
    expect(wails.confirm).not.toHaveBeenCalled();
    expect(wails.toast).toHaveBeenCalledWith('editor.toast.conflictMarkersRemain', 'warning');
  });
  it('preserva lock e dirty se uma nova sessão de merge surgiu durante a escrita', async () => {
    const merge = mergeFixture();
    const firstSession = { id: 'merge-1' };
    const newSession = { id: 'merge-2' };
    (merge.getMergeSession as unknown as ReturnType<typeof vi.fn>).mockReturnValue(firstSession);
    installState();
    const { result } = renderActions(merge);
    await act(async () => { await result.current.prepareFileCommand('editor.file.save'); });
    (merge.getMergeSession as unknown as ReturnType<typeof vi.fn>).mockReturnValue(newSession);
    act(() => { result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/doc.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a'].isDirty).toBe(true);
    expect(merge.cleanupMergeSessionForTab).not.toHaveBeenCalled();
    expect(merge.setDiskBaselineForTab).not.toHaveBeenCalled();
  });
  it('não transforma falha de cleanup em falha do save', async () => {
    const merge = mergeFixture();
    const mergeSession = { id: 'merge-cleanup' };
    (merge.getMergeSession as unknown as ReturnType<typeof vi.fn>).mockReturnValue(mergeSession);
    (merge.cleanupMergeSessionForTab as unknown as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('cleanup failed'));
    const { result } = renderActions(merge);
    await act(async () => { await result.current.prepareFileCommand('editor.file.save'); });
    act(() => { result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/doc.md', written: true }); });
    expect(useEditorStore.getState().documents['doc-a'].isDirty).toBe(false);
    expect(merge.cleanupMergeSessionForTab).toHaveBeenCalledWith('doc-a');
  });
  it('primeiro save atualiza título/caminho atomicamente mesmo com nova edição', async () => {
    installState({ 'doc-a': baseDoc({ filePath: null }) });
    const { result } = renderActions();
    await act(async () => { await result.current.prepareFileCommand('editor.file.save'); });
    act(() => useEditorStore.getState().setDocMarkdown('doc-a', 'newer'));
    const observations: string[] = [];
    const unsubscribe = useEditorStore.subscribe((state) => {
      const doc = state.documents['doc-a'];
      observations.push(`${doc.filePath}:${doc.title}`);
    });
    act(() => result.current.applyCommittedFileCommand('editor.file.save', { tabId: 'doc-a', path: 'C:/first.md', written: true }));
    unsubscribe();
    expect(observations).toEqual(['C:/first.md:first.md']);
    expect(useEditorStore.getState().documents['doc-a']).toMatchObject({ markdown: 'newer', isDirty: true, draftId: 'draft-a' });
  });
});
