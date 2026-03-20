import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../store/taskListStore';
import { useNavigationStore } from '../store/navigationStore';
import { Tabs, TabList, Tab, TabPanel } from '../components/ui/tabs';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { Button } from '../components/ui/Button';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { MenuButton } from '../components/layout/MenuButton';
import { EditorPanelFooter } from '../components/ui/EditorPanel';
import { FormField } from '../components/ui/FormField';
import { Input } from '../components/ui/Input';
import { Textarea } from '../components/ui/Textarea';
import TasksTable, { type TasksTableRef } from '../components/taskLists/TasksTable';
import KanbanBoard, { type KanbanBoardRef } from '../components/taskLists/KanbanBoard';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useGridFocus } from '../hooks/useGridFocus';
import { useConfirm } from '../hooks/useConfirm';
import { useAnchoredContextMenu } from '../hooks/useAnchoredContextMenu';
import { useUIStore } from '../store/uiStore';
import { TaskListHistoryPicker } from '../components/pickers/TaskListHistoryPicker';
import { ContextMenu } from '../components/menu';
import type { MenuItem } from '../components/menu';
import type { ViewMode, TaskListWithWorkflow } from '../types/tasklist';
import './TaskListsPage.css';

type ActiveTab = 'home' | number;

interface TaskListRow extends TaskListWithWorkflow {
  id: number;
}

export default function TaskListsPage() {
  const { t } = useTranslation();
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  const requestConfirm = useConfirm();
  useGridPageLandmarks({ pageClass: 'tasklist-page' });

  // Zustand store
  const openTabs = useTaskListStore((state) => state.openTabs);
  const taskLists = useTaskListStore((state) => state.taskLists);
  const {
    loadTabs,
    closeTab,
    createTaskList,
    createTab,
    deleteTaskList,
    cloneTaskList,
    setViewMode,
    getCachedTaskList,
    loadTaskList,
  } = useTaskListStore();

  // Local state - Grid
  const [activeTab, setActiveTab] = useState<ActiveTab>('home');
  const [searchTerm, setSearchTerm] = useState('');

  // Local state - Editor
  const [editorOpen, setEditorOpen] = useState(false);
  const [editTitle, setEditTitle] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [editorMode, setEditorMode] = useState<'create' | 'edit'>('create');
  const [editingLoading, setEditingLoading] = useState(false);

  // Carrega abas persistidas do backend na inicialização
  const tabsLoadedRef = useRef(false);
  useEffect(() => {
    if (tabsLoadedRef.current) return;
    tabsLoadedRef.current = true;
    void loadTabs().then(() => {
      // Restaura aba ativa da store
      const tabs = useTaskListStore.getState().openTabs;
      const activeBackendTab = tabs.find((t) => t.isActive);
      if (activeBackendTab) {
        setActiveTab(activeBackendTab.taskListId);
      }
    });
  }, [loadTabs]);

  // Deep link: consome pendingEdit para abrir uma tasklist específica
  useEffect(() => {
    const pending = useNavigationStore.getState().consumeResourceEdit('tasklists');
    if (pending && pending.id) {
      const taskListId = Number(pending.id);
      if (taskListId > 0) {
        const isOpen = useTaskListStore.getState().openTabs.some((t) => t.taskListId === taskListId);
        if (!isOpen) {
          void createTab(taskListId);
        }
        setActiveTab(taskListId);
      }
    }
  }, [createTab]);

  // Local state - Focused item (for toolbar actions)
  const [focusedTaskList, setFocusedTaskList] = useState<TaskListRow | null>(null);

  // Context menu para tabs (right-click)
  const { menu: tabContextMenu, openAtPoint: openTabMenu, closeMenu: closeTabMenu, onSelectItem: onTabMenuSelect } = useAnchoredContextMenu();
  const tabContextTargetRef = useRef<number | null>(null);

  // Refs para TasksTable e landmarks
  const tasksTableRefs = useRef<Map<number, TasksTableRef | KanbanBoardRef>>(new Map());
  const toolbarRef = useRef<HTMLDivElement>(null);
  const tabsRef = useRef<HTMLDivElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);

  // Stable callbacks for DataGrid (same pattern as ProvidersPage)
  const getRowId = useCallback((item: TaskListRow) => item.id, []);
  const handleFocusChange = useCallback((item: TaskListRow | null) => setFocusedTaskList(item), []);

  // Pega a TaskList atual (quando não estiver na home)
  const currentTaskListId = activeTab !== 'home' ? (activeTab as number) : null;
  const currentTaskList = currentTaskListId ? getCachedTaskList(currentTaskListId) : null;
  const currentViewMode: ViewMode = currentTaskList?.preferredViewMode || 'list';

  // Auto-load tasklist data quando aba ativa não está no cache
  useEffect(() => {
    if (currentTaskListId !== null && !taskLists.has(currentTaskListId)) {
      void loadTaskList(currentTaskListId);
    }
  }, [currentTaskListId, taskLists, loadTaskList]);

  // ── Grid Data ───────────────────────────────────────────────────
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

  // ── Editor Handlers ─────────────────────────────────────────────

  const handleOpenEditor = useCallback((list?: TaskListRow) => {
    if (list) {
      // Editar
      setEditorMode('edit');
      setEditTitle(list.title);
      setEditDescription(list.description || '');
    } else {
      // Criar
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
          await createTab(newTaskList.id);
          setActiveTab(newTaskList.id);
          addToast(t('tasklist.createdSuccess', `Lista "${editTitle}" criada com sucesso!`), 'success');
          announce(t('tasklist.createdSuccess', `Lista "${editTitle}" criada com sucesso!`));
        }
      } else {
        // Editar (quando implementado no backend)
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
  }, [editTitle, editDescription, editorMode, createTaskList, createTab, addToast, announce, handleCloseEditor, t]);

  // ── Row Actions ──────────────────────────────────────────────────

  const handleOpenTaskList = useCallback(async (taskListId: number) => {
    const isOpen = openTabs.some((tab) => tab.taskListId === taskListId);
    if (!isOpen) {
      await createTab(taskListId);
    }
    // Garante que os dados estejam no cache antes de exibir
    if (!getCachedTaskList(taskListId)) {
      await loadTaskList(taskListId);
    }
    setActiveTab(taskListId);
    announce(t('tasklist.opened', 'Lista aberta'));
  }, [openTabs, createTab, getCachedTaskList, loadTaskList, announce, t]);

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
        const tabIndex = openTabs.findIndex((tab) => tab.taskListId === taskListId);
        if (tabIndex !== -1) {
          closeTab(openTabs[tabIndex].id);
          setActiveTab('home');
        }
        addToast(t('tasklist.deletedSuccess', 'Lista deletada com sucesso'), 'success');
        announce(t('tasklist.deletedSuccess', 'Lista deletada com sucesso'));
      } catch (error) {
        const msg = error instanceof Error ? error.message : String(error);
        addToast(msg || t('common.error', 'Erro ao deletar'), 'error');
        announce(msg || t('common.error', 'Erro ao deletar'));
      }
    },
    [allTaskLists, openTabs, requestConfirm, deleteTaskList, closeTab, addToast, announce, t]
  );

  const handleCloneTaskList = useCallback(
    async (taskListId: number) => {
      const list = allTaskLists.find((l) => l.id === taskListId);
      const newTitle = `${list?.title || 'Lista'} (Cópia)`;

      try {
        const clonedTaskList = await cloneTaskList(taskListId, newTitle);
        if (clonedTaskList) {
          createTab(clonedTaskList.id);
          setActiveTab(clonedTaskList.id);
          addToast(t('tasklist.clonedSuccess', 'Lista clonada com sucesso'), 'success');
          announce(t('tasklist.clonedSuccess', 'Lista clonada com sucesso'));
        }
      } catch (error) {
        const msg = error instanceof Error ? error.message : String(error);
        addToast(msg || t('common.error', 'Erro ao clonar'), 'error');
        announce(msg || t('common.error', 'Erro ao clonar'));
      }
    },
    [allTaskLists, cloneTaskList, createTab, addToast, announce, t]
  );

  const handleToggleViewMode = useCallback(
    async (taskListId: number) => {
      if (!currentTaskList) return;
      const newMode: ViewMode = currentViewMode === 'list' ? 'kanban' : 'list';
      try {
        await setViewMode(taskListId, newMode);
        announce(
          t('tasklist.viewModeChanged', `Alterado para visualização ${newMode === 'list' ? 'Lista' : 'Kanban'}`)
        );
      } catch (error) {
        const msg = error instanceof Error ? error.message : String(error);
        addToast(msg || t('common.error', 'Erro ao alterar visualização'), 'error');
      }
    },
    [currentTaskList, currentViewMode, setViewMode, announce, addToast, t]
  );

  // ── History Picker Handler ──────────────────────────────────────

  const handleHistorySelect = useCallback(
    async (taskListId: number) => {
      const isOpen = openTabs.some((tab) => tab.taskListId === taskListId);
      if (!isOpen) {
        await createTab(taskListId);
      }
      // Garante que os dados estejam no cache
      if (!getCachedTaskList(taskListId)) {
        await loadTaskList(taskListId);
      }
      setActiveTab(taskListId);
      announce(t('tasklist.opened', 'Lista aberta'));
    },
    [openTabs, createTab, getCachedTaskList, loadTaskList, announce, t]
  );

  // ── Tab Context Menu ────────────────────────────────────────────

  const handleTabContextMenu = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>, tabId: number) => {
      e.preventDefault();
      tabContextTargetRef.current = tabId;

      const items: MenuItem[] = [
        {
          id: 'close',
          label: t('tasklist.tabClose', 'Fechar'),
          action: () => {
            const tab = openTabs.find((t) => t.taskListId === tabId);
            if (tab) {
              closeTab(tab.id);
              if (activeTab === tabId) setActiveTab('home');
              announce(t('tasklist.tabClosed', 'Aba fechada'));
            }
          },
        },
        {
          id: 'closeOthers',
          label: t('tasklist.tabCloseOthers', 'Fechar outras'),
          disabled: openTabs.length <= 1,
          action: () => {
            openTabs.forEach((tab) => {
              if (tab.taskListId !== tabId) closeTab(tab.id);
            });
            setActiveTab(tabId);
            announce(t('tasklist.tabClosedOthers', 'Outras abas fechadas'));
          },
        },
        {
          id: 'closeAll',
          label: t('tasklist.tabCloseAll', 'Fechar todas'),
          action: () => {
            openTabs.forEach((tab) => closeTab(tab.id));
            setActiveTab('home');
            announce(t('tasklist.tabClosedAll', 'Todas as abas fechadas'));
          },
        },
      ];

      openTabMenu(e.clientX, e.clientY, t('tasklist.tabContextMenu', 'Menu da aba'), items);
    },
    [openTabs, closeTab, activeTab, announce, openTabMenu, t]
  );

  const handleOpenCreateTask = useCallback(() => {
    if (currentTaskListId === null) return;
    const ref = tasksTableRefs.current.get(currentTaskListId);
    if (ref) {
      ref.openCreateModal();
    }
  }, [currentTaskListId]);

  // ── Keyboard Shortcuts ──────────────────────────────────────────

  // Ctrl+N: Create new list (HOME tab) or new task (detail tab)
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

      // Se está na aba HOME, abre criar nova lista
      if (activeTab === 'home') {
        handleOpenEditor();
      } else {
        // Se está em uma lista específica, abre criar nova task
        const tabId = activeTab as number;
        const ref = tasksTableRefs.current.get(tabId);
        if (ref) {
          ref.openCreateModal();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [activeTab, handleOpenEditor, tasksTableRefs]);

  // Close editor with Esc
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

  // Reset focused list when switching tabs (skip initial mount)
  const prevActiveTabRef = useRef(activeTab);
  useEffect(() => {
    if (prevActiveTabRef.current !== activeTab) {
      prevActiveTabRef.current = activeTab;
      setFocusedTaskList(null);
    }
  }, [activeTab]);

  // Reset focused list when grid becomes empty (e.g. after deleting all lists)
  useEffect(() => {
    if (filteredTaskLists.length === 0) {
      setFocusedTaskList(null);
    }
  }, [filteredTaskLists.length]);

  // Tab navigation shortcuts (Ctrl+W, Ctrl+Tab, Ctrl+Shift+Tab, Ctrl+PageUp/Down)
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (isModalOpen()) return;

      const tabs = [
        { id: 'home' as const, label: t('tasklist.lists', 'Listas') },
        ...openTabs.map((tab) => ({
          id: tab.taskListId,
          label: tab.title,
        })),
      ];

      const currentIndex = tabs.findIndex((t) => t.id === activeTab);

      // Ctrl+W: Fechar aba atual
      if (event.ctrlKey && event.key === 'w' && !event.shiftKey && !event.altKey) {
        event.preventDefault();
        if (activeTab !== 'home') {
          const tabToClose = openTabs.find((t) => t.taskListId === activeTab);
          if (tabToClose) {
            closeTab(tabToClose.id);
            // Volta para home depois de fechar
            setActiveTab('home');
            announce(t('tasklist.tabClosed', 'Aba fechada'));
          }
        }
        return;
      }

      // Ctrl+Tab: Próxima aba (circular)
      if (event.ctrlKey && event.key === 'Tab' && !event.shiftKey && !event.altKey) {
        event.preventDefault();
        if (tabs.length > 1 && currentIndex !== -1) {
          const nextIndex = currentIndex < tabs.length - 1 ? currentIndex + 1 : 0;
          const nextTab = tabs[nextIndex];
          setActiveTab(nextTab.id);
          announce(`${nextTab.label}, ${nextIndex + 1} de ${tabs.length}`);
        }
        return;
      }

      // Ctrl+Shift+Tab: Aba anterior (circular)
      if (event.ctrlKey && event.key === 'Tab' && event.shiftKey && !event.altKey) {
        event.preventDefault();
        if (tabs.length > 1 && currentIndex !== -1) {
          const prevIndex = currentIndex > 0 ? currentIndex - 1 : tabs.length - 1;
          const prevTab = tabs[prevIndex];
          setActiveTab(prevTab.id);
          announce(`${prevTab.label}, ${prevIndex + 1} de ${tabs.length}`);
        }
        return;
      }

      // Ctrl+PageDown: Próxima aba (redundância para Ctrl+Tab)
      if (event.ctrlKey && event.key === 'PageDown' && !event.shiftKey) {
        event.preventDefault();
        if (tabs.length > 1 && currentIndex !== -1) {
          const nextIndex = currentIndex < tabs.length - 1 ? currentIndex + 1 : 0;
          const nextTab = tabs[nextIndex];
          setActiveTab(nextTab.id);
          announce(`${nextTab.label}, ${nextIndex + 1} de ${tabs.length}`);
        }
        return;
      }

      // Ctrl+PageUp: Aba anterior (redundância para Ctrl+Shift+Tab)
      if (event.ctrlKey && event.key === 'PageUp' && !event.shiftKey) {
        event.preventDefault();
        if (tabs.length > 1 && currentIndex !== -1) {
          const prevIndex = currentIndex > 0 ? currentIndex - 1 : tabs.length - 1;
          const prevTab = tabs[prevIndex];
          setActiveTab(prevTab.id);
          announce(`${prevTab.label}, ${prevIndex + 1} de ${tabs.length}`);
        }
        return;
      }

      // Ctrl+1-9: Ir para aba N
      if (event.ctrlKey && !event.shiftKey && !event.altKey) {
        const num = parseInt(event.key, 10);
        if (num >= 1 && num <= 9) {
          event.preventDefault();
          const targetTab = tabs[num - 1];
          if (targetTab) {
            setActiveTab(targetTab.id);
            announce(`${targetTab.label}, ${num} de ${tabs.length}`);
          }
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [activeTab, openTabs, closeTab, announce, t]);

  // ── Rendering - DataGrid Columns ───────────────────────────────

  const getTaskListRowActions = useCallback(
    (list: TaskListRow) => [
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
      {
        id: 'delete',
        label: t('tasklist.delete', 'Deletar'),
        icon: '🗑️',
        onClick: () => handleDeleteTaskList(list.id),
        danger: true,
      },
    ],
    [t, handleOpenTaskList, handleOpenEditor, handleCloneTaskList, handleDeleteTaskList]
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

  // ── Rendering - HOME Tab Content ────────────────────────────────────
  // Note: F6 landmark navigation is handled by useGridPageLandmarks hook above

  const renderHomeContent = () => {
    const hasLists = filteredTaskLists.length > 0;

    // Toolbar actions for HOME tab (same pattern as ProvidersPage)
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
      <div className="tasklist-home">
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
      </div>
    );
  };

  // ── Rendering - Open TaskList Tab Content ────────────────────

  const renderTaskListContent = (tabId: number) => {
    const taskList = getCachedTaskList(tabId);
    if (!taskList) {
      return <div className="tasklist-loading">{t('tasklist.loading', 'Carregando...')}</div>;
    }

    const tasks = taskList.tasks || [];
    const hasTasks = tasks.length > 0;

    return (
      <div className="tasklist-detail">
        <Toolbar
          left={
            <h1 className="page-toolbar__title">
              {taskList.title}
            </h1>
          }
          actions={[
            {
              key: 'new-task',
              label: t('tasklist.createTask', 'Nova Tarefa'),
              icon: '➕',
              onClick: handleOpenCreateTask,
              shortcut: 'Ctrl+N',
              variant: 'primary',
            },
            ...(hasTasks
              ? [
                  {
                    key: 'toggle-view',
                    label: currentViewMode === 'list' ? 'Kanban' : 'Lista',
                    icon: currentViewMode === 'list' ? '🎯' : '📋',
                    onClick: () => handleToggleViewMode(tabId),
                    variant: 'secondary' as const,
                  },
                ]
              : []),
            {
              key: 'clone-list',
              label: t('tasklist.cloneList', 'Clonar Lista'),
              icon: '📋',
              onClick: () => handleCloneTaskList(tabId),
              variant: 'secondary' as const,
            },
            {
              key: 'delete-list',
              label: t('tasklist.deleteList', 'Deletar Lista'),
              icon: '🗑️',
              onClick: () => handleDeleteTaskList(tabId),
              variant: 'danger' as const,
            },
          ]}
        />

        <div ref={gridRef}>
          {currentViewMode === 'kanban' ? (
            <KanbanBoard
              ref={(r) => {
                if (r) {
                  tasksTableRefs.current.set(tabId, r);
                } else {
                  tasksTableRefs.current.delete(tabId);
                }
              }}
              taskListId={tabId}
              tasks={tasks}
              taskList={taskList}
              onTaskCreated={(_task) => {
                // Sincronizado via evento Wails
              }}
              onTaskUpdated={(_task) => {
                // Sincronizado via evento Wails
              }}
              onTaskDeleted={(_taskId) => {
                // Sincronizado via evento Wails
              }}
            />
          ) : (
            <TasksTable
              ref={(r) => {
                if (r) {
                  tasksTableRefs.current.set(tabId, r);
                } else {
                  tasksTableRefs.current.delete(tabId);
                }
              }}
              taskListId={tabId}
              tasks={tasks}
              taskList={taskList}
              onTaskCreated={(_task) => {
                // Sincronizado via evento Wails
              }}
              onTaskUpdated={(_task) => {
                // Sincronizado via evento Wails
              }}
              onTaskDeleted={(_taskId) => {
                // Sincronizado via evento Wails
              }}
            />
          )}
        </div>
      </div>
    );
  };

  // ── Main Render ────────────────────────────────────────────────

  const tabs: Array<{ id: 'home' | number; label: string }> = useMemo(
    () => [
      { id: 'home', label: t('tasklist.lists', 'Listas') },
      ...openTabs.map((tab) => ({
        id: tab.taskListId,
        label: tab.title,
      })),
    ],
    [openTabs, t]
  );

  return (
    <div className="tasklist-page">
      <Tabs
        value={String(activeTab)}
        onValueChange={(value) => setActiveTab(value === 'home' ? 'home' : Number(value))}
      >
        <div ref={tabsRef} className="tasklist-page__tabs-row">
          <TabList ariaLabel={t('tasklist.listNavigation', 'Navegação de listas')}>
            {tabs.map((tab) => (
              <Tab
                key={tab.id}
                value={String(tab.id)}
                onContextMenu={
                  tab.id !== 'home'
                    ? (e) => handleTabContextMenu(e, tab.id as number)
                    : undefined
                }
              >
                {tab.label}
              </Tab>
            ))}
          </TabList>
          <div className="tasklist-page__history-picker">
            <TaskListHistoryPicker
              value={currentTaskListId ?? undefined}
              onChange={(id) => handleHistorySelect(id)}
              onAnnounce={announce}
            />
          </div>
        </div>

        {/* HOME Tab */}
        <TabPanel value="home">{activeTab === 'home' && renderHomeContent()}</TabPanel>

        {/* Open TaskLists Tabs */}
        {openTabs.map((tab) => (
          <TabPanel key={tab.taskListId} value={String(tab.taskListId)}>
            {activeTab === tab.taskListId && renderTaskListContent(tab.taskListId)}
          </TabPanel>
        ))}
      </Tabs>

      {/* Tab Context Menu */}
      <ContextMenu
        {...tabContextMenu}
        onClose={closeTabMenu}
        onSelect={onTabMenuSelect}
      />

      {/* Create/Edit TaskList Modal */}
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
