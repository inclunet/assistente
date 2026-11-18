import { useEffect, useRef, useState, type RefObject } from 'react';
import { useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { DataGrid, DATAGRID_ENTRY_SELECTOR, type GridFocusRequest } from '../ui/DataGrid';
import { ConfirmDialog } from '../ui/ConfirmDialog';
import { Toolbar, ToolbarButton } from '../ui/Toolbar';
import { getModalRegistrySnapshot } from '../../lib/modalRegistry';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useAuthStore } from '../../store/authStore';
import { useQuestionnaireUIStore } from '../../store/questionnaireUIStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { usePagePresentationCommands, WORKSPACE_LIST_COMMAND_IDS } from '../../lib/commandPagePresentation';

interface Props {
  rootRef?: RefObject<HTMLDivElement>;
  onClose(): void;
  onActivate(id: string): void;
}

type Operation = 'idle' | 'renaming' | 'removing';

// Mounted only while open: closing the list also revokes its command surface.
export function WorkspaceListDialog({ onClose, onActivate, rootRef }: Props) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { announce } = useAnnouncer();
  const cachedWorkspaces = useWorkspaceStore(s => s.workspaces);
  const activeWorkspaceId = useWorkspaceStore(s => s.workspace?.id);
  const workspaces = cachedWorkspaces.map(ws => ({ ...ws, is_active: ws.id === activeWorkspaceId }));
  const ownRoot = useRef<HTMLDivElement>(null);
  const root = rootRef ?? ownRoot;
  const focusGrid = useRef<GridFocusRequest>();
  const mountedRef = useRef(true);
  // The ref blocks repeated commands before React commits the disabled UI.
  const operationRef = useRef<Operation>('idle');
  const questionnaireIdRef = useRef<string | null>(null);
  const [operation, setOperation] = useState<Operation>('idle');
  const pending = operation !== 'idle';
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [removalTargetId, setRemovalTargetId] = useState<string | null>(null);
  const selectedWorkspace = workspaces.find(ws => ws.id === selectedId);
  const removalTarget = workspaces.find(ws => ws.id === removalTargetId);
  const user = useAuthStore(s => s.user);
  const ownerRef = useRef(user);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const visibleWorkspaces = workspaces.filter(ws => `${ws.name} ${ws.path}`.toLocaleLowerCase().includes(search.toLocaleLowerCase()));
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (questionnaireIdRef.current) {
        useQuestionnaireUIStore.getState().cancelById(questionnaireIdRef.current);
      }
    };
  }, []);

  useEffect(() => {
    if (user?.userId !== ownerRef.current?.userId || user?.sessionId !== ownerRef.current?.sessionId) {
      onClose();
    }
  }, [user, onClose]);

  const isSessionCurrent = () => {
    const auth = useAuthStore.getState();
    return mountedRef.current && auth.isAuthenticated && auth.user?.userId === ownerRef.current?.userId &&
      auth.user?.sessionId === ownerRef.current?.sessionId;
  };
  const ownsTopmostModal = () => {
    const top = getModalRegistrySnapshot().topID;
    return Boolean(top && root.current?.closest('[data-modal-id]')?.getAttribute('data-modal-id') === top);
  };
  const reportOperationError = () => {
    if (!isSessionCurrent()) return;
    const message = t('commandPalette.executionFailed');
    setError(message);
    announce(message);
  };

  const beginOperation = (nextOperation: Exclude<Operation, 'idle'>) => {
    if (operationRef.current !== 'idle') return false;
    operationRef.current = nextOperation;
    setOperation(nextOperation);
    setError('');
    return true;
  };

  const finishOperation = (id?: string) => {
    operationRef.current = 'idle';
    if (!mountedRef.current) return;
    setOperation('idle');
    requestAnimationFrame(() => {
      if (isSessionCurrent() && ownsTopmostModal()) focusGrid.current?.({ itemId: id });
    });
  };
  const renameWorkspace = async (id: string, name: string) => {
    if (!beginOperation('renaming')) return;
    try {
      questionnaireIdRef.current = `workspace-rename-${crypto.randomUUID()}`;
      const response = await useQuestionnaireUIStore.getState().request({
        id: questionnaireIdRef.current,
        title: t('workspace.renamePlaceholder'),
        allowCancel: true,
        submitLabel: t('common.save'),
        cancelLabel: t('common.cancel'),
        questions: [{ id: 'name', type: 'text', prompt: t('workspace.renamePlaceholder'), required: true, default: name }],
      });
      if (response.cancelled || !isSessionCurrent()) return;
      const value = response.answers.name;
      if (typeof value !== 'string' || !value.trim()) return;
      await useWorkspaceStore.getState().renameWorkspace(id, value.trim());
      await useWorkspaceStore.getState().refreshWorkspaceList();
    } catch {
      reportOperationError();
    } finally {
      questionnaireIdRef.current = null;
      finishOperation(id);
    }
  };
  const { request } = usePagePresentationCommands({
    root,
    pathname,
    workspaceManager: true,
    allowedCommands: WORKSPACE_LIST_COMMAND_IDS,
    readTarget: () => selectedId,
    isCurrent: isSessionCurrent,
    canOpen: id => operationRef.current === 'idle' && !removalTargetId && Boolean(selectedWorkspace) &&
      (id === 'workspaces.rename.open' || !selectedWorkspace?.is_active),
    open: id => {
      if (!selectedWorkspace || operationRef.current !== 'idle' || removalTargetId) return false;
      if (id === 'workspaces.rename.open') {
        void renameWorkspace(selectedWorkspace.id, selectedWorkspace.name);
      } else if (!selectedWorkspace.is_active) {
        setRemovalTargetId(selectedWorkspace.id);
      } else {
        return false;
      }
      return true;
    },
  });
  const removeWorkspace = async () => {
    if (!removalTargetId || !isSessionCurrent() ||
      useWorkspaceStore.getState().workspace?.id === removalTargetId) return;
    if (!beginOperation('removing')) return;
    try {
      await useWorkspaceStore.getState().deleteWorkspace(removalTargetId);
    } catch {
      reportOperationError();
    } finally {
      setRemovalTargetId(null);
      finishOperation();
    }
  };
  useEffect(() => {
    void useWorkspaceStore.getState().refreshWorkspaceList().then(ok => {
      if (!ok) reportOperationError();
    });
  // Refresh once for this dialog lifetime; session checks guard late results.
  }, []);
  const activateWorkspace = (id: string) => {
    if (operationRef.current !== 'idle' || !isSessionCurrent()) return;
    const modal = root.current?.closest('[data-modal-id]');
    if (modal?.getAttribute('data-modal-id') !== getModalRegistrySnapshot().topID) return;
    if (useWorkspaceStore.getState().workspaces.some(ws => ws.id === id)) onActivate(id);
  };
  return (
    <>
      <Modal
        isOpen
        onClose={onClose}
        title={t('workspace.workspaceList')}
        allowClose={!pending}
        initialFocusSelector={visibleWorkspaces.length ? DATAGRID_ENTRY_SELECTOR : 'input'}
        returnFocusOnClose={false}
      >
        <div ref={root}>
          <Toolbar
            ariaLabel={t('workspace.workspaceList')}
            searchValue={search}
            onSearchChange={setSearch}
            searchPlaceholder={t('workspace.searchWorkspaces')}
            onFocusContent={() => focusGrid.current?.({})}
            left={
              <>
                <ToolbarButton
                  label={t('common.open')}
                  onClick={() => { if (selectedId) activateWorkspace(selectedId); }}
                  disabled={!selectedWorkspace || pending}
                />
                <ToolbarButton
                  label={t('workspace.renamePlaceholder')}
                  onClick={() => request('workspaces.rename.open')}
                  disabled={!selectedWorkspace || pending}
                />
                <ToolbarButton
                  label={t('workspace.remove')}
                  onClick={() => request('workspaces.delete.open')}
                  disabled={!selectedWorkspace || selectedWorkspace.is_active || pending}
                />
              </>
            }
          />
          {error && <p>{error}</p>}
          <DataGrid
            items={visibleWorkspaces}
            getItemId={ws => ws.id}
            label={t('workspace.workspaceList')}
            onEmptyFocus={() => {
              if (!ownsTopmostModal()) return false;
              const searchInput = root.current?.querySelector<HTMLInputElement>('input');
              searchInput?.focus();
              return Boolean(searchInput && document.activeElement === searchInput);
            }}
            commandManagedActions
            onGridReady={focus => { focusGrid.current = focus; }}
            onFocusChange={ws => setSelectedId(ws?.id ?? null)}
            columns={[
              { key: 'name', label: t('workspace.renamePlaceholder') },
              { key: 'path', label: t('workspace.directory') },
              {
                key: 'is_active',
                label: t('common.status'),
                format: value => t(value ? 'workspace.activeStatus' : 'workspace.inactiveStatus'),
              },
            ]}
            onActivate={ws => activateWorkspace(ws.id)}
          />
        </div>
      </Modal>
      <ConfirmDialog
        isOpen={Boolean(removalTargetId)}
        title={t('workspace.remove')}
        message={t('workspace.removeConfirmation', { name: removalTarget?.name ?? '' })}
        onConfirm={() => { void removeWorkspace(); }}
        onCancel={() => {
          if (operationRef.current !== 'idle') return;
          setRemovalTargetId(null);
          finishOperation(selectedId ?? undefined);
        }}
        returnFocusOnClose={false}
      />
    </>
  );
}
