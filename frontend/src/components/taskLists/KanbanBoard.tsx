import {
  useState,
  useRef,
  useCallback,
  useEffect,
  useMemo,
  forwardRef,
  useImperativeHandle,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useTaskListStore } from '../../store/taskListStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { ContextMenu } from '../menu';
import { Modal } from '../ui/Modal';
import TaskForm from './TaskForm';
import type { Task, TaskListWithWorkflow } from '../../types/tasklist';
import './KanbanBoard.css';

/* ── Tipos ─────────────────────────────────────────────────────── */

interface KanbanBoardProps {
  taskListId: number;
  tasks: Task[];
  taskList: TaskListWithWorkflow;
  onTaskCreated?: (task: Task) => void;
  onTaskUpdated?: (task: Task) => void;
  onTaskDeleted?: (taskId: number) => void;
}

export interface KanbanBoardRef {
  openCreateModal: () => void;
}

interface FocusPos {
  col: number;
  row: number;
}

/* ── Componente ────────────────────────────────────────────────── */

const KanbanBoard = forwardRef<KanbanBoardRef, KanbanBoardProps>(function KanbanBoard(
  { taskListId, tasks, taskList, onTaskCreated, onTaskUpdated, onTaskDeleted },
  ref,
) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const { updateTaskStatus, reorderTasks, deleteTask } = useTaskListStore();

  // ── Estado ─────────────────────────────────────────────────
  const [focusPos, setFocusPos] = useState<FocusPos>({ col: 0, row: 0 });
  const [grabbedTask, setGrabbedTask] = useState<Task | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);

  // ── Refs ───────────────────────────────────────────────────
  const boardRef = useRef<HTMLDivElement>(null);
  const cardRefs = useRef<Map<string, HTMLDivElement>>(new Map());

  // ── Context menu ───────────────────────────────────────────
  const {
    menu: contextMenu,
    openForTrigger,
    closeMenu,
    onSelectItem,
  } = useAnchoredContextMenu();

  // ── Dados derivados ────────────────────────────────────────
  const statuses = useMemo(
    () => [...(taskList.workflow?.statuses ?? [])].sort((a, b) => a.order - b.order),
    [taskList.workflow?.statuses],
  );

  const tasksByStatus = useMemo(() => {
    const map = new Map<number, Task[]>();
    for (const status of statuses) {
      map.set(status.id, []);
    }
    const fallbackStatusId = statuses[0]?.id;
    for (const task of tasks) {
      let arr = map.get(task.statusId);
      if (!arr && fallbackStatusId !== undefined) {
        arr = map.get(fallbackStatusId);
      }
      if (arr) {
        arr.push(task);
      }
    }
    // Ordena cada coluna pelo campo order
    for (const arr of map.values()) {
      arr.sort((a, b) => a.order - b.order);
    }
    return map;
  }, [tasks, statuses]);

  const getColumnTasks = useCallback(
    (colIdx: number): Task[] => tasksByStatus.get(statuses[colIdx]?.id) ?? [],
    [tasksByStatus, statuses],
  );

  // ── Imperative handle para criar tarefa ────────────────────
  useImperativeHandle(ref, () => ({
    openCreateModal: () => {
      setEditingTask(null);
      setIsCreateModalOpen(true);
    },
  }));

  // ── Focus management ───────────────────────────────────────
  const focusCard = useCallback((col: number, row: number) => {
    const key = `${col}-${row}`;
    const el = cardRefs.current.get(key);
    if (el) {
      el.focus();
    }
  }, []);

  // Após alterar focusPos, mover foco real
  useEffect(() => {
    focusCard(focusPos.col, focusPos.row);
  }, [focusPos, focusCard]);

  // ── Announce card info ─────────────────────────────────────
  const announceCard = useCallback(
    (task: Task, colIdx: number, rowIdx: number) => {
      const status = statuses[colIdx];
      const columnTasks = getColumnTasks(colIdx);
      const posInfo = `${rowIdx + 1} ${t('tasklist.kanban.of', 'de')} ${columnTasks.length}`;
      const statusInfo = `${status?.icon ?? ''} ${status?.label ?? ''}`.trim();
      announce(`${task.title}. ${statusInfo}, ${posInfo}`, 'assertive');
    },
    [statuses, getColumnTasks, announce, t],
  );

  // ── Ações de tarefa ────────────────────────────────────────
  const moveTaskToColumn = useCallback(
    async (task: Task, targetColIdx: number) => {
      const targetStatus = statuses[targetColIdx];
      if (!targetStatus || task.statusId === targetStatus.id) return;

      try {
        await updateTaskStatus(task.id, targetStatus.id);
        announce(
          t('tasklist.kanban.movedToColumn', 'Movido para {{column}}', {
            column: targetStatus.label,
          }),
          'assertive',
        );
      } catch {
        announce(t('tasklist.kanban.moveFailed', 'Não foi possível mover'), 'assertive');
      }
    },
    [statuses, updateTaskStatus, announce, t],
  );

  const reorderInColumn = useCallback(
    async (task: Task, colIdx: number, direction: -1 | 1) => {
      const columnTasks = getColumnTasks(colIdx);
      const currentIdx = columnTasks.findIndex((t) => t.id === task.id);
      const newIdx = currentIdx + direction;
      if (newIdx < 0 || newIdx >= columnTasks.length) return;

      // Swap localmente e enviar nova ordem
      const newOrder = [...columnTasks];
      [newOrder[currentIdx], newOrder[newIdx]] = [newOrder[newIdx], newOrder[currentIdx]];
      const orderedIds = newOrder.map((t) => t.id);

      const status = statuses[colIdx];
      try {
        await reorderTasks(taskListId, status.id, orderedIds);
        setFocusPos({ col: colIdx, row: newIdx });
        announce(
          t('tasklist.kanban.reordered', 'Reordenado para posição {{pos}}', {
            pos: String(newIdx + 1),
          }),
          'assertive',
        );
      } catch {
        announce(t('tasklist.kanban.reorderFailed', 'Não foi possível reordenar'), 'assertive');
      }
    },
    [getColumnTasks, statuses, reorderTasks, taskListId, announce, t],
  );

  const handleDeleteTask = useCallback(
    async (task: Task) => {
      try {
        await deleteTask(task.id);
        onTaskDeleted?.(task.id);
        announce(t('tasklist.taskDeleted', 'Tarefa deletada com sucesso'), 'assertive');
      } catch {
        announce(t('common.error', 'Erro'), 'assertive');
      }
    },
    [deleteTask, onTaskDeleted, announce, t],
  );

  // ── Modais ─────────────────────────────────────────────────
  const handleCloseModals = useCallback(() => {
    setIsCreateModalOpen(false);
    setIsEditModalOpen(false);
    setEditingTask(null);
  }, []);

  const handleTaskCreated = useCallback(
    (task: Task) => {
      handleCloseModals();
      onTaskCreated?.(task);
    },
    [handleCloseModals, onTaskCreated],
  );

  const handleTaskUpdated = useCallback(
    (task: Task) => {
      handleCloseModals();
      onTaskUpdated?.(task);
    },
    [handleCloseModals, onTaskUpdated],
  );

  // ── Context menu para card ─────────────────────────────────
  const openCardContextMenu = useCallback(
    (task: Task, _colIdx: number, trigger: HTMLElement) => {
      const items = [
        {
          id: 'edit',
          label: t('tasklist.edit', 'Editar'),
          action: () => {
            setEditingTask(task);
            setIsEditModalOpen(true);
          },
        },
        { separator: true, id: 'sep-1' },
        // Opções de mover para outra coluna
        ...statuses
          .filter((s) => s.id !== task.statusId)
          .map((s) => ({
            id: `move-${s.id}`,
            label: t('tasklist.kanban.moveTo', 'Mover para {{column}}', { column: s.label }),
            action: () => {
              const targetIdx = statuses.findIndex((st) => st.id === s.id);
              moveTaskToColumn(task, targetIdx);
            },
          })),
        { separator: true, id: 'sep-2' },
        {
          id: 'delete',
          label: t('tasklist.delete', 'Deletar'),
          danger: true,
          action: () => handleDeleteTask(task),
        },
      ];

      openForTrigger(trigger, t('tasklist.kanban.cardMenu', 'Menu do card'), items);
    },
    [statuses, moveTaskToColumn, handleDeleteTask, openForTrigger, t],
  );

  // ── Inline rename (F2) ────────────────────────────────────
  const [renamingTaskId, setRenamingTaskId] = useState<number | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);
  const { updateTask } = useTaskListStore();

  const startRename = useCallback((task: Task) => {
    setRenamingTaskId(task.id);
    setRenameValue(task.title);
  }, []);

  const commitRename = useCallback(async () => {
    if (renamingTaskId === null) return;
    const trimmed = renameValue.trim();
    if (!trimmed) {
      setRenamingTaskId(null);
      return;
    }
    try {
      await updateTask(renamingTaskId, trimmed);
      announce(t('tasklist.taskUpdated', 'Tarefa atualizada com sucesso'), 'assertive');
    } catch {
      announce(t('common.error', 'Erro'), 'assertive');
    }
    setRenamingTaskId(null);
  }, [renamingTaskId, renameValue, updateTask, announce, t]);

  const cancelRename = useCallback(() => {
    setRenamingTaskId(null);
  }, []);

  useEffect(() => {
    if (renamingTaskId !== null) {
      renameInputRef.current?.focus();
      renameInputRef.current?.select();
    }
  }, [renamingTaskId]);

  // ── Keyboard handler (setas, atalhos) ──────────────────────
  const handleBoardKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const { col, row } = focusPos;
      const columnTasks = getColumnTasks(col);
      const currentTask = columnTasks[row];

      switch (e.key) {
        // ── Navegação entre colunas ──
        case 'ArrowLeft': {
          e.preventDefault();
          if (col > 0) {
            const newCol = col - 1;
            const targetTasks = getColumnTasks(newCol);
            const newRow = Math.min(row, Math.max(0, targetTasks.length - 1));
            setFocusPos({ col: newCol, row: newRow });

            if (grabbedTask) {
              moveTaskToColumn(grabbedTask, newCol);
            } else {
              const task = targetTasks[newRow];
              if (task) announceCard(task, newCol, newRow);
              else announce(statuses[newCol]?.label ?? '', 'assertive');
            }
          }
          break;
        }
        case 'ArrowRight': {
          e.preventDefault();
          if (col < statuses.length - 1) {
            const newCol = col + 1;
            const targetTasks = getColumnTasks(newCol);
            const newRow = Math.min(row, Math.max(0, targetTasks.length - 1));
            setFocusPos({ col: newCol, row: newRow });

            if (grabbedTask) {
              moveTaskToColumn(grabbedTask, newCol);
            } else {
              const task = targetTasks[newRow];
              if (task) announceCard(task, newCol, newRow);
              else announce(statuses[newCol]?.label ?? '', 'assertive');
            }
          }
          break;
        }

        // ── Navegação entre cards ──
        case 'ArrowUp': {
          e.preventDefault();
          if (e.altKey && currentTask) {
            // Alt+Up: reordenar para cima
            reorderInColumn(currentTask, col, -1);
          } else if (row > 0) {
            const newRow = row - 1;
            setFocusPos({ col, row: newRow });
            const task = columnTasks[newRow];
            if (task) announceCard(task, col, newRow);
          }
          break;
        }
        case 'ArrowDown': {
          e.preventDefault();
          if (e.altKey && currentTask) {
            // Alt+Down: reordenar para baixo
            reorderInColumn(currentTask, col, 1);
          } else if (row < columnTasks.length - 1) {
            const newRow = row + 1;
            setFocusPos({ col, row: newRow });
            const task = columnTasks[newRow];
            if (task) announceCard(task, col, newRow);
          }
          break;
        }

        // ── Grab/Drop com Space ──
        case ' ': {
          e.preventDefault();
          if (!currentTask) break;

          if (grabbedTask) {
            // Soltar
            setGrabbedTask(null);
            announce(
              t('tasklist.kanban.dropped', '{{name}} solto', { name: currentTask.title }),
              'assertive',
            );
          } else {
            // Agarrar
            setGrabbedTask(currentTask);
            announce(
              t('tasklist.kanban.grabbed', '{{name}} selecionado. Use setas para mover, Espaço para soltar', {
                name: currentTask.title,
              }),
              'assertive',
            );
          }
          break;
        }

        // ── Escape: cancelar grab ──
        case 'Escape': {
          if (grabbedTask) {
            e.preventDefault();
            setGrabbedTask(null);
            announce(t('tasklist.kanban.grabCancelled', 'Movimentação cancelada'), 'assertive');
          }
          break;
        }

        // ── Delete: apagar card ──
        case 'Delete': {
          e.preventDefault();
          if (currentTask) {
            handleDeleteTask(currentTask);
          }
          break;
        }

        // ── F2: renomear ──
        case 'F2': {
          e.preventDefault();
          if (currentTask) {
            startRename(currentTask);
          }
          break;
        }

        // ── Enter / Shift+F10: context menu ──
        case 'Enter':
        case 'ContextMenu': {
          e.preventDefault();
          if (currentTask) {
            const key = `${col}-${row}`;
            const el = cardRefs.current.get(key);
            if (el) openCardContextMenu(currentTask, col, el);
          }
          break;
        }
        case 'F10': {
          if (e.shiftKey && currentTask) {
            e.preventDefault();
            const key = `${col}-${row}`;
            const el = cardRefs.current.get(key);
            if (el) openCardContextMenu(currentTask, col, el);
          }
          break;
        }
      }
    },
    [
      focusPos, getColumnTasks, statuses, grabbedTask,
      moveTaskToColumn, reorderInColumn, handleDeleteTask,
      startRename, openCardContextMenu, announceCard,
      announce, t,
    ],
  );

  // Alt+Arrow para mover de coluna sem grab
  const handleCardKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>, task: Task, colIdx: number) => {
      if (e.altKey && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
        e.preventDefault();
        e.stopPropagation();
        const direction = e.key === 'ArrowLeft' ? -1 : 1;
        const targetCol = colIdx + direction;
        if (targetCol >= 0 && targetCol < statuses.length) {
          moveTaskToColumn(task, targetCol);
          setFocusPos((prev) => ({
            col: targetCol,
            row: Math.min(prev.row, Math.max(0, getColumnTasks(targetCol).length)),
          }));
        }
      }
    },
    [statuses, moveTaskToColumn, getColumnTasks],
  );

  // ── Drag & Drop (visual) ──────────────────────────────────
  const [dragOverCol, setDragOverCol] = useState<number | null>(null);

  const handleDragStart = useCallback((e: React.DragEvent, task: Task) => {
    e.dataTransfer.setData('text/plain', String(task.id));
    e.dataTransfer.effectAllowed = 'move';
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
  }, []);

  const handleDragEnterColumn = useCallback((_e: React.DragEvent, colIdx: number) => {
    setDragOverCol(colIdx);
  }, []);

  const handleDragLeaveColumn = useCallback(() => {
    setDragOverCol(null);
  }, []);

  const handleDropOnColumn = useCallback(
    async (e: React.DragEvent, colIdx: number) => {
      e.preventDefault();
      setDragOverCol(null);
      const taskId = Number(e.dataTransfer.getData('text/plain'));
      if (!taskId) return;

      const task = tasks.find((t) => t.id === taskId);
      if (task) {
        await moveTaskToColumn(task, colIdx);
      }
    },
    [tasks, moveTaskToColumn],
  );

  // ── Tab trap: Tab entra, próximo Tab sai ───────────────────
  const handleBoardTabKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Tab' && !e.shiftKey) {
        // Tab sai do board
        // Comportamento padrão: foco vai para o próximo elemento focável fora do board
        return;
      }
    },
    [],
  );

  // ── Render ─────────────────────────────────────────────────

  if (!statuses.length) {
    return (
      <>
        <div className="kanban-empty" role="status">
          {t('tasklist.noWorkflow', 'Sem workflow definido')}
        </div>
        <Modal
          isOpen={isCreateModalOpen}
          onClose={handleCloseModals}
          title={t('tasklist.createTask', 'Criar Tarefa')}
        >
          <TaskForm taskListId={taskListId} onSuccess={handleTaskCreated} onCancel={handleCloseModals} />
        </Modal>
      </>
    );
  }

  return (
    <div className="kanban-board-wrapper">
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions -- role via inner elements */}
      <div
        ref={boardRef}
        className="kanban-board"
        role="grid"
        aria-label={t('tasklist.kanban.boardLabel', 'Quadro Kanban de {{name}}', {
          name: taskList.title,
        })}
        aria-describedby="kanban-instructions"
        tabIndex={0}
        onKeyDown={(e) => {
          handleBoardTabKeyDown(e);
          handleBoardKeyDown(e);
        }}
        onFocus={() => {
          // Ao receber foco via Tab, anunciar contexto
          const columnTasks = getColumnTasks(focusPos.col);
          const task = columnTasks[focusPos.row];
          if (task) {
            announceCard(task, focusPos.col, focusPos.row);
          } else {
            const status = statuses[focusPos.col];
            announce(
              `${status?.label ?? ''}, ${t('tasklist.kanban.emptyColumn', 'coluna vazia')}`,
              'assertive',
            );
          }
        }}
      >
        <div id="kanban-instructions" className="sr-only">
          {t(
            'tasklist.kanban.instructions',
            'Use setas esquerda e direita para trocar de coluna. Setas para cima e baixo trocam de card. Alt+Setas reordena ou move entre colunas. Espaço seleciona e solta um card. Delete apaga. F2 renomeia. Enter abre o menu.',
          )}
        </div>

        {/* Row header (status labels) */}
        <div className="kanban-columns" role="row">
          {statuses.map((status, colIdx) => {
            const columnTasks = getColumnTasks(colIdx);
            return (
              <div
                key={status.id}
                className={`kanban-column ${dragOverCol === colIdx ? 'kanban-column--drag-over' : ''}`}
                role="gridcell"
                aria-label={`${status.label}, ${columnTasks.length} ${
                  columnTasks.length === 1
                    ? t('tasklist.kanban.task', 'tarefa')
                    : t('tasklist.kanban.tasks', 'tarefas')
                }`}
                onDragOver={handleDragOver}
                onDragEnter={(e) => handleDragEnterColumn(e, colIdx)}
                onDragLeave={handleDragLeaveColumn}
                onDrop={(e) => handleDropOnColumn(e, colIdx)}
              >
                <div className="kanban-column__header">
                  <span className="kanban-column__icon" aria-hidden="true">
                    {status.icon}
                  </span>
                  <span className="kanban-column__label">{status.label}</span>
                  <span className="kanban-column__count" aria-hidden="true">
                    {columnTasks.length}
                  </span>
                </div>

                <div className="kanban-column__cards">
                  {columnTasks.length === 0 ? (
                    <div className="kanban-column__empty" aria-hidden="true">
                      {t('tasklist.kanban.emptyColumn', 'coluna vazia')}
                    </div>
                  ) : (
                    columnTasks.map((task, rowIdx) => {
                      const isFocused = focusPos.col === colIdx && focusPos.row === rowIdx;
                      const isGrabbed = grabbedTask?.id === task.id;
                      const key = `${colIdx}-${rowIdx}`;

                      return (
                        <div
                          key={task.id}
                          ref={(el) => {
                            if (el) cardRefs.current.set(key, el);
                            else cardRefs.current.delete(key);
                          }}
                          className={`kanban-card ${isFocused ? 'kanban-card--focused' : ''} ${
                            isGrabbed ? 'kanban-card--grabbed' : ''
                          }`}
                          role="gridcell"
                          tabIndex={isFocused ? 0 : -1}
                          aria-label={task.title}
                          aria-grabbed={isGrabbed}
                          draggable
                          onDragStart={(e) => handleDragStart(e, task)}
                          onClick={() => {
                            setFocusPos({ col: colIdx, row: rowIdx });
                            announceCard(task, colIdx, rowIdx);
                          }}
                          onContextMenu={(e) => {
                            e.preventDefault();
                            setFocusPos({ col: colIdx, row: rowIdx });
                            openCardContextMenu(task, colIdx, e.currentTarget);
                          }}
                          onKeyDown={(e) => handleCardKeyDown(e, task, colIdx)}
                        >
                          {renamingTaskId === task.id ? (
                            <input
                              ref={renameInputRef}
                              className="kanban-card__rename-input"
                              value={renameValue}
                              onChange={(e) => setRenameValue(e.target.value)}
                              onBlur={commitRename}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter') {
                                  e.preventDefault();
                                  commitRename();
                                } else if (e.key === 'Escape') {
                                  e.preventDefault();
                                  cancelRename();
                                }
                                e.stopPropagation();
                              }}
                              aria-label={t('tasklist.kanban.renameCard', 'Renomear tarefa')}
                            />
                          ) : (
                            <>
                              <span className="kanban-card__title">{task.title}</span>
                              {task.dueDate && (
                                <span
                                  className={`kanban-card__due ${
                                    new Date(task.dueDate) < new Date() ? 'kanban-card__due--overdue' : ''
                                  }`}
                                >
                                  {new Date(task.dueDate).toLocaleDateString('pt-BR', {
                                    month: '2-digit',
                                    day: '2-digit',
                                  })}
                                </span>
                              )}
                            </>
                          )}
                        </div>
                      );
                    })
                  )}
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Context Menu */}
      <ContextMenu {...contextMenu} onClose={closeMenu} onSelect={onSelectItem} />

      {/* Modal de Criar Tarefa */}
      <Modal
        isOpen={isCreateModalOpen}
        onClose={handleCloseModals}
        title={t('tasklist.createTask', 'Criar Tarefa')}
      >
        <TaskForm taskListId={taskListId} onSuccess={handleTaskCreated} onCancel={handleCloseModals} />
      </Modal>

      {/* Modal de Editar Tarefa */}
      <Modal
        isOpen={isEditModalOpen}
        onClose={handleCloseModals}
        title={t('tasklist.editTask', 'Editar Tarefa')}
      >
        {editingTask && (
          <TaskForm
            taskListId={taskListId}
            task={editingTask}
            onSuccess={handleTaskUpdated}
            onCancel={handleCloseModals}
          />
        )}
      </Modal>
    </div>
  );
});

KanbanBoard.displayName = 'KanbanBoard';

export default KanbanBoard;
