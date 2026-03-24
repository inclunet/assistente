import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useTaskListStore } from '../store/taskListStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { Modal, isModalOpen } from '../components/ui/Modal';
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
import { useConfirm } from '../hooks/useConfirm';
import { useUIStore } from '../store/uiStore';
import { executeDeepLink } from '../lib/deepLinks';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import type { TaskListWithWorkflow } from '../types/tasklist';
import './TaskListsPage.css';

interface TaskListRow extends TaskListWithWorkflow {
  id: number;
}

export default function TaskListsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  const requestConfirm = useConfirm();
  useGridPageLandmarks({ pageClass: 'tasklist-page' });

  const taskLists = useTaskListStore((state) => state.taskLists);
  const { createTaskList, deleteTaskList, cloneTaskList, getCachedTaskList, loadTaskList, fetchAllTaskLists } = useTaskListStore();
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

  const toolbarRef = useRef<HTMLDivElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);

  const getRowId = useCallback((item: TaskListRow) => item.id, []);
  const handleFocusChange = useCallback((item: TaskListRow | null) => setFocusedTaskList(item), []);

  // Carrega todas as listas ao montar
  const loadedRef = useRef(false);
  useEffect(() => {
    if (loadedRef.current) return;
    loadedRef.current = true;
    void fetchAllTaskLists().then((lists) => {
      for (const list of lists) {
        const store = useTaskListStore.getState();
        if (!store.taskLists.has(list.id)) {
          void store.loadTaskList(list.id);
        }
      }
    });
  }, [fetchAllTaskLists]);

  useResourceEditRequest('tasklists', {
    onEdit: (id) => {
      const list = taskLists.get(Number(id));
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

  const handleOpenEditor = useCallback((list?: TaskListRow) => {
    if (list) {
      setEditorMode('edit');
      setEditTitle(list.title);
      setEditDescription(list.description || '');
    } else {
      setEditorMode('create');
      setEditTitle('');
      setEditDescription('');
    }
    setEditorOpen(true);
  }, []);

  const handleCloseEditor = useCallback(() => {
    setEditorOpen(false);
    setEditTitle('');
    setEditDescription('');
    setEditingLoading(false);
  }, []);

  const handleSaveTaskList = useCallback(async () => {
    if (!editTitle.trim()) {
      addToast(t('tasklist.emptyTitle', 'Título não pode estar vazio'), 'error');
      announce(t('tasklist.emptyTitle', 'Título não pode estar vazio'));
      return;
    }

    setEditingLoading(true);
    try {
      if (editorMode === 'create') {
        const newTaskList = await createTaskList(editTitle, editDescription || undefined);
        if (newTaskList) {
          await executeDeepLink(
            { type: 'tab:open', tabType: 'tasklist', contentId: String(newTaskList.id), title: newTaskList.title },
            { navigate },
          );
          addToast(t('tasklist.createdSuccess', `Lista "${editTitle}" criada com sucesso!`), 'success');
          announce(t('tasklist.createdSuccess', `Lista "${editTitle}" criada com sucesso!`));
        }
      } else {
        addToast(t('common.success', 'Salvo com sucesso'), 'success');
        announce(t('common.success', 'Salvo com sucesso'));
      }
      handleCloseEditor();
    } catch (error) {
      const msg = error instanceof Error ? error.message : String(error);
      addToast(msg || t('common.error', 'Erro ao salvar'), 'error');
      announce(msg || t('common.error', 'Erro ao salvar'));
    } finally {
      setEditingLoading(false);
    }
  }, [editTitle, editDescription, editorMode, createTaskList, navigate, addToast, announce, handleCloseEditor, t]);

  const handleOpenTaskList = useCallback(async (taskListId: number) => {
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

  const handleDeleteTaskList = useCallback(
    async (taskListId: number) => {
      const list = allTaskLists.find((l) => l.id === taskListId);
      const confirmed = await requestConfirm({
        title: t('tasklist.deleteConfirmTitle', 'Deletar Lista'),
        message: t('tasklist.deleteConfirmMessage', `Tem certeza que deseja deletar "${list?.title}"? Esta ação não pode ser desfeita.`),
      });

      if (!confirmed) return;

      try {
        await deleteTaskList(taskListId);
        addToast(t('tasklist.deletedSuccess', 'Lista deletada com sucesso'), 'success');
        announce(t('tasklist.deletedSuccess', 'Lista deletada com sucesso'));
      } catch (error) {
        const msg = error instanceof Error ? error.message : String(error);
        addToast(msg || t('common.error', 'Erro ao deletar'), 'error');
        announce(msg || t('common.error', 'Erro ao deletar'));
      }
    },
    [allTaskLists, requestConfirm, deleteTaskList, addToast, announce, t]
  );

  const handleCloneTaskList = useCallback(
    async (taskListId: number) => {
      const list = allTaskLists.find((l) => l.id === taskListId);
      const newTitle = `${list?.title || 'Lista'} (Cópia)`;

      try {
        const clonedTaskList = await cloneTaskList(taskListId, newTitle);
        if (clonedTaskList) {
          await executeDeepLink(
            { type: 'tab:open', tabType: 'tasklist', contentId: String(clonedTaskList.id), title: clonedTaskList.title },
            { navigate },
          );
          addToast(t('tasklist.clonedSuccess', 'Lista clonada com sucesso'), 'success');
          announce(t('tasklist.clonedSuccess', 'Lista clonada com sucesso'));
        }
      } catch (error) {
        const msg = error instanceof Error ? error.message : String(error);
        addToast(msg || t('common.error', 'Erro ao clonar'), 'error');
        announce(msg || t('common.error', 'Erro ao clonar'));
      }
    },
    [allTaskLists, cloneTaskList, navigate, addToast, announce, t]
  );

  // Ctrl+N: Create new list
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (isModalOpen()) return;
      if (!event.ctrlKey || event.shiftKey || event.altKey) return;
      if (event.key !== 'n' && event.key !== 'N') return;

      const target = event.target as HTMLElement | null;
      const isInput =
        target?.tagName === 'INPUT' ||
        target?.tagName === 'TEXTAREA' ||
        target?.isContentEditable;
      if (isInput) return;

      event.preventDefault();
      handleOpenEditor();
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [handleOpenEditor]);

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

  const handleSendToWorkspace = useCallback(async (taskListId: number, title: string, targetWorkspaceId: string) => {
    try {
      const tabId = await addTab('tasklist', String(taskListId), title);
      await moveTabToWorkspace(tabId, targetWorkspaceId);
      announce(t('tasklist.sentToWorkspace', 'Lista enviada ao workspace'));
    } catch (error) {
      console.error('Erro ao enviar lista ao workspace:', error);
    }
  }, [addTab, moveTabToWorkspace, announce, t]);

  const otherWorkspaces = useMemo(
    () => workspaces.filter(ws => !ws.is_active),
    [workspaces]
  );

  const getTaskListRowActions = useCallback(
    (list: TaskListRow) => {
      const actions = [
        {
          id: 'open',
          label: t('tasklist.open', 'Abrir'),
          icon: '📖',
          onClick: () => handleOpenTaskList(list.id),
        },
        {
          id: 'edit',
          label: t('tasklist.edit', 'Editar'),
          icon: '✏️',
          onClick: () => handleOpenEditor(list),
        },
        {
          id: 'clone',
          label: t('tasklist.clone', 'Clonar'),
          icon: '📋',
          onClick: () => handleCloneTaskList(list.id),
        },
      ];

      if (otherWorkspaces.length > 0) {
        actions.push({
          id: 'send-to-workspace',
          label: t('tasklist.sendToWorkspace', 'Enviar ao workspace'),
          icon: '📤',
          onClick: undefined as unknown as () => void,
          submenu: otherWorkspaces.map(ws => ({
            id: `ws-${ws.id}`,
            label: ws.name,
            icon: '📂',
            onClick: () => handleSendToWorkspace(list.id, list.title, ws.id),
          })),
        } as typeof actions[0] & { submenu: { id: string; label: string; icon: string; onClick: () => void }[] });
      }

      actions.push({
        id: 'delete',
        label: t('tasklist.delete', 'Deletar'),
        icon: '🗑️',
        onClick: () => handleDeleteTaskList(list.id),
        danger: true,
      } as typeof actions[0]);

      return actions;
    },
    [t, handleOpenTaskList, handleOpenEditor, handleCloneTaskList, handleDeleteTaskList, handleSendToWorkspace, otherWorkspaces]
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
      onClick: () => handleOpenEditor(),
      shortcut: 'Ctrl+N',
      variant: 'primary' as const,
    },
    {
      key: 'edit-list',
      label: t('tasklist.edit', 'Editar'),
      onClick: () => focusedTaskList && handleOpenEditor(focusedTaskList),
      disabled: !focusedTaskList,
    },
    {
      key: 'clone-list',
      label: t('tasklist.clone', 'Clonar'),
      onClick: () => focusedTaskList && handleCloneTaskList(focusedTaskList.id),
      disabled: !focusedTaskList,
    },
    {
      key: 'delete-list',
      label: t('tasklist.delete', 'Deletar'),
      onClick: () => focusedTaskList && handleDeleteTaskList(focusedTaskList.id),
      disabled: !focusedTaskList,
      variant: 'danger' as const,
    },
  ];

  return (
    <div className="tasklist-page">
      <Toolbar
        ref={toolbarRef}
        left={
          <h1 className="page-toolbar__title">
            {t('tasklist.allLists', 'Todas as Listas')}
          </h1>
        }
        searchPlaceholder={t('tasklist.search', 'Buscar listas...')}
        searchValue={searchTerm}
        onSearchChange={hasLists ? setSearchTerm : undefined}
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
          <p className="tasklist-empty-hint">{t('tasklist.createNewHint', 'Use Ctrl+N ou clique em "Nova Lista" acima')}</p>
        </div>
      )}

      <Modal isOpen={editorOpen} onClose={handleCloseEditor} title={
        editorMode === 'create' ? t('tasklist.createNew', 'Criar Nova Lista') : t('tasklist.editList', 'Editar Lista')
      }>
        <form className="tasklist-editor-form" onSubmit={(e) => {
          e.preventDefault();
          handleSaveTaskList();
        }}>
          <FormField label={t('tasklist.title', 'Título')} id="edit-title">
            <Input
              value={editTitle}
              onChange={(e) => setEditTitle(e.target.value)}
              placeholder={t('tasklist.titlePlaceholder', 'Título da lista')}
              autoFocus
            />
          </FormField>

          <FormField label={t('tasklist.description', 'Descrição')} id="edit-description">
            <Textarea
              value={editDescription}
              onChange={(e) => setEditDescription(e.target.value)}
              placeholder={t('common.description', 'Descrição (opcional)')}
              rows={4}
            />
          </FormField>

          <EditorPanelFooter className="tasklist-editor__footer">
            <Button onClick={handleSaveTaskList} loading={editingLoading}>
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
