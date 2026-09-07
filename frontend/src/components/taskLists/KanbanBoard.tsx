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
  // Id do card que detém o foco — capturado por id (não por posição), para que
  // possamos reencontrá-lo mesmo quando uma atualização EXTERNA (ex.: um job) o
  // move de coluna e a posição antiga passa a apontar para outro card (issue #177).
  const focusedTaskIdRef = useRef<string | null>(null);
  // Indica que o board detém o foco do teclado. Mantido `true` mesmo quando o
  // foco "cai" no body por desmontagem do card focado (job/menu de contexto),
  // para que o effect de recuperação saiba que deve reposicionar o foco.
  const boardOwnsFocusRef = useRef(false);

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

  // Espelho síncrono das colunas atuais, para ler em effects/handlers SEM
  // recriá-los (e sem re-capturar o id do card focado) a cada update (issue #177).
  const columnsRef = useRef(tasksByStatus);
  columnsRef.current = tasksByStatus;

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

  // Após alterar focusPos, mover foco real e registrar (por id) qual card detém
  // o foco. Lemos as colunas via ref para NÃO recapturar o id quando apenas as
  // tarefas mudam (ex.: um job) — assim o id permanece o de ANTES da atualização,
  // permitindo seguir o card até sua nova coluna (issue #177).
  useEffect(() => {
    focusCard(focusPos.col, focusPos.row);
    const status = statuses[focusPos.col];
    const colTasks = status ? (columnsRef.current.get(status.id) ?? []) : [];
    focusedTaskIdRef.current = colTasks[focusPos.row]?.id ?? null;
  }, [focusPos, focusCard, statuses]);

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

  // Resolve o Task ATUAL do card carregado no modo grab. Se o card não existe
  // mais (ex.: foi deletado durante o grab), cancela o grab e retorna null —
  // nunca devemos mover um card diferente do que estava carregado (issue #177).
  const resolveGrabbedTask = useCallback((): Task | null => {
    if (!grabbedTask) return null;
    const live = findTaskById(grabbedTask.id);
    if (!live) {
      setGrabbedTask(null);
      announce(t('tasklist.kanban.grabCancelled', 'Movimentação cancelada'), 'assertive');
      return null;
    }
    return live;
  }, [grabbedTask, findTaskById, announce, t]);

  // Primeira coluna que ainda tem cards (último fallback de foco).
  const firstNonEmptyColumnPos = useCallback((): FocusPos | null => {
    for (let c = 0; c < statuses.length; c += 1) {
      if (getColumnTasks(c).length > 0) return { col: c, row: 0 };
    }
    return null;
  }, [statuses, getColumnTasks]);

  // issue #177: ao mover um card entre colunas, o card focado é desmontado e o
  // foco "cai" para o body, obrigando o usuário a apertar Tab. Reposicionamos o
  // foco dentro do board assim que as colunas são recompostas:
  //   1. Movimento iniciado pelo usuário (teclado/menu): há um `pendingFocusRef`
  //      armado, e seguimos a regra dele (próximo card da origem ou o card movido).
  //   2. Atualização EXTERNA (ex.: um job mudou o status de um card sem gesto do
  //      usuário): não há foco pendente. Se o board detinha o foco e o card focado
  //      foi desmontado (o foco escapou para o body), seguimos esse mesmo card até
  //      a nova coluna — ou caímos num fallback — para não exigir Tab.
  useEffect(() => {
    const pending = pendingFocusRef.current;
    if (pending) {
      pendingFocusRef.current = null;

      let next: FocusPos | null = null;
      if (pending.kind === 'followTask') {
        // Se o card movido não for encontrado (ex.: removido/consolidado por uma
        // atualização concorrente), mantém o foco no board indo para o primeiro
        // card de uma coluna não vazia, em vez de deixar o foco cair no body.
        next = findTaskPos(pending.taskId) ?? firstNonEmptyColumnPos();
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
        // `next` é sempre um objeto novo, então `setFocusPos` dispara o effect de
        // `focusPos` acima, que aplica o `.focus()` uma única vez. Não chamamos
        // `focusCard` aqui para evitar foco/anúncio duplicado.
        setFocusPos(next);
      }
      return;
    }

    // Sem foco pendente: a recomposição veio de FORA (ex.: um job). Só agimos se o
    // board detinha o foco E ele escapou do board (caiu no body) — caso contrário
    // não roubamos o foco de onde o usuário o deixou.
    if (!boardOwnsFocusRef.current) return;
    const boardEl = boardRef.current;
    const active = document.activeElement;
    if (boardEl && active && active !== document.body && boardEl.contains(active)) {
      return; // o foco ainda está dentro do board — nada a fazer.
    }

    // Segue o card que estava focado (capturado por id) até sua nova coluna.
    const focusedId = focusedTaskIdRef.current;
    let next: FocusPos | null = focusedId ? findTaskPos(focusedId) : null;
    if (!next) {
      // O card focado sumiu (deletado/consolidado): vai para o card que ocupou a
      // posição antiga; se a coluna esvaziou, para a primeira coluna não vazia.
      const colTasks = getColumnTasks(focusPos.col);
      next =
        colTasks.length > 0
          ? { col: focusPos.col, row: Math.min(focusPos.row, colTasks.length - 1) }
          : firstNonEmptyColumnPos();
    }
    if (next) {
      // `next` é sempre um objeto novo, então `setFocusPos` dispara o effect de
      // `focusPos`, que aplica o `.focus()` uma única vez (mesmo padrão do ramo
      // de foco pendente acima).
      setFocusPos(next);
    }
  }, [
    tasksByStatus,
    getColumnTasks,
    findTaskPos,
    firstNonEmptyColumnPos,
    focusPos,
  ]);

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
                const liveTask = findTaskById(task.id) ?? task;
                const targetStatus = statuses[targetIdx];
                if (!targetStatus || liveTask.statusId === targetStatus.id) return;
                // Mantém o foco no board após mover (issue #177): vai para o
                // próximo card da coluna de origem, igual ao Alt+Seta. Sem isso, o
                // card focado é desmontado e o foco cai no body (precisa de Tab).
                const pos = findTaskPos(task.id);
                if (pos) {
                  pendingFocusRef.current = {
                    kind: 'sourceNext',
                    sourceCol: pos.col,
                    sourceRow: pos.row,
                    taskId: task.id,
                  };
                }
                moveTaskToColumn(liveTask, targetIdx);
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
    [statuses, moveTaskToColumn, handleDeleteTask, openForTrigger, t, listCardCustomActions, runCustomAction, taskListId, findTaskById, findTaskPos],
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
              // Usa o Task ATUAL do card carregado (o `grabbedTask` capturado
              // fica stale após o update otimista). Se o card sumiu, o grab é
              // cancelado e nada é movido (issue #177).
              const liveTask = resolveGrabbedTask();
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
              // Usa o Task ATUAL do card carregado (o `grabbedTask` capturado
              // fica stale após o update otimista). Se o card sumiu, o grab é
              // cancelado e nada é movido (issue #177).
              const liveTask = resolveGrabbedTask();
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

        // ── Home/End: primeiro/último card (Ctrl = board inteiro) ──
        case 'Home': {
          e.preventDefault();
          if (e.ctrlKey) {
            // Primeiro card do board inteiro (primeira coluna com cards).
            const firstCol = statuses.findIndex((_, i) => getColumnTasks(i).length > 0);
            if (firstCol === -1 || (firstCol === col && row === 0)) {
              playBumpSound();
              break;
            }
            setFocusPos({ col: firstCol, row: 0 });
            const task = getColumnTasks(firstCol)[0];
            if (task) announceCard(task, firstCol, 0);
          } else {
            // Primeiro card da coluna atual.
            if (columnTasks.length === 0 || row === 0) {
              playBumpSound();
              break;
            }
            setFocusPos({ col, row: 0 });
            const task = columnTasks[0];
            if (task) announceCard(task, col, 0);
          }
          break;
        }
        case 'End': {
          e.preventDefault();
          if (e.ctrlKey) {
            // Último card do board inteiro (última coluna com cards).
            let lastCol = -1;
            for (let i = statuses.length - 1; i >= 0; i--) {
              if (getColumnTasks(i).length > 0) {
                lastCol = i;
                break;
              }
            }
            const lastRow = lastCol === -1 ? 0 : getColumnTasks(lastCol).length - 1;
            if (lastCol === -1 || (lastCol === col && row === lastRow)) {
              playBumpSound();
              break;
            }
            setFocusPos({ col: lastCol, row: lastRow });
            const task = getColumnTasks(lastCol)[lastRow];
            if (task) announceCard(task, lastCol, lastRow);
          } else {
            // Último card da coluna atual.
            const lastRow = columnTasks.length - 1;
            if (columnTasks.length === 0 || row === lastRow) {
              playBumpSound();
              break;
            }
            setFocusPos({ col, row: lastRow });
            const task = columnTasks[lastRow];
            if (task) announceCard(task, col, lastRow);
          }
          break;
        }

        // ── PageUp/PageDown: salta 10 cards dentro da coluna ──
        // Não interceptamos quando ctrlKey está pressionado para não conflitar
        // com o atalho global Ctrl+PageUp/PageDown de troca de abas
        // (useWorkspaceKeyboardShortcuts). metaKey é ignorado apenas para deixar
        // passar atalhos do browser/OS — não é um atalho do app.
        case 'PageUp': {
          if (e.ctrlKey || e.metaKey) break;
          e.preventDefault();
          if (columnTasks.length === 0 || row === 0) {
            playBumpSound();
            break;
          }
          const newRow = Math.max(row - 10, 0);
          setFocusPos({ col, row: newRow });
          const task = columnTasks[newRow];
          if (task) announceCard(task, col, newRow);
          break;
        }
        case 'PageDown': {
          if (e.ctrlKey || e.metaKey) break;
          e.preventDefault();
          if (columnTasks.length === 0 || row === columnTasks.length - 1) {
            playBumpSound();
            break;
          }
          const newRow = Math.min(row + 10, columnTasks.length - 1);
          setFocusPos({ col, row: newRow });
          const task = columnTasks[newRow];
          if (task) announceCard(task, col, newRow);
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
      focusPos, getColumnTasks, statuses, grabbedTask, resolveGrabbedTask,
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
          // Usa o status ATUAL do card (estado mais recente) e só arma o foco
          // pendente / move quando há mudança real de status — mesma condição
          // do `moveTaskToColumn`. Evita reposicionar o foco em uma atualização
          // futura quando não houve movimento (ex.: card em coluna fallback ou
          // alvo igual ao status atual). Issue #177.
          const liveTask = findTaskById(task.id) ?? task;
          const targetStatus = statuses[targetCol];
          if (targetStatus && liveTask.statusId !== targetStatus.id) {
            // Após sair da coluna, o foco vai para o próximo card da coluna de
            // origem para que o board não perca o foco e o usuário continue
            // processando a coluna sem precisar de Tab.
            const sourceTasks = getColumnTasks(colIdx);
            const sourceRow = sourceTasks.findIndex((tk) => tk.id === task.id);
            pendingFocusRef.current = {
              kind: 'sourceNext',
              sourceCol: colIdx,
              sourceRow: sourceRow < 0 ? 0 : sourceRow,
              taskId: task.id,
            };
            moveTaskToColumn(liveTask, targetCol);
          }
        }
      }
    },
    [statuses, moveTaskToColumn, getColumnTasks, findTaskById],
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
        <div className="kanban-empty">
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
              focusedTaskIdRef.current = task.id;
            } else {
              const status = statuses[focusPos.col];
              announce(
                `${status?.label ?? ''}, ${t('tasklist.kanban.emptyColumn', 'coluna vazia')}`,
                'assertive',
              );
            }
          }
          // O board passou a deter o foco do teclado (issue #177).
          boardOwnsFocusRef.current = true;
          setBoardHasInternalFocus(true);
        }}
        onBlur={(e) => {
          const next = e.relatedTarget as Node | null;
          if (!boardRef.current?.contains(next)) {
            setBoardHasInternalFocus(false);
            // Só abrimos mão da POSSE do foco quando o usuário o move para um
            // elemento real FORA do board. Se `relatedTarget` é null, o foco caiu
            // no body — provável desmontagem do card focado por um job/menu —, e
            // mantemos a posse para o effect de recuperação reposicionar (issue #177).
            if (next) boardOwnsFocusRef.current = false;
          }
        }}
      >
        <div id="kanban-instructions" className="sr-only">
          {t(
            'tasklist.kanban.instructions',
            'Use setas esquerda e direita para trocar de coluna. Setas para cima e baixo trocam de card. Início e Fim vão ao primeiro e último card da coluna; Ctrl+Início e Ctrl+Fim vão ao primeiro e último card do quadro. Page Up e Page Down saltam 10 cards na coluna. Alt+Setas reordena ou move entre colunas. Espaço seleciona e solta um card. Delete apaga. F2 renomeia. Enter abre os detalhes do card. Shift+F10 ou a tecla Menu abrem o menu de contexto.',
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
