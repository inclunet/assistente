import { useEffect, useRef, useState, type RefObject } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { DataGrid, DATAGRID_ENTRY_SELECTOR, type GridFocusRequest } from '../ui/DataGrid';
import { ConfirmDialog } from '../ui/ConfirmDialog';
import { Toolbar, ToolbarButton } from '../ui/Toolbar';
import type { MenuItem } from '../menu';
import { getModalRegistrySnapshot } from '../../lib/modalRegistry';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useAuthStore } from '../../store/authStore';
import { useQuestionnaireUIStore } from '../../store/questionnaireUIStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { usePagePresentationCommands, WORKSPACE_LIST_COMMAND_IDS } from '../../lib/commandPagePresentation';

interface Props { rootRef?: RefObject<HTMLDivElement>; items: MenuItem[]; onClose(): void; onSelect(item: MenuItem): void }

// Mounted only while open: closing the list also revokes its command surface.
export function WorkspaceListDialog({ items, onClose, onSelect, rootRef }: Props) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { announce } = useAnnouncer();
  const workspaces = useWorkspaceStore(s => s.workspaces);
  const ownRoot = useRef<HTMLDivElement>(null);
  const root = rootRef ?? ownRoot;
  const focusGrid = useRef<GridFocusRequest>();
  const mounted = useRef(true);
  const busy = useRef(false);
  const questionnaireId = useRef<string | null>(null);
  const [pending, setPending] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [removeId, setRemoveId] = useState<string | null>(null);
  const selectedWorkspace = workspaces.find(ws => ws.id === selected);
  const removedWorkspace = workspaces.find(ws => ws.id === removeId);
  const user = useAuthStore(s => s.user);
  const owner = useRef(user);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const visibleWorkspaces = workspaces.filter(ws => `${ws.name} ${ws.path}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()));
  useEffect(() => { mounted.current = true; return () => {
    mounted.current = false;
    if (questionnaireId.current) useQuestionnaireUIStore.getState().cancelById(questionnaireId.current);
  }; }, []);
  useEffect(() => {
    if (user?.userId !== owner.current?.userId || user?.sessionId !== owner.current?.sessionId) onClose();
  }, [user, onClose]);
  const current = () => {
    const auth = useAuthStore.getState();
    return mounted.current && auth.isAuthenticated && auth.user?.userId === owner.current?.userId &&
      auth.user?.sessionId === owner.current?.sessionId;
  };
  const ownsTopmostModal = () => {
    const top = getModalRegistrySnapshot().topID;
    return Boolean(top && root.current?.closest('[data-modal-id]')?.getAttribute('data-modal-id') === top);
  };
  const restore = (id?: string) => {
    busy.current = false;
    if (!mounted.current) return;
    setPending(false);
    requestAnimationFrame(() => { if (current() && ownsTopmostModal()) focusGrid.current?.({ itemId: id }); });
  };
  const rename = async (id: string, name: string) => {
    busy.current = true;
    setPending(true);
    setError('');
    try {
      questionnaireId.current = `workspace-rename-${crypto.randomUUID()}`;
      const response = await useQuestionnaireUIStore.getState().request({
        id: questionnaireId.current, title: t('workspace.renamePlaceholder'), allowCancel: true,
        submitLabel: t('common.save'), cancelLabel: t('common.cancel'),
        questions: [{ id: 'name', type: 'text', prompt: t('workspace.renamePlaceholder'), required: true, default: name }],
      });
      if (response.cancelled || !current()) return;
      const value = response.answers.name;
      if (typeof value !== 'string' || !value.trim()) return;
      await useWorkspaceStore.getState().renameWorkspace(id, value.trim());
      await useWorkspaceStore.getState().refreshWorkspaceList();
    } catch {
      if (current()) {
        const message = t('commandPalette.executionFailed');
        setError(message);
        announce(message);
      }
    } finally { questionnaireId.current = null; restore(id); }
  };
  const { request } = usePagePresentationCommands({
    root, pathname, workspaceManager: true, allowedCommands: WORKSPACE_LIST_COMMAND_IDS,
    readTarget: () => selected,
    isCurrent: current,
    canOpen: id => !busy.current && !removeId && Boolean(selectedWorkspace) &&
      (id === 'workspaces.rename.open' || !selectedWorkspace?.is_active),
    open: id => {
      if (!selectedWorkspace || busy.current || removeId) return false;
      if (id === 'workspaces.rename.open') void rename(selectedWorkspace.id, selectedWorkspace.name);
      else if (!selectedWorkspace.is_active) setRemoveId(selectedWorkspace.id);
      else return false;
      return true;
    },
  });
  const remove = async () => {
    if (!removeId || busy.current || !current()) return;
    busy.current = true;
    setPending(true);
    setError('');
    try {
      await useWorkspaceStore.getState().deleteWorkspace(removeId);
      if (current()) announce(t('workspace.removed'));
    } catch {
      if (current()) {
        const message = t('commandPalette.executionFailed');
        setError(message);
        announce(message);
      }
    } finally {
      setRemoveId(null);
      restore();
    }
  };
  const activate = (id: string) => {
    if (busy.current || !current()) return;
    const modal = root.current?.closest('[data-modal-id]');
    if (modal?.getAttribute('data-modal-id') !== getModalRegistrySnapshot().topID) return;
    const item = items.find(candidate => candidate.id === `ws-${id}` || candidate.id === `ws-command-${id}`);
    if (item) { item.action?.(); onSelect(item); onClose(); }
  };
  return <>
    <Modal isOpen onClose={onClose} title={t('workspace.workspaceList')} allowClose={!pending}
      initialFocusSelector={visibleWorkspaces.length ? DATAGRID_ENTRY_SELECTOR : 'input'} returnFocusOnClose={false}>
      <div ref={root}>
        <Toolbar ariaLabel={t('workspace.workspaceList')} searchValue={search} onSearchChange={setSearch} searchPlaceholder={t('workspace.searchWorkspaces')}
          onFocusContent={() => focusGrid.current?.({})} left={<>
          <ToolbarButton label={t('common.open')} onClick={() => { if (selected) activate(selected); }} disabled={!selectedWorkspace || pending} />
          <ToolbarButton label={t('workspace.renamePlaceholder')} onClick={() => request('workspaces.rename.open')} disabled={!selectedWorkspace || pending} />
          <ToolbarButton label={t('workspace.remove')} onClick={() => request('workspaces.delete.open')} disabled={!selectedWorkspace || selectedWorkspace.is_active || pending} />
        </>} />
        {error && <p>{error}</p>}
        <DataGrid items={visibleWorkspaces} getItemId={ws => ws.id} label={t('workspace.workspaceList')}
          onEmptyFocus={() => {
            if (!ownsTopmostModal()) return false;
            const searchInput = root.current?.querySelector<HTMLInputElement>('input');
            searchInput?.focus();
            return Boolean(searchInput && document.activeElement === searchInput);
          }}
          commandManagedActions onGridReady={focus => { focusGrid.current = focus; }}
          onFocusChange={ws => setSelected(ws?.id ?? null)}
          columns={[{ key: 'name', label: t('workspace.renamePlaceholder') }, { key: 'path', label: t('workspace.directory') },
            { key: 'is_active', label: t('common.status'), format: value => t(value ? 'workspace.activeStatus' : 'workspace.inactiveStatus') }]}
          onActivate={ws => activate(ws.id)} />
      </div>
    </Modal>
    <ConfirmDialog isOpen={Boolean(removeId)} title={t('workspace.remove')}
      message={t('workspace.removeConfirmation', { name: removedWorkspace?.name ?? '' })}
      onConfirm={() => { void remove(); }} onCancel={() => { if (!busy.current) { setRemoveId(null); restore(selected ?? undefined); } }}
      returnFocusOnClose={false} />
  </>;
}
