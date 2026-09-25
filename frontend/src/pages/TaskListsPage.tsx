import { logger } from '../utils/logger';
import { useState, useEffect, useMemo, useCallback, useRef, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  CheckOutlined,
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  ExportOutlined,
  ReadOutlined,
} from '@ant-design/icons';
import { useTaskListStore } from '../store/taskListStore';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { Modal } from '../components/ui/Modal';
import { Button } from '../components/ui/Button';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { FormField } from '../components/ui/FormField';
import { Input } from '../components/ui/Input';
import { Textarea } from '../components/ui/Textarea';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useGridFocus } from '../hooks/useGridFocus';
import { useUIStore } from '../store/uiStore';
import { executeDeepLink } from '../lib/deepLinks';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import { usePagePresentationCommands, type PagePresentationCommandID } from '../lib/commandPagePresentation';
import { readTaskListCommandTarget } from '../lib/commandPageMutationWails';
import { usePageMutationCommands, type PageMutationID, type PageMutationRequest, type PageMutationResult } from '../lib/commandPageMutation';
import { useCommandShortcutHint } from '../lib/commandShortcutHints';
import type { TaskListWithWorkflow } from '../types/tasklist';
import './TaskListsPage.css';

interface TaskListRow extends TaskListWithWorkflow {
  id: string;
}

interface CommandPageContext {
  pathname: string;
  ownerId: string;
  sessionId: string;
  workspaceId: string;
}

interface MutationSuccessGuard {
  context: CommandPageContext;
  targetId?: string;
  capturedTarget?: TaskListRow;
  editorVersion?: number;
  draftVersion?: number;
}

export default function TaskListsPage() {
  const { t } = useTranslation();
  const createShortcutHint = useCommandShortcutHint('tasklists.create.open', 'tasklists');
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'tasklist-page' });

  const taskLists = useTaskListStore((state) => state.taskLists);
  const { getCachedTaskList, loadTaskList, fetchAllTaskLists } = useTaskListStore();
  const addTab = useWorkspaceStore((s) => s.addTab);
  const moveTabToWorkspace = useWorkspaceStore((s) => s.moveTabToWorkspace);
  const workspaces = useWorkspaceStore((s) => s.workspaces);

  const [searchTerm, setSearchTerm] = useState('');
  const [editorOpen, setEditorOpen] = useState(false);
  const [editTitle, setEditTitle] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [editorMode, setEditorMode] = useState<'create' | 'edit'>('create');
  const [editingLoading, setEditingLoading] = useState(false);
  const [focusedTaskList, setFocusedTaskList] = useState<TaskListRow | null>(null);
  const [editorReadLoading, setEditorReadLoading] = useState(false);

  const pageRef = useRef<HTMLDivElement>(null);
  const toolbarRef = useRef<HTMLDivElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);
  const presentationTargetRef = useRef<TaskListRow | null>(null);
  const editorFormRef = useRef<HTMLFormElement>(null);
  const editorSnapshotRef = useRef<{ id: string | null; fingerprint: string; version: number }>({ id: null, fingerprint: '', version: 0 });
  const editorVersionRef = useRef(0);
  const editorDraftRef = useRef({ title: '', description: '', version: 0 });
  const editorOpenRef = useRef(editorOpen);
  const editorReadLoadingRef = useRef(editorReadLoading);
  const editingLoadingRef = useRef(editingLoading);
  const editorModeRef = useRef(editorMode);
  const pathnameRef = useRef(pathname);
  const mountedRef = useRef(true);
  pathnameRef.current = pathname;
  editorOpenRef.current = editorOpen;
  editorReadLoadingRef.current = editorReadLoading;
  editingLoadingRef.current = editingLoading;
  editorModeRef.current = editorMode;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      ++editorVersionRef.current;
    };
  }, []);

  const readCommandPageContext = useCallback((): CommandPageContext | null => {
    const auth = useAuthStore.getState();
    const workspace = useWorkspaceStore.getState().workspace;
    if (!auth.isAuthenticated || !auth.user || !workspace) return null;
    return {
      pathname: pathnameRef.current,
      ownerId: auth.user.userId,
      sessionId: auth.user.sessionId,
      workspaceId: workspace.id,
    };
  }, []);

  const isCommandPageContextCurrent = useCallback((expected: CommandPageContext) => {
    const current = readCommandPageContext();
    return mountedRef.current && current !== null &&
      current.pathname === expected.pathname &&
      current.ownerId === expected.ownerId &&
      current.sessionId === expected.sessionId &&
      current.workspaceId === expected.workspaceId;
  }, [readCommandPageContext]);

  const setEditorDraft = useCallback((title: string, description: string) => {
    editorDraftRef.current = { title, description, version: editorDraftRef.current.version + 1 };
    setEditTitle(title);
    setEditDescription(description);
  }, []);

  const updateEditorTitle = useCallback((title: string) => {
    editorDraftRef.current = { ...editorDraftRef.current, title, version: editorDraftRef.current.version + 1 };
    setEditTitle(title);
  }, []);

  const updateEditorDescription = useCallback((description: string) => {
    editorDraftRef.current = { ...editorDraftRef.current, description, version: editorDraftRef.current.version + 1 };
    setEditDescription(description);
  }, []);

  const getRowId = useCallback((item: TaskListRow) => item.id, []);
  const handleFocusChange = useCallback((item: TaskListRow | null) => {
    setFocusedTaskList(item);
    presentationTargetRef.current = item;
  }, []);

  // Carrega todas as listas ao montar
  const loadedRef = useRef(false);
  useEffect(() => {
    if (loadedRef.current) return;
    loadedRef.current = true;
    // O catálogo já traz metadados, workflow e contagem agregada em uma
    // chamada. Cards são carregados por keyset somente ao abrir uma lista.
    void fetchAllTaskLists();
  }, [fetchAllTaskLists]);

  useResourceEditRequest('tasklists', {
    onEdit: (id) => {
      const list = taskLists.get(id);
      if (list) handleOpenEditor(list as TaskListRow);
    },
    onNew: () => handleOpenEditor(),
    ready: loadedRef.current,
  });

  const allTaskLists = useMemo(() => Array.from(taskLists.values()), [taskLists]);

  const filteredTaskLists = useMemo(
    () =>
      allTaskLists.filter(
        (list) =>
          list.title.toLowerCase().includes(searchTerm.toLowerCase()) ||
          (list.description || '').toLowerCase().includes(searchTerm.toLowerCase())
      ),
    [allTaskLists, searchTerm]
  );

  const handleOpenEditor = useCallback(async (list?: TaskListRow) => {
    const context = readCommandPageContext();
    if (!context) return;
    const version = ++editorVersionRef.current;
    editorSnapshotRef.current = { id: list?.id ?? null, fingerprint: '', version };
    editorDraftRef.current = { title: '', description: '', version: editorDraftRef.current.version + 1 };
    if (!list) {
      setEditorMode('create');
      editorModeRef.current = 'create';
      setEditorDraft('', '');
      setEditorReadLoading(false);
      editorReadLoadingRef.current = false;
      setEditorOpen(true);
      editorOpenRef.current = true;
      return;
    }

    setEditorMode('edit');
    editorModeRef.current = 'edit';
    setEditorDraft('', '');
    setEditorReadLoading(true);
    editorReadLoadingRef.current = true;
    setEditorOpen(true);
    editorOpenRef.current = true;
    try {
      const target = await readTaskListCommandTarget(list.id);
      if (!mountedRef.current || editorVersionRef.current !== version || !isCommandPageContextCurrent(context)) return;
      editorSnapshotRef.current = { id: list.id, fingerprint: target.fingerprint, version };
      setEditorDraft(target.taskList.title, target.taskList.description || '');
    } catch (error) {
      if (!mountedRef.current || editorVersionRef.current !== version || !isCommandPageContextCurrent(context)) return;
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao carregar lista'), 'error', undefined, undefined, { suppressAnnounce: true });
      announce(msg || t('common.error', 'Erro ao carregar lista'));
      setEditorOpen(false);
      editorOpenRef.current = false;
      editorSnapshotRef.current = { id: null, fingerprint: '', version };
    } finally {
      if (mountedRef.current && editorVersionRef.current === version && isCommandPageContextCurrent(context)) {
        setEditorReadLoading(false);
        editorReadLoadingRef.current = false;
      }
    }
  }, [addToast, announce, isCommandPageContextCurrent, readCommandPageContext, setEditorDraft, t]);

  const readLivePresentationTarget = useCallback(() => {
    const captured = presentationTargetRef.current;
    if (!captured) return null;
    const current = useTaskListStore.getState().taskLists.get(captured.id);
    return current === captured ? captured : null;
  }, []);

  const pagePresentationCommands: readonly PagePresentationCommandID[] = [
    'tasklists.create.open',
    'tasklists.edit.open',
    'tasklists.search.focus',
  ];

  const { request: requestPagePresentationCommand } = usePagePresentationCommands({
    root: pageRef,
    pathname,
    allowedCommands: pagePresentationCommands,
    readTarget: readLivePresentationTarget,
    subscribe: (changed) => useTaskListStore.subscribe(() => changed()),
    isCurrent: () => pathnameRef.current === '/tasklists' && mountedRef.current,
    canOpen: (id) => {
      if (editorOpenRef.current) return false;
      if (id === 'tasklists.edit.open') {
        return readLivePresentationTarget() !== null;
      }
      if (id === 'tasklists.search.focus') {
        return Boolean(pageRef.current?.querySelector<HTMLInputElement>('.toolbar__search'));
      }
      return true;
    },
    open: (id) => {
      if (editorOpenRef.current) return false;
      if (id === 'tasklists.create.open') {
        handleOpenEditor();
        return true;
      }
      if (id === 'tasklists.edit.open') {
        const target = readLivePresentationTarget();
        if (!target) return false;
        handleOpenEditor(target);
        return true;
      }
      if (id === 'tasklists.search.focus') {
        const search = pageRef.current?.querySelector<HTMLInputElement>('.toolbar__search');
        if (!search) return false;
        search.focus();
        return true;
      }
      return false;
    },
  });

  const handleCloseEditor = useCallback(() => {
    ++editorVersionRef.current;
    setEditorOpen(false);
    editorOpenRef.current = false;
    setEditTitle('');
    setEditDescription('');
    editorDraftRef.current = { title: '', description: '', version: editorDraftRef.current.version + 1 };
    setEditorReadLoading(false);
    editorReadLoadingRef.current = false;
    setEditingLoading(false);
    editingLoadingRef.current = false;
    editorSnapshotRef.current = { id: null, fingerprint: '', version: editorVersionRef.current };
  }, []);

  const { request: requestFormPageMutation } = usePageMutationCommands({
    root: editorFormRef,
    pathname,
    allowedCommands: ['tasklists.create', 'tasklists.update'],
    canStart: (commandId) => {
      if (!editorOpenRef.current || editorReadLoadingRef.current || editingLoadingRef.current || !editorDraftRef.current.title.trim()) return false;
      if (commandId === 'tasklists.create') return editorModeRef.current === 'create';
      return editorModeRef.current === 'edit' && Boolean(editorSnapshotRef.current.id && editorSnapshotRef.current.fingerprint);
    },
    prepare: (commandId) => {
      const snapshot = editorSnapshotRef.current;
      const context = readCommandPageContext();
      const draftVersion = editorDraftRef.current.version;
      if (!context || !editorDraftRef.current.title.trim()) return undefined;
      const draft: PageMutationRequest = {
        targetId: commandId === 'tasklists.create' ? '' : snapshot.id || '',
        expectedFingerprint: commandId === 'tasklists.create' ? '' : snapshot.fingerprint,
        title: editorDraftRef.current.title,
        description: editorDraftRef.current.description,
      };
      if (commandId === 'tasklists.update' && (!draft.targetId || !draft.expectedFingerprint)) return undefined;
      return {
        readRequest: async () => draft,
        isCurrent: () => editorOpenRef.current &&
          editorVersionRef.current === snapshot.version &&
          editorSnapshotRef.current.id === snapshot.id &&
          editorSnapshotRef.current.fingerprint === snapshot.fingerprint &&
          editorDraftRef.current.version === draftVersion &&
          editorDraftRef.current.title === draft.title &&
          editorDraftRef.current.description === draft.description &&
          isCommandPageContextCurrent(context),
        canPresent: () => editorOpenRef.current &&
          editorVersionRef.current === snapshot.version &&
          editorDraftRef.current.version === draftVersion &&
          isCommandPageContextCurrent(context),
        succeeded: async (result: PageMutationResult) => handlePageMutationSucceeded(commandId, result, draft.targetId, {
          context,
          targetId: draft.targetId,
          editorVersion: snapshot.version,
          draftVersion,
        }),
      };
    },
  });

  const handlePageMutationSucceeded = useCallback(async (commandId: PageMutationID, result: PageMutationResult, targetId?: string, guard?: MutationSuccessGuard) => {
    if (guard && !isCommandPageContextCurrent(guard.context)) return;
    if (guard?.editorVersion !== undefined && (
      !editorOpenRef.current ||
      editorVersionRef.current !== guard.editorVersion ||
      (guard.draftVersion !== undefined && editorDraftRef.current.version !== guard.draftVersion)
    )) return;
    if (guard?.capturedTarget && presentationTargetRef.current !== guard.capturedTarget) return;
    await fetchAllTaskLists();
    if (guard && !isCommandPageContextCurrent(guard.context)) return;
    if (guard?.editorVersion !== undefined && (
      !editorOpenRef.current ||
      editorVersionRef.current !== guard.editorVersion ||
      (guard.draftVersion !== undefined && editorDraftRef.current.version !== guard.draftVersion)
    )) return;
    if (guard?.capturedTarget && presentationTargetRef.current !== guard.capturedTarget) return;
    if (guard && !isCommandPageContextCurrent(guard.context)) return;
    if (guard?.editorVersion !== undefined) {
      const successMessage = commandId === 'tasklists.create'
        ? t('tasklist.createdSuccess', `Lista "${result.title || editorDraftRef.current.title}" criada com sucesso!`)
        : t('common.success', 'Salvo com sucesso');
      addToast(successMessage, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(successMessage);
      handleCloseEditor();
      if (commandId === 'tasklists.create' && result.id) {
        await executeDeepLink(
          { type: 'tab:open', tabType: 'tasklist', contentId: String(result.id), title: result.title },
          { navigate },
        );
      }
      return;
    }
    const successMessage = commandId === 'tasklists.duplicate'
      ? t('tasklist.clonedSuccess', 'Lista clonada com sucesso')
      : commandId === 'tasklists.clear'
        ? t('tasklist.clearedSuccess', 'Lista limpa com sucesso')
        : t('tasklist.deletedSuccess', 'Lista deletada com sucesso');
    addToast(successMessage, 'success', undefined, undefined, { suppressAnnounce: true });
    announce(successMessage);
    if (commandId === 'tasklists.duplicate' && result.id) {
      await executeDeepLink(
        { type: 'tab:open', tabType: 'tasklist', contentId: String(result.id), title: result.title },
        { navigate },
      );
    }
    if (commandId === 'tasklists.delete' && guard?.capturedTarget && presentationTargetRef.current === guard.capturedTarget && focusedTaskList?.id === targetId) {
      setFocusedTaskList(null);
      presentationTargetRef.current = null;
    }
  }, [addToast, announce, fetchAllTaskLists, focusedTaskList, handleCloseEditor, isCommandPageContextCurrent, navigate, t]);

  const handleSaveTaskList = useCallback(async () => {
    if (!editorDraftRef.current.title.trim()) {
      addToast(t('tasklist.emptyTitle', 'Título não pode estar vazio'), 'error', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('tasklist.emptyTitle', 'Título não pode estar vazio'));
      return;
    }

    const operationVersion = editorVersionRef.current;
    const operationContext = readCommandPageContext();
    if (!operationContext) return;
    try {
      const commandId: PageMutationID = editorModeRef.current === 'create' ? 'tasklists.create' : 'tasklists.update';
      // O dispatch do comando é síncrono até a captura do alvo. Marcar busy
      // antes dele faria canStart rejeitar a própria operação nativa.
      const outcomePromise = requestFormPageMutation(commandId);
      setEditingLoading(true);
      editingLoadingRef.current = true;
      const outcome = await outcomePromise;
      if (outcome.status !== 'succeeded') {
        if (outcome.status === 'cancelled') return;
        throw new Error(t('common.error', 'Erro ao salvar'));
      }
    } catch (error) {
      if (!mountedRef.current || editorVersionRef.current !== operationVersion || !isCommandPageContextCurrent(operationContext)) return;
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao salvar'), 'error', undefined, undefined, {
        suppressAnnounce: true,
      });
    } finally {
      if (mountedRef.current && editorVersionRef.current === operationVersion && isCommandPageContextCurrent(operationContext)) {
        setEditingLoading(false);
        editingLoadingRef.current = false;
      }
    }
  }, [addToast, announce, isCommandPageContextCurrent, readCommandPageContext, requestFormPageMutation, t]);

  const handleOpenTaskList = useCallback(async (taskListId: string) => {
    if (!getCachedTaskList(taskListId)) {
      await loadTaskList(taskListId);
    }
    const cached = useTaskListStore.getState().taskLists.get(taskListId);
    const title = cached?.title || 'Lista';
    await executeDeepLink(
      { type: 'tab:open', tabType: 'tasklist', contentId: String(taskListId), title },
      { navigate },
    );
  }, [getCachedTaskList, loadTaskList, navigate]);

  const { request: requestRootPageMutation } = usePageMutationCommands({
    root: pageRef,
    pathname,
    allowedCommands: ['tasklists.duplicate', 'tasklists.delete', 'tasklists.clear'],
    canStart: (commandId) => {
      if (editorOpenRef.current) return false;
      const captured = presentationTargetRef.current;
      return Boolean(captured && useTaskListStore.getState().taskLists.get(captured.id) === captured &&
        (commandId !== 'tasklists.clear' || (captured.taskCount ?? captured.tasks.length) > 0));
    },
    prepare: (commandId) => {
      const captured = presentationTargetRef.current;
      if (!captured) return undefined;
      const targetId = captured.id;
      const context = readCommandPageContext();
      if (!context) return undefined;
      return {
        readRequest: async (): Promise<PageMutationRequest> => {
          const target = await readTaskListCommandTarget(targetId);
          const title = commandId === 'tasklists.duplicate'
            ? `${target.taskList.title} ${t('tasklist.cloneTitleSuffix', '(Cópia)')}`
            : '';
          return { targetId, expectedFingerprint: target.fingerprint, title, description: target.taskList.description || '' };
        },
        isCurrent: () => {
          const current = useTaskListStore.getState().taskLists.get(targetId);
          return !editorOpenRef.current &&
            presentationTargetRef.current === captured &&
            current === captured &&
            (commandId !== 'tasklists.clear' || (captured.taskCount ?? captured.tasks.length) > 0) &&
            isCommandPageContextCurrent(context);
        },
        canPresent: () => !editorOpenRef.current &&
          presentationTargetRef.current === captured &&
          isCommandPageContextCurrent(context),
        succeeded: async (result: PageMutationResult) => handlePageMutationSucceeded(commandId, result, targetId, {
          context,
          targetId,
          capturedTarget: captured,
        }),
      };
    },
    subscribe: (changed) => useTaskListStore.subscribe(() => changed()),
  });

  const runRootPageMutation = useCallback(async (commandId: PageMutationID, list: TaskListRow) => {
    const context = readCommandPageContext();
    if (!context) return;
    presentationTargetRef.current = list;
    setFocusedTaskList(list);
    const outcome = await requestRootPageMutation(commandId);
    if (outcome.status !== 'succeeded' && outcome.status !== 'cancelled' &&
      isCommandPageContextCurrent(context) && presentationTargetRef.current === list) {
      const message = t('common.error', 'Erro ao alterar lista');
      addToast(message, 'error', undefined, undefined, { suppressAnnounce: true });
    }
  }, [requestRootPageMutation, addToast, isCommandPageContextCurrent, readCommandPageContext, t]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && editorOpen) {
        event.preventDefault();
        handleCloseEditor();
      }
    };

    if (editorOpen) {
      window.addEventListener('keydown', handleKeyDown);
      return () => window.removeEventListener('keydown', handleKeyDown);
    }
  }, [editorOpen, handleCloseEditor]);

  useEffect(() => {
    if (filteredTaskLists.length === 0) {
      setFocusedTaskList(null);
    }
  }, [filteredTaskLists.length]);

  const handleSendToWorkspace = useCallback(async (taskListId: string, title: string, targetWorkspaceId: string, isActive: boolean) => {
    try {
      if (isActive) {
        await addTab('tasklist', title, { tasklistId: String(taskListId) });
        navigate('/');
      } else {
        const tabId = await addTab('tasklist', title, { tasklistId: String(taskListId) });
        await moveTabToWorkspace(tabId, targetWorkspaceId);
      }
      announce(t('tasklist.sentToWorkspace', 'Lista enviada ao workspace'));
    } catch (error) {
      logger.error('Erro ao enviar lista ao workspace:', error);
    }
  }, [addTab, moveTabToWorkspace, navigate, announce, t]);

  const getTaskListRowActions = useCallback(
    (list: TaskListRow) => {
      const captureRowTarget = () => {
        presentationTargetRef.current = list;
        setFocusedTaskList(list);
      };
      const actions = [
        {
          id: 'open',
          label: t('tasklist.open', 'Abrir'),
          icon: <ReadOutlined />,
          onClick: () => handleOpenTaskList(list.id),
        },
        {
          id: 'edit',
          label: t('tasklist.edit', 'Editar'),
          icon: <EditOutlined />,
          onClick: () => {
            captureRowTarget();
            requestPagePresentationCommand('tasklists.edit.open');
          },
        },
        {
          id: 'clone',
          label: t('tasklist.clone', 'Clonar'),
          icon: <CopyOutlined />,
          onClick: () => { captureRowTarget(); void runRootPageMutation('tasklists.duplicate', list); },
        },
      ];

      if (workspaces.length > 0) {
        actions.push({
          id: 'send-to-workspace',
          label: t('tasklist.sendToWorkspace', 'Enviar ao workspace'),
          icon: <ExportOutlined />,
          onClick: undefined as unknown as () => void,
          submenu: workspaces.map(ws => ({
            id: `ws-${ws.id}`,
            label: ws.name,
            icon: ws.is_active ? <CheckOutlined /> : undefined,
            onClick: () => handleSendToWorkspace(list.id, list.title, ws.id, ws.is_active),
          })),
        } as typeof actions[0] & { submenu: { id: string; label: string; icon?: ReactNode; onClick: () => void }[] });
      }

      actions.push({
        id: 'delete',
        label: t('tasklist.delete', 'Deletar'),
        icon: <DeleteOutlined />,
        onClick: () => { captureRowTarget(); void runRootPageMutation('tasklists.delete', list); },
        danger: true,
      } as typeof actions[0]);

      return actions;
    },
    [t, handleOpenTaskList, requestPagePresentationCommand, runRootPageMutation, handleSendToWorkspace, workspaces]
  );

  const columns: DataGridColumn<TaskListRow>[] = useMemo(
    () => [
      {
        key: 'title',
        label: t('tasklist.title', 'Título'),
        width: '25%',
      },
      {
        key: 'description',
        label: t('tasklist.description', 'Descrição'),
        width: '40%',
        truncate: true,
      },
      {
        key: 'createdAt',
        label: t('tasklist.created', 'Criado em'),
        width: '15%',
        format: (value) => {
          if (!value) return '—';
          const date = new Date(value as string);
          return date.toLocaleDateString('pt-BR', { month: '2-digit', day: '2-digit', year: '2-digit' });
        },
      },
      {
        key: 'actions',
        label: '',
        width: '5%',
        format: (_value, item) => (
          <MenuButton
            items={getTaskListRowActions(item as TaskListRow)}
            buttonLabel={t('common.actions', 'Ações')}
          />
        ),
      },
    ],
    [t, getTaskListRowActions]
  );

  const hasLists = filteredTaskLists.length > 0;

  const homeActions = [
    {
      key: 'new-list',
      label: t('tasklist.createNew', 'Nova Lista'),
      onClick: () => requestPagePresentationCommand('tasklists.create.open'),
      shortcut: createShortcutHint,
      variant: 'primary' as const,
    },
    {
      key: 'edit-list',
      label: t('tasklist.edit', 'Editar'),
      onClick: () => {
        if (!focusedTaskList) return;
        presentationTargetRef.current = focusedTaskList;
        requestPagePresentationCommand('tasklists.edit.open');
      },
      disabled: !focusedTaskList,
    },
    {
      key: 'clone-list',
      label: t('tasklist.clone', 'Clonar'),
      onClick: () => focusedTaskList && void runRootPageMutation('tasklists.duplicate', focusedTaskList),
      disabled: !focusedTaskList,
    },
    {
      key: 'delete-list',
      label: t('tasklist.delete', 'Deletar'),
      onClick: () => focusedTaskList && void runRootPageMutation('tasklists.delete', focusedTaskList),
      disabled: !focusedTaskList,
      variant: 'danger' as const,
    },
  ];

  return (
    <div ref={pageRef} className="tasklist-page">
      <Toolbar
        ref={toolbarRef}
        left={
          <h1 className="page-toolbar__title">
            {t('tasklist.allLists', 'Todas as Listas')}
          </h1>
        }
        searchPlaceholder={t('tasklist.search', 'Buscar listas...')}
        searchValue={searchTerm}
        onSearchChange={allTaskLists.length > 0 ? setSearchTerm : undefined}
        actions={homeActions}
      />
      {hasLists ? (
        <div ref={gridRef}>
          <DataGrid
            items={filteredTaskLists as TaskListRow[]}
            columns={columns}
            getItemId={getRowId}
            onActivate={(item: TaskListRow) => handleOpenTaskList(item.id)}
            getRowActions={getTaskListRowActions}
            onFocusChange={handleFocusChange}
            onGridReady={handleGridReady}
            label={t('tasklist.gridLabel', 'Lista de listas de tarefas')}
          />
        </div>
      ) : (
        <div className="tasklist-empty-state">
          <p className="tasklist-empty-message">{t('tasklist.noLists', 'Nenhuma lista de tarefas criada')}</p>
          <p className="tasklist-empty-hint">{t('tasklist.createNewHintGeneric', 'Clique em "Nova Lista" acima')}</p>
        </div>
      )}

      <Modal isOpen={editorOpen} onClose={handleCloseEditor} title={
        editorMode === 'create' ? t('tasklist.createNew', 'Criar Nova Lista') : t('tasklist.editList', 'Editar Lista')
      }>
        <form ref={editorFormRef} className="tasklist-editor-form" onSubmit={(e) => {
          e.preventDefault();
          handleSaveTaskList();
        }}>
          <FormField label={t('tasklist.title', 'Título')} id="edit-title">
            <Input
              value={editTitle}
              onChange={(e) => updateEditorTitle(e.target.value)}
              placeholder={t('tasklist.titlePlaceholder', 'Título da lista')}
              autoFocus
              disabled={editorReadLoading}
            />
          </FormField>

          <FormField label={t('tasklist.description', 'Descrição')} id="edit-description">
            <Textarea
              value={editDescription}
              onChange={(e) => updateEditorDescription(e.target.value)}
              placeholder={t('common.description', 'Descrição (opcional)')}
              rows={4}
              disabled={editorReadLoading}
            />
          </FormField>

          <EditorPanelFooter className="tasklist-editor__footer">
            <Button type="submit" loading={editingLoading || editorReadLoading}>
              {t('common.save', 'Salvar')}
            </Button>
            <div style={{ flex: 1 }} />
            <Button onClick={handleCloseEditor} variant="secondary">
              {t('common.cancel', 'Cancelar')}
            </Button>
          </EditorPanelFooter>
        </form>
      </Modal>
    </div>
  );
}
