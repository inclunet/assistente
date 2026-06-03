import {
  useState,
  useRef,
  useCallback,
  useEffect,
  useMemo,
  forwardRef,
  useImperativeHandle,
} from 'react';
import { LinkOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useTaskListStore } from '../../store/taskListStore';
import { openTaskLink } from '../../lib/deepLinks';
import { formatRelativeTime } from '../../lib/dateUtils';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { ContextMenu, type MenuItem } from '../menu';
import { Modal } from '../ui/Modal';
import { playBumpSound } from '../../services/audioFeedback';
import TaskForm from './TaskForm';
import TaskDetailModal from './TaskDetailModal';
import { useCustomActions } from './useCustomActions';
import type { Task, TaskListWithWorkflow } from '../../types/tasklist';
import './KanbanBoard.css';

/* ── Tipos ─────────────────────────────────────────────────────── */

interface KanbanBoardProps {
  taskListId: string;
  tasks: Task[];
  taskList: TaskListWithWorkflow;
  onTaskCreated?: (task: Task) => void;
  onTaskUpdated?: (task: Task) => void;
  onTaskDeleted?: (taskId: string) => void;
}

export interface KanbanBoardRef {
  openCreateModal: () => void;
}

interface FocusPos {
  col: number;
  row: number;
}

// Para onde reposicionar o foco depois que um card muda de coluna (issue #177).
// `sourceNext`: vai para o próximo card da coluna de ORIGEM (comportamento
// preferido). `followTask`: o foco acompanha o card movido (usado no modo grab
// e como fallback quando a coluna de origem fica vazia).
type PendingFocus =
  | { kind: 'sourceNext'; sourceCol: number; sourceRow: number; taskId: string }
  | { kind: 'followTask'; taskId: string };

/* ── Componente ────────────────────────────────────────────────── */

const KanbanBoard = forwardRef<KanbanBoardRef, KanbanBoardProps>(function KanbanBoard(
  { taskListId, tasks, taskList, onTaskCreated, onTaskUpdated, onTaskDeleted },
  ref,
) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { announce } = useAnnouncer();
  const { updateTaskStatus, reorderTasks, deleteTask, listCardCustomActions } = useTaskListStore();
  const { runCustomAction } = useCustomActions();

  // ── Estado ─────────────────────────────────────────────────
  const [focusPos, setFocusPos] = useState<FocusPos>({ col: 0, row: 0 });
  const [grabbedTask, setGrabbedTask] = useState<Task | null>(null);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [isDetailModalOpen, setIsDetailModalOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<Task | null>(null);
  const [detailTask, setDetailTask] = useState<Task | null>(null);
  const [boardHasInternalFocus, setBoardHasInternalFocus] = useState(false);

  // ── Refs ───────────────────────────────────────────────────
  const boardRef = useRef<HTMLDivElement>(null);
  const cardRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  // Foco a reaplicar depois que um movimento de card recompõe as colunas.
  const pendingFocusRef = useRef<PendingFocus | null>(null);

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

  // Localiza a posição (coluna/linha) atual de um card pelo id.
  const findTaskPos = useCallback(
    (taskId: string): FocusPos | null => {
      for (let c = 0; c < statuses.length; c += 1) {
        const idx = getColumnTasks(c).findIndex((tk) => tk.id === taskId);
        if (idx >= 0) return { col: c, row: idx };
      }
      return null;
    },
    [statuses, getColumnTasks],
  );

  // Retorna o objeto Task ATUAL (do estado mais recente) a partir do id.
  // Necessário porque `grabbedTask` é capturado no momento do grab e fica
  // stale após o update otimista de status (issue #177).
  const findTaskById = useCallback(
    (taskId: string): Task | undefined => {
      for (const arr of tasksByStatus.values()) {
        const found = arr.find((tk) => tk.id === taskId);
        if (found) return found;
      }
      return undefined;
    },
    [tasksByStatus],
  );

  // Primeira coluna que ainda tem cards (último fallback de foco).
  const firstNonEmptyColumnPos = useCallback((): FocusPos | null => {
    for (let c = 0; c < statuses.length; c += 1) {
      if (getColumnTasks(c).length > 0) return { col: c, row: 0 };
    }
    return null;
  }, [statuses, getColumnTasks]);

  // issue #177: ao mover um card entre colunas, o card focado é desmontado e o
  // foco "cai" para o body, obrigando o usuário a apertar Tab. Quando há um
  // foco pendente, reposicionamos o foco dentro do board assim que as colunas
  // são recompostas (depois da atualização otimista de `tasks`).
  useEffect(() => {
    const pending = pendingFocusRef.current;
    if (!pending) return;
    pendingFocusRef.current = null;

    let next: FocusPos | null = null;
    if (pending.kind === 'followTask') {
      next = findTaskPos(pending.taskId);
    } else {
      const sourceTasks = getColumnTasks(pending.sourceCol);
      if (sourceTasks.length > 0) {
        // Próximo card da coluna de origem (o que ocupou a posição liberada);
        // se o card movido era o último, cai no novo último da coluna.
        next = {
          col: pending.sourceCol,
          row: Math.min(pending.sourceRow, sourceTasks.length - 1),
        };
      } else {
        // Coluna de origem ficou vazia: o foco acompanha o card movido.
        next = findTaskPos(pending.taskId) ?? firstNonEmptyColumnPos();
      }
    }

    if (next) {
      setFocusPos(next);
      // Reaplica o foco imediatamente, pois `focusPos` pode não mudar quando
      // o card seguinte assume a mesma posição (col/row) do card que saiu.
      focusCard(next.col, next.row);
    }
  }, [tasksByStatus, getColumnTasks, findTaskPos, firstNonEmptyColumnPos, focusCard]);

  // Formata a data de criação no MESMO formato relativo usado nas mensagens
  // do chat (ver `getAriaLabel` em ChatMessage.tsx e `buildChatMessageAriaLabel`).
  // O leitor de tela passa a anunciar a "idade" do card (issue #151).
  const formatCardCreatedAt = useCallback(
    (task: Task): string | null => {
      if (!task.createdAt) return null;
      const ts = new Date(task.createdAt).getTime();
      if (Number.isNaN(ts)) return null;
      return `${t('tasklist.kanban.cardCreatedAt', 'criado')} ${formatRelativeTime(ts)}`;
    },
    [t],
  );

  const announceCard = useCallback(
    (task: Task, colIdx: number, rowIdx: number) => {
      const status = statuses[colIdx];
      const columnTasks = getColumnTasks(colIdx);
      const parts: string[] = [task.title];
      if (task.assigneeName) parts.push(`${t('tasklist.assignee', 'Responsável')}: ${task.assigneeName}`);
      if (task.creatorName) parts.push(`${t('tasklist.creator', 'Criador')}: ${task.creatorName}`);
      parts.push(status?.label ?? '');
      parts.push(`${rowIdx + 1} ${t('tasklist.kanban.of', 'de')} ${columnTasks.length}`);
      const createdLabel = formatCardCreatedAt(task);
      if (createdLabel) parts.push(createdLabel);
      announce(parts.join('. '), 'assertive');
    },
    [statuses, getColumnTasks, announce, t, formatCardCreatedAt],
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
    setIsDetailModalOpen(false);
    setEditingTask(null);
    setDetailTask(null);
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
    async (task: Task, _colIdx: number, trigger: HTMLElement) => {
      const items: MenuItem[] = [
        {
          id: 'details',
          label: t('tasklist.details', 'Detalhes'),
          action: () => {
            setDetailTask(task);
            setIsDetailModalOpen(true);
          },
        },
        {
          id: 'edit',
          label: t('tasklist.edit', 'Editar'),
          action: () => {
            setEditingTask(task);
            setIsEditModalOpen(true);
          },
        },
        { separator: true, id: 'sep-1' },
        {
          id: 'move-to',
          label: t('tasklist.kanban.moveToSubmenu', 'Mover para…'),
          submenu: statuses
            .filter((s) => s.id !== task.statusId)
            .map((s) => ({
              id: `move-${s.id}`,
              label: s.label,
              action: () => {
                const targetIdx = statuses.findIndex((st) => st.id === s.id);
                moveTaskToColumn(task, targetIdx);
              },
            })),
        },
        { separator: true, id: 'sep-2' },
        {
          id: 'delete',
          label: t('tasklist.delete', 'Deletar'),
          danger: true,
          action: () => handleDeleteTask(task),
        },
      ];

      // Custom actions (AEP-0067): when avaliado server-side; só aparecem as visíveis.
      try {
        const customs = await listCardCustomActions(task.id, 'card_menu');
        if (customs.length > 0) {
          items.push({ separator: true, id: 'sep-custom' });
          for (const ca of customs) {
            items.push({
              id: `custom-${ca.id}`,
              label: ca.label,
              icon: ca.icon || undefined,
              danger: ca.danger,
              action: () => { void runCustomAction(ca, taskListId, task.id); },
            });
          }
        }
      } catch {
        // Best-effort: ausência de custom actions não deve quebrar o menu.
      }

      // Durante o await acima o card pode ter sido removido/desmontado: abrir o
      // menu com um trigger desconectado ancoraria errado. Aborta se for o caso.
      if (!trigger.isConnected) return;

      openForTrigger(trigger, t('tasklist.kanban.cardMenu', 'Menu do card'), items);
    },
    [statuses, moveTaskToColumn, handleDeleteTask, openForTrigger, t, listCardCustomActions, runCustomAction, taskListId],
  );

  // ── Inline rename (F2) ────────────────────────────────────
  const [renamingTaskId, setRenamingTaskId] = useState<string | null>(null);
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
            if (grabbedTask) {
              // Usa o Task ATUAL do card carregado (não o `grabbedTask`
              // capturado, que fica stale após o update otimista) e só
              // reposiciona o foco quando há movimento real (issue #177).
              const liveTask = findTaskById(grabbedTask.id) ?? currentTask ?? grabbedTask;
              const targetStatus = statuses[newCol];
              if (liveTask && targetStatus && liveTask.statusId !== targetStatus.id) {
                pendingFocusRef.current = { kind: 'followTask', taskId: liveTask.id };
                moveTaskToColumn(liveTask, newCol);
              }
            } else {
              const targetTasks = getColumnTasks(newCol);
              const newRow = Math.min(row, Math.max(0, targetTasks.length - 1));
              setFocusPos({ col: newCol, row: newRow });
              const task = targetTasks[newRow];
              if (task) announceCard(task, newCol, newRow);
              else announce(statuses[newCol]?.label ?? '', 'assertive');
            }
          } else {
            playBumpSound();
          }
          break;
        }
        case 'ArrowRight': {
          e.preventDefault();
          if (col < statuses.length - 1) {
            const newCol = col + 1;
            if (grabbedTask) {
              // Usa o Task ATUAL do card carregado (não o `grabbedTask`
              // capturado, que fica stale após o update otimista) e só
              // reposiciona o foco quando há movimento real (issue #177).
              const liveTask = findTaskById(grabbedTask.id) ?? currentTask ?? grabbedTask;
              const targetStatus = statuses[newCol];
              if (liveTask && targetStatus && liveTask.statusId !== targetStatus.id) {
                pendingFocusRef.current = { kind: 'followTask', taskId: liveTask.id };
                moveTaskToColumn(liveTask, newCol);
              }
            } else {
              const targetTasks = getColumnTasks(newCol);
              const newRow = Math.min(row, Math.max(0, targetTasks.length - 1));
              setFocusPos({ col: newCol, row: newRow });
              const task = targetTasks[newRow];
              if (task) announceCard(task, newCol, newRow);
              else announce(statuses[newCol]?.label ?? '', 'assertive');
            }
          } else {
            playBumpSound();
          }
          break;
        }

        // ── Navegação entre cards ──
        case 'ArrowUp': {
          e.preventDefault();
          if (e.altKey && currentTask) {
            reorderInColumn(currentTask, col, -1);
          } else if (row > 0) {
            const newRow = row - 1;
            setFocusPos({ col, row: newRow });
            const task = columnTasks[newRow];
            if (task) announceCard(task, col, newRow);
          } else {
            playBumpSound();
          }
          break;
        }
        case 'ArrowDown': {
          e.preventDefault();
          if (e.altKey && currentTask) {
            reorderInColumn(currentTask, col, 1);
          } else if (row < columnTasks.length - 1) {
            const newRow = row + 1;
            setFocusPos({ col, row: newRow });
            const task = columnTasks[newRow];
            if (task) announceCard(task, col, newRow);
          } else {
            playBumpSound();
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

        // ── Enter: open detail modal ──
        case 'Enter': {
          e.preventDefault();
          if (currentTask) {
            setDetailTask(currentTask);
            setIsDetailModalOpen(true);
          }
          break;
        }
        // ── Shift+F10 / ContextMenu: context menu ──
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
      focusPos, getColumnTasks, statuses, grabbedTask, findTaskById,
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
          // issue #177: após sair da coluna, o foco vai para o próximo card da
          // coluna de origem (em vez de acompanhar o card até o destino), para
          // que o board não perca o foco e o usuário continue processando a
          // coluna sem precisar de Tab.
          const sourceTasks = getColumnTasks(colIdx);
          const sourceRow = sourceTasks.findIndex((tk) => tk.id === task.id);
          pendingFocusRef.current = {
            kind: 'sourceNext',
            sourceCol: colIdx,
            sourceRow: sourceRow < 0 ? 0 : sourceRow,
            taskId: task.id,
          };
          moveTaskToColumn(task, targetCol);
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
      const taskId = e.dataTransfer.getData('text/plain');
      if (!taskId) return;

      const task = tasks.find((t) => String(t.id) === taskId);
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
      <div
        ref={boardRef}
        className="kanban-board"
        role="grid"
        aria-label={t('tasklist.kanban.boardLabel', 'Quadro Kanban de {{name}}', {
          name: taskList.title,
        })}
        aria-describedby="kanban-instructions"
        tabIndex={boardHasInternalFocus ? -1 : 0}
        onKeyDown={(e) => {
          handleBoardTabKeyDown(e);
          handleBoardKeyDown(e);
        }}
        onFocus={(e) => {
          if (e.target === boardRef.current) {
            const columnTasks = getColumnTasks(focusPos.col);
            const task = columnTasks[focusPos.row];
            if (task) {
              focusCard(focusPos.col, focusPos.row);
              announceCard(task, focusPos.col, focusPos.row);
            } else {
              const status = statuses[focusPos.col];
              announce(
                `${status?.label ?? ''}, ${t('tasklist.kanban.emptyColumn', 'coluna vazia')}`,
                'assertive',
              );
            }
          }
          setBoardHasInternalFocus(true);
        }}
        onBlur={(e) => {
          if (!boardRef.current?.contains(e.relatedTarget as Node)) {
            setBoardHasInternalFocus(false);
          }
        }}
      >
        <div id="kanban-instructions" className="sr-only">
          {t(
            'tasklist.kanban.instructions',
            'Use setas esquerda e direita para trocar de coluna. Setas para cima e baixo trocam de card. Alt+Setas reordena ou move entre colunas. Espaço seleciona e solta um card. Delete apaga. F2 renomeia. Enter abre detalhes. Shift+F10 abre o menu.',
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
                          aria-describedby={`card-desc-${task.id}`}
                          aria-grabbed={isGrabbed}
                          draggable
                          onDragStart={(e) => handleDragStart(e, task)}
                          onClick={() => {
                            setFocusPos({ col: colIdx, row: rowIdx });
                            announceCard(task, colIdx, rowIdx);
                          }}
                          onDoubleClick={() => {
                            setDetailTask(task);
                            setIsDetailModalOpen(true);
                          }}
                          onContextMenu={(e) => {
                            e.preventDefault();
                            setFocusPos({ col: colIdx, row: rowIdx });
                            openCardContextMenu(task, colIdx, e.currentTarget);
                          }}
                          onKeyDown={(e) => handleCardKeyDown(e, task, colIdx)}
                        >
                          <span id={`card-desc-${task.id}`} className="sr-only">
                            {[
                              task.assigneeName && `${t('tasklist.assignee', 'Responsável')}: ${task.assigneeName}`,
                              task.creatorName && `${t('tasklist.creator', 'Criador')}: ${task.creatorName}`,
                              status?.label ?? '',
                              `${rowIdx + 1} ${t('tasklist.kanban.of', 'de')} ${columnTasks.length}`,
                              formatCardCreatedAt(task),
                            ].filter(Boolean).join('. ')}
                          </span>
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
                              {task.code && (
                                <span
                                  className={`kanban-card__code${task.link ? ' kanban-card__code--link' : ''}`}
                                  {...(task.link ? {
                                    role: 'button',
                                    tabIndex: -1,
                                    onClick: (e: React.MouseEvent) => { e.stopPropagation(); openTaskLink(task.link!, { navigate }); },
                                  } : {})}
                                >
                                  {task.code}
                                  {task.link && <span className="kanban-card__link-icon" aria-hidden="true"><LinkOutlined /></span>}
                                </span>
                              )}
                              {!task.code && task.link && (
                                <span
                                  className="kanban-card__link-only"
                                  role="button"
                                  aria-label={`Abrir link: ${task.link}`}
                                  tabIndex={-1}
                                  onClick={(e) => { e.stopPropagation(); openTaskLink(task.link!, { navigate }); }}
                                ><LinkOutlined aria-hidden="true" /></span>
                              )}
                              <span className="kanban-card__title">{task.title}</span>
                              <div className="kanban-card__meta">
                                {task.assigneeName && (
                                  <span className="kanban-card__assignee" title={task.assigneeId || undefined}>
                                    👤 {task.assigneeName}
                                  </span>
                                )}
                                {task.creatorName && (
                                  <span className="kanban-card__creator" title={task.creatorId || undefined}>
                                    ✏️ {task.creatorName}
                                  </span>
                                )}
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
                              </div>
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

      {/* Modal de Detalhes da Tarefa */}
      <TaskDetailModal
        isOpen={isDetailModalOpen}
        onClose={handleCloseModals}
        task={detailTask}
        statuses={statuses}
      />
    </div>
  );
});

KanbanBoard.displayName = 'KanbanBoard';

export default KanbanBoard;
