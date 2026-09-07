import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { useEffect, useState, type ReactNode } from 'react';
import { act, render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import KanbanBoard from './KanbanBoard';

/* ── Mock fns ──────────────────────────────────────────────── */

const mockUpdateTaskStatus = vi.fn();
const mockReorderTasks = vi.fn();
const mockDeleteTask = vi.fn();
const mockUpdateTask = vi.fn();
const mockAnnounce = vi.fn();
const mockListCardCustomActions = vi.fn();
const mockRunCustomAction = vi.fn();
const mockOpenForTrigger = vi.fn();

/* ── Mocks de módulos ──────────────────────────────────────── */

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({
      t: (_key: string, fallback?: string) => fallback ?? _key,
    }),
  };
});

vi.mock('../../store/taskListStore', () => ({
  useTaskListStore: Object.assign(
    () => ({
      updateTaskStatus: mockUpdateTaskStatus,
      reorderTasks: mockReorderTasks,
      deleteTask: mockDeleteTask,
      updateTask: mockUpdateTask,
      listCardCustomActions: mockListCardCustomActions,
    }),
    { getState: () => ({ updateTask: mockUpdateTask }) },
  ),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce }),
}));

vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, x: 0, y: 0, items: [], ariaLabel: '' },
    triggerElementRef: { current: null },
    openForTrigger: mockOpenForTrigger,
    openAtPoint: vi.fn(),
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

vi.mock('./useCustomActions', () => ({
  useCustomActions: () => ({
    runCustomAction: mockRunCustomAction,
  }),
}));

vi.mock('../menu', () => ({
  ContextMenu: () => null,
}));

vi.mock('../ui/Modal', () => ({
  Modal: ({ isOpen, children, title }: { isOpen: boolean; children: ReactNode; title?: string }) =>
    isOpen ? (
      <div data-testid="modal">
        {title && <h2>{title}</h2>}
        {children}
      </div>
    ) : null,
}));

vi.mock('./TaskForm', () => ({
  default: ({ onSuccess, onCancel }: { onSuccess?: (t: unknown) => void; onCancel?: () => void }) => (
    <div data-testid="task-form">
      <button onClick={() => onSuccess?.({ id: "99", title: 'Nova', taskListId: "1", statusId: 1, order: 0, description: '', createdAt: '', updatedAt: '' })}>
        salvar
      </button>
      <button onClick={onCancel}>cancelar</button>
    </div>
  ),
}));

/* ── Dados de teste ────────────────────────────────────────── */

const statuses = [
  { id: 1, order: 0, label: 'A Fazer', color: 'gray', icon: '⌛' },
  { id: 2, order: 1, label: 'Em Progresso', color: 'blue', icon: '▶️' },
  { id: 3, order: 2, label: 'Concluído', color: 'green', icon: '✅' },
];

const workflow = {
  id: "1",
  taskListId: "1",
  statuses,
  allowedTransitions: { 1: [2, 3], 2: [1, 3], 3: [1, 2] },
  initialStatusId: 1,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
};

const makeTasks = () => [
  { id: "10", taskListId: "1", title: 'Tarefa Alpha', description: '', statusId: 1, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
  { id: "11", taskListId: "1", title: 'Tarefa Beta', description: '', statusId: 1, order: 1, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
  { id: "12", taskListId: "1", title: 'Tarefa Gamma', description: '', statusId: 2, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
];

const makeTaskList = (tasks = makeTasks()) => ({
  id: "1",
  title: 'Minha Lista',
  description: '',
  preferredViewMode: 'kanban' as const,
  createdAt: '2024-01-01',
  updatedAt: '2024-01-01',
  workflow,
  tasks,
});

type TestTask = ReturnType<typeof makeTasks>[number];

/**
 * Board "controlado": liga o mock de `updateTaskStatus` a uma atualização
 * otimista real do prop `tasks`, como o store faz em produção. Sem isso, mover
 * um card não recompõe as colunas e a lógica de foco pós-move (issue #177) não
 * seria exercida nos testes.
 */
type TasksUpdater = (updater: (prev: TestTask[]) => TestTask[]) => void;

function ControlledBoard({
  initialTasks,
  removeOnStatusChange = false,
  workflowOverride,
  controlRef,
}: {
  initialTasks: TestTask[];
  // Simula uma atualização concorrente que REMOVE o card ao mudar de status
  // (ex.: consolidação), para exercitar o fallback de foco quando o followTask
  // não acha mais o card movido.
  removeOnStatusChange?: boolean;
  // Permite usar um workflow diferente do padrão (ex.: colunas que compartilham
  // o mesmo status id).
  workflowOverride?: typeof workflow;
  // Expõe o setter de `tasks` para o teste simular atualizações concorrentes.
  controlRef?: { current: TasksUpdater | null };
}) {
  const [tasks, setTasks] = useState<TestTask[]>(initialTasks);

  // Configura os mocks uma única vez (no mount) e os limpa no unmount, evitando
  // side-effect durante o render. O updater funcional do setState garante que
  // a atualização sempre parte do estado mais recente (sem capturar stale).
  useEffect(() => {
    mockUpdateTaskStatus.mockImplementation(async (taskId: string, statusId: number) => {
      setTasks((prev) =>
        removeOnStatusChange
          ? prev.filter((t) => t.id !== taskId)
          : prev.map((t) => (t.id === taskId ? { ...t, statusId } : t)),
      );
    });
    mockDeleteTask.mockImplementation(async (taskId: string) => {
      setTasks((prev) => prev.filter((t) => t.id !== taskId));
    });
    if (controlRef) controlRef.current = (updater) => setTasks(updater);
    return () => {
      mockUpdateTaskStatus.mockReset();
      mockDeleteTask.mockReset();
      if (controlRef) controlRef.current = null;
    };
  }, [removeOnStatusChange, controlRef]);

  const taskList = workflowOverride
    ? { ...makeTaskList(tasks), workflow: workflowOverride }
    : makeTaskList(tasks);

  return (
    <MemoryRouter>
      <KanbanBoard taskListId={"1"} tasks={tasks} taskList={taskList} />
    </MemoryRouter>
  );
}

/* ── Suíte de testes ───────────────────────────────────────── */

describe('KanbanBoard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUpdateTaskStatus.mockResolvedValue(undefined);
    mockReorderTasks.mockResolvedValue(undefined);
    mockDeleteTask.mockResolvedValue(undefined);
    mockUpdateTask.mockResolvedValue(undefined);
    mockListCardCustomActions.mockResolvedValue([]);
    mockRunCustomAction.mockResolvedValue(undefined);
    // Congela apenas o Date (sem afetar timers/Promises) para que os
    // asserts sobre formatRelativeTime sejam determinísticos: com createdAt
    // fixo em 2024-01-01 e "agora" travado em 2026-06-01, o helper produz
    // "há 2 anos" independentemente do ano em que o teste rodar.
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-06-01T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  async function renderBoard(tasks = makeTasks()) {
    const taskList = makeTaskList(tasks);
    return render(
      <MemoryRouter>
        <KanbanBoard taskListId={"1"} tasks={tasks} taskList={taskList} />
      </MemoryRouter>,
    );
  }

  // ── Renderização ──────────────────────────────────────────

  it('renderiza colunas por status do workflow', async () => {
    await renderBoard();

    expect(screen.getByText('A Fazer')).toBeInTheDocument();
    expect(screen.getByText('Em Progresso')).toBeInTheDocument();
    expect(screen.getByText('Concluído')).toBeInTheDocument();
  });

  it('distribui cards nas colunas corretas', async () => {
    await renderBoard();

    // A Fazer tem 2 tarefas
    expect(screen.getByText('Tarefa Alpha')).toBeInTheDocument();
    expect(screen.getByText('Tarefa Beta')).toBeInTheDocument();
    // Em Progresso tem 1
    expect(screen.getByText('Tarefa Gamma')).toBeInTheDocument();
  });

  it('mostra contagem de cards em cada coluna', async () => {
    await renderBoard();

    // Contagens (aria-hidden, mas no DOM)
    const counts = screen.getAllByText(/^[0-3]$/);
    // Deve haver "2" para A Fazer, "1" para Em Progresso, "0" para Concluído
    const countTexts = counts.map((el) => el.textContent);
    expect(countTexts).toContain('2');
    expect(countTexts).toContain('1');
    expect(countTexts).toContain('0');
  });

  // ── Acessibilidade ────────────────────────────────────────

  it('tem role=grid e aria-label no board', async () => {
    await renderBoard();

    const board = screen.getByRole('grid');
    expect(board).toHaveAttribute('aria-label');
    // O mock de t() retorna o fallback com {{name}} literal
    expect(board.getAttribute('aria-label')).toContain('Kanban');
  });

  it('contém instruções de teclado em sr-only', async () => {
    await renderBoard();

    const instructions = document.getElementById('kanban-instructions');
    expect(instructions).toBeInTheDocument();
    expect(instructions?.textContent).toContain('setas');
  });

  it('anuncia card ao receber foco no board', async () => {
    await renderBoard();

    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('Tarefa Alpha'),
      'assertive',
    );
  });

  it('card tem aria-describedby com status e posição', async () => {
    await renderBoard();

    const desc = document.getElementById('card-desc-10');
    expect(desc).toBeInTheDocument();
    expect(desc?.textContent).toContain('A Fazer');
    expect(desc?.textContent).toContain('1 de 2');
  });

  // ── Data de criação (issue #151) ──────────────────────────

  it('card aria-describedby inclui a data de criação ao final, no formato do chat', async () => {
    await renderBoard();

    const desc = document.getElementById('card-desc-10');
    expect(desc).toBeInTheDocument();
    // Prefixo i18n para a data de criação
    expect(desc?.textContent).toContain('criado');
    // Sufixo no mesmo formato relativo usado nas mensagens do chat.
    // Com o relógio travado em 2026-06-01 e createdAt em 2024-01-01,
    // formatRelativeTime retorna "há 2 anos".
    expect(desc?.textContent).toMatch(/há \d+ anos?/);
    // E a data deve vir DEPOIS da posição (último item lido).
    const text = desc?.textContent ?? '';
    expect(text.indexOf('criado')).toBeGreaterThan(text.indexOf('1 de 2'));
  });

  it('anúncio do card inclui a data de criação ao receber foco', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringMatching(/criado há \d+ anos?/),
      'assertive',
    );
  });

  // ── Navegação por teclado ─────────────────────────────────

  it('navega entre cards com ArrowDown', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board);
    mockAnnounce.mockClear();

    fireEvent.keyDown(board, { key: 'ArrowDown' });

    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('Tarefa Beta'),
      'assertive',
    );
  });

  it('navega entre colunas com ArrowRight', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board);
    mockAnnounce.mockClear();

    fireEvent.keyDown(board, { key: 'ArrowRight' });

    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('Tarefa Gamma'),
      'assertive',
    );
  });

  // ── Home/End/PageUp/PageDown/Ctrl+Home/End (issue #179) ───

  it('End vai ao último card da coluna e Home volta ao primeiro', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board); // foca col 0, row 0 (Tarefa Alpha)
    mockAnnounce.mockClear();

    fireEvent.keyDown(board, { key: 'End' });
    expect(mockAnnounce).toHaveBeenLastCalledWith(
      expect.stringContaining('Tarefa Beta'),
      'assertive',
    );

    mockAnnounce.mockClear();
    fireEvent.keyDown(board, { key: 'Home' });
    expect(mockAnnounce).toHaveBeenLastCalledWith(
      expect.stringContaining('Tarefa Alpha'),
      'assertive',
    );
  });

  it('PageDown salta 10 cards dentro da coluna e PageUp retorna', async () => {
    const manyTasks = Array.from({ length: 12 }, (_, i) => ({
      id: String(100 + i),
      taskListId: '1',
      title: `Card ${i + 1}`,
      description: '',
      statusId: 1,
      order: i,
      createdAt: '2024-01-01',
      updatedAt: '2024-01-01',
    }));
    await renderBoard(manyTasks);
    const board = screen.getByRole('grid');
    fireEvent.focus(board); // col 0, row 0 (Card 1)
    mockAnnounce.mockClear();

    fireEvent.keyDown(board, { key: 'PageDown' });
    expect(mockAnnounce).toHaveBeenLastCalledWith(
      expect.stringContaining('Card 11'),
      'assertive',
    );

    mockAnnounce.mockClear();
    fireEvent.keyDown(board, { key: 'PageUp' });
    expect(mockAnnounce).toHaveBeenLastCalledWith(
      expect.stringContaining('Card 1'),
      'assertive',
    );
  });

  it('não intercepta Ctrl+PageDown/Ctrl+PageUp (atalho global de abas)', async () => {
    const manyTasks = Array.from({ length: 12 }, (_, i) => ({
      id: String(100 + i),
      taskListId: '1',
      title: `Card ${i + 1}`,
      description: '',
      statusId: 1,
      order: i,
      createdAt: '2024-01-01',
      updatedAt: '2024-01-01',
    }));
    await renderBoard(manyTasks);
    const board = screen.getByRole('grid');
    fireEvent.focus(board); // col 0, row 0 (Card 1)
    mockAnnounce.mockClear();

    // fireEvent.keyDown retorna false quando preventDefault foi chamado.
    // O board NÃO deve cancelar o evento nem mover o foco.
    const notCanceledDown = fireEvent.keyDown(board, { key: 'PageDown', ctrlKey: true });
    const notCanceledUp = fireEvent.keyDown(board, { key: 'PageUp', ctrlKey: true });

    expect(notCanceledDown).toBe(true);
    expect(notCanceledUp).toBe(true);
    expect(mockAnnounce).not.toHaveBeenCalled();
  });

  it('Ctrl+End vai ao último card do board e Ctrl+Home ao primeiro', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board); // col 0, row 0 (Tarefa Alpha)
    mockAnnounce.mockClear();

    // Última coluna com cards é "Em Progresso" (Tarefa Gamma)
    fireEvent.keyDown(board, { key: 'End', ctrlKey: true });
    expect(mockAnnounce).toHaveBeenLastCalledWith(
      expect.stringContaining('Tarefa Gamma'),
      'assertive',
    );

    mockAnnounce.mockClear();
    fireEvent.keyDown(board, { key: 'Home', ctrlKey: true });
    expect(mockAnnounce).toHaveBeenLastCalledWith(
      expect.stringContaining('Tarefa Alpha'),
      'assertive',
    );
  });

  // ── Reordenar com Alt+Arrow ───────────────────────────────

  it('reordena card com Alt+ArrowDown', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    fireEvent.keyDown(board, { key: 'ArrowDown', altKey: true });

    await waitFor(() => {
      expect(mockReorderTasks).toHaveBeenCalledWith("1", 1, ["11", "10"]);
    });
  });

  // ── Mover entre colunas com Alt+ArrowRight ────────────────

  it('move card para outra coluna com Alt+ArrowRight', async () => {
    await renderBoard();

    // Foca o primeiro card ("Tarefa Alpha", col 0)
    const card = screen.getByText('Tarefa Alpha').closest('.kanban-card');
    expect(card).toBeTruthy();

    fireEvent.keyDown(card!, { key: 'ArrowRight', altKey: true });

    await waitFor(() => {
      expect(mockUpdateTaskStatus).toHaveBeenCalledWith("10", 2);
    });
  });

  // ── Grab/Drop com Space ───────────────────────────────────

  it('graba e solta card com Space', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board);
    mockAnnounce.mockClear();

    // Grab
    fireEvent.keyDown(board, { key: ' ' });
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('selecionado'),
      'assertive',
    );

    mockAnnounce.mockClear();

    // Drop
    fireEvent.keyDown(board, { key: ' ' });
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('solto'),
      'assertive',
    );
  });

  it('cancela grab com Escape', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    fireEvent.keyDown(board, { key: ' ' }); // grab
    mockAnnounce.mockClear();

    fireEvent.keyDown(board, { key: 'Escape' });
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('cancelada'),
      'assertive',
    );
  });

  // ── Delete ────────────────────────────────────────────────

  it('deleta card com Delete', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    fireEvent.keyDown(board, { key: 'Delete' });

    await waitFor(() => {
      expect(mockDeleteTask).toHaveBeenCalledWith("10");
    });
  });

  it('inclui custom actions no menu de contexto do card e executa a seleção', async () => {
    mockListCardCustomActions.mockResolvedValue([
      {
        id: 'investigate',
        label: 'Investigar',
        icon: '🔍',
        danger: false,
      },
    ]);
    await renderBoard();

    const card = screen.getByText('Tarefa Alpha').closest('.kanban-card');
    expect(card).toBeTruthy();
    fireEvent.contextMenu(card!);

    await waitFor(() => {
      expect(mockOpenForTrigger).toHaveBeenCalled();
    });

    const [, , items] = mockOpenForTrigger.mock.calls[mockOpenForTrigger.mock.calls.length - 1];
    const customItem = items.find((item: { id: string }) => item.id === 'custom-investigate');
    expect(customItem).toEqual(expect.objectContaining({
      label: 'Investigar',
      icon: '🔍',
    }));

    customItem.action();

    expect(mockRunCustomAction).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'investigate', label: 'Investigar' }),
      '1',
      '10',
    );
  });

  // ── F2 Rename ─────────────────────────────────────────────

  it('inicia rename com F2 e salva com Enter', async () => {
    await renderBoard();
    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    fireEvent.keyDown(board, { key: 'F2' });

    const input = await screen.findByRole('textbox', { name: /Renomear/i });
    expect(input).toBeInTheDocument();

    fireEvent.change(input, { target: { value: 'Novo Nome' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    await waitFor(() => {
      expect(mockUpdateTask).toHaveBeenCalledWith("10", 'Novo Nome');
    });
  });

  // ── Drag & Drop visual ───────────────────────────────────

  it('aceita drop de tarefa em outra coluna', async () => {
    await renderBoard();

    // Simula drag start no card
    const card = screen.getByText('Tarefa Alpha').closest('.kanban-card');
    expect(card).toBeTruthy();

    // Encontra a coluna "Em Progresso" (segunda coluna)
    const columns = document.querySelectorAll('.kanban-column');
    const progressColumn = columns[1];

    const dataTransfer = {
      setData: vi.fn(),
      getData: vi.fn().mockReturnValue('10'),
      effectAllowed: '',
      dropEffect: '',
    };

    fireEvent.dragStart(card!, { dataTransfer });
    expect(dataTransfer.setData).toHaveBeenCalledWith('text/plain', '10');

    fireEvent.dragOver(progressColumn, { dataTransfer });
    fireEvent.drop(progressColumn, { dataTransfer });

    await waitFor(() => {
      expect(mockUpdateTaskStatus).toHaveBeenCalledWith("10", 2);
    });
  });

  // ── Foco após mover entre colunas (issue #177) ───────────

  it('mantém o foco no board e vai para o próximo card da coluna de origem ao mover (Alt+ArrowRight)', async () => {
    render(<ControlledBoard initialTasks={makeTasks()} />);

    // Move "Tarefa Alpha" (col 0, linha 0) para a coluna 1.
    const alphaCard = screen.getByText('Tarefa Alpha').closest('.kanban-card');
    expect(alphaCard).toBeTruthy();
    fireEvent.keyDown(alphaCard!, { key: 'ArrowRight', altKey: true });

    await waitFor(() => {
      expect(mockUpdateTaskStatus).toHaveBeenCalledWith("10", 2);
    });

    const board = screen.getByRole('grid');
    const betaCard = screen.getByText('Tarefa Beta').closest('.kanban-card');

    await waitFor(() => {
      // (1) o foco permanece DENTRO do board
      expect(board.contains(document.activeElement)).toBe(true);
      // (2) o foco foi para o PRÓXIMO card da coluna de origem ("Tarefa Beta")
      expect(document.activeElement).toBe(betaCard);
    });
  });

  it('ao esvaziar a coluna de origem, o foco acompanha o card movido', async () => {
    const tasks: TestTask[] = [
      { id: "10", taskListId: "1", title: 'Tarefa Alpha', description: '', statusId: 1, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
      { id: "12", taskListId: "1", title: 'Tarefa Gamma', description: '', statusId: 2, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
    ];
    render(<ControlledBoard initialTasks={tasks} />);

    // "Tarefa Alpha" é o único card da coluna 0; ao movê-la a coluna esvazia.
    const alphaCard = screen.getByText('Tarefa Alpha').closest('.kanban-card');
    fireEvent.keyDown(alphaCard!, { key: 'ArrowRight', altKey: true });

    await waitFor(() => {
      expect(mockUpdateTaskStatus).toHaveBeenCalledWith("10", 2);
    });

    const board = screen.getByRole('grid');
    await waitFor(() => {
      const movedAlpha = screen.getByText('Tarefa Alpha').closest('.kanban-card');
      expect(board.contains(document.activeElement)).toBe(true);
      // fallback: o foco acompanha o card movido (agora na coluna de destino)
      expect(document.activeElement).toBe(movedAlpha);
    });
  });

  it('no modo grab usa o task atual e permite mover de volta à coluna de origem', async () => {
    const tasks: TestTask[] = [
      { id: "10", taskListId: "1", title: 'Tarefa Alpha', description: '', statusId: 1, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
      { id: "12", taskListId: "1", title: 'Tarefa Gamma', description: '', statusId: 2, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
    ];
    render(<ControlledBoard initialTasks={tasks} />);

    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    // Agarra "Tarefa Alpha" e move para a direita (status 2).
    fireEvent.keyDown(board, { key: ' ' });
    fireEvent.keyDown(board, { key: 'ArrowRight' });

    await waitFor(() => {
      expect(mockUpdateTaskStatus).toHaveBeenCalledWith("10", 2);
    });
    // O foco acompanha o card carregado até a nova coluna.
    await waitFor(() => {
      const alpha = screen.getByText('Tarefa Alpha').closest('.kanban-card');
      expect(document.activeElement).toBe(alpha);
    });

    // Move de volta para a coluna original: sem usar o task atual, o
    // `grabbedTask` stale bloquearia este movimento (early-return por status).
    fireEvent.keyDown(board, { key: 'ArrowLeft' });
    await waitFor(() => {
      expect(mockUpdateTaskStatus).toHaveBeenCalledWith("10", 1);
    });
  });

  it('cancela o grab (e não move outro card) se o card carregado for removido', async () => {
    const tasks: TestTask[] = [
      { id: "10", taskListId: "1", title: 'Tarefa Alpha', description: '', statusId: 1, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
      { id: "12", taskListId: "1", title: 'Tarefa Gamma', description: '', statusId: 2, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
    ];
    render(<ControlledBoard initialTasks={tasks} />);

    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    // Agarra "Tarefa Alpha" e em seguida a deleta (some do estado).
    fireEvent.keyDown(board, { key: ' ' });
    fireEvent.keyDown(board, { key: 'Delete' });
    await waitFor(() => {
      expect(mockDeleteTask).toHaveBeenCalledWith("10");
    });

    mockAnnounce.mockClear();

    // Tenta mover com o card carregado já inexistente: deve cancelar o grab
    // e NÃO mover nenhum outro card.
    fireEvent.keyDown(board, { key: 'ArrowRight' });

    await waitFor(() => {
      expect(mockAnnounce).toHaveBeenCalledWith(
        expect.stringContaining('cancelada'),
        'assertive',
      );
    });
    expect(mockUpdateTaskStatus).not.toHaveBeenCalled();
  });

  it('no modo followTask, se o card sumir após o move, o foco vai para outra coluna não vazia (não cai no body)', async () => {
    const tasks: TestTask[] = [
      { id: "10", taskListId: "1", title: 'Tarefa Alpha', description: '', statusId: 1, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
      { id: "11", taskListId: "1", title: 'Tarefa Beta', description: '', statusId: 1, order: 1, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
    ];
    // `removeOnStatusChange` simula uma atualização concorrente que remove o
    // card movido — então `findTaskPos` não acha o card no branch followTask.
    render(<ControlledBoard initialTasks={tasks} removeOnStatusChange />);

    const board = screen.getByRole('grid');
    fireEvent.focus(board);

    // Agarra "Tarefa Alpha" e move para a direita (o card é removido no caminho).
    fireEvent.keyDown(board, { key: ' ' });
    fireEvent.keyDown(board, { key: 'ArrowRight' });

    await waitFor(() => {
      expect(mockUpdateTaskStatus).toHaveBeenCalledWith("10", 2);
    });

    // Fallback: como "Alpha" sumiu, o foco vai para o 1º card de uma coluna não
    // vazia ("Tarefa Beta"), permanecendo dentro do board (não no <body>).
    await waitFor(() => {
      const betaCard = screen.getByText('Tarefa Beta').closest('.kanban-card');
      expect(board.contains(document.activeElement)).toBe(true);
      expect(document.activeElement).toBe(betaCard);
    });
    expect(document.activeElement).not.toBe(document.body);
  });

  it('Alt+Seta para coluna de MESMO status não arma foco pendente (não reposiciona em update posterior)', async () => {
    // Workflow propositalmente com duas colunas que COMPARTILHAM o mesmo status
    // id — única forma de o alvo do Alt+Seta ter o mesmo status do card e, assim,
    // exercitar o early-return de `moveTaskToColumn` (nenhum movimento real).
    // Esse dado malformado gera um aviso esperado de "key duplicada" do React
    // (colunas são keyed por status.id); silenciamos só dentro deste teste.
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      const dupWorkflow = {
        ...workflow,
        statuses: [
          { id: 7, order: 0, label: 'Col A', color: 'gray', icon: '⌛' },
          { id: 7, order: 1, label: 'Col B', color: 'blue', icon: '▶️' },
          { id: 9, order: 2, label: 'Col C', color: 'green', icon: '✅' },
        ],
      };
      const tasks: TestTask[] = [
        { id: "11", taskListId: "1", title: 'DupA', description: '', statusId: 7, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
        { id: "10", taskListId: "1", title: 'Keeper', description: '', statusId: 9, order: 0, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
      ];
      const controlRef: { current: TasksUpdater | null } = { current: null };
      render(<ControlledBoard initialTasks={tasks} workflowOverride={dupWorkflow} controlRef={controlRef} />);

      const board = screen.getByRole('grid');
      fireEvent.focus(board);

      // Alt+ArrowRight no card da Col A: o alvo (Col B) tem o MESMO status id (7),
      // então não há movimento — e o foco pendente NÃO deve ser armado.
      const dupInColA = screen.getAllByText('DupA')[0].closest('.kanban-card');
      fireEvent.keyDown(dupInColA!, { key: 'ArrowRight', altKey: true });
      expect(mockUpdateTaskStatus).not.toHaveBeenCalled();

      // Move o foco para "Keeper" (Col C), distante do alvo de um eventual foco
      // pendente espúrio (Col A).
      fireEvent.keyDown(board, { key: 'ArrowRight' });
      fireEvent.keyDown(board, { key: 'ArrowRight' });
      const keeperCard = screen.getByText('Keeper').closest('.kanban-card');
      await waitFor(() => expect(document.activeElement).toBe(keeperCard));

      // Atualização concorrente não relacionada: se um foco pendente tivesse sido
      // armado indevidamente, ele dispararia aqui e roubaria o foco para a Col A.
      act(() => {
        controlRef.current?.((prev) => [
          ...prev,
          { id: "99", taskListId: "1", title: 'Novo', description: '', statusId: 9, order: 1, createdAt: '2024-01-01', updatedAt: '2024-01-01' },
        ]);
      });

      // O foco permanece em "Keeper" — sem reposicionamento espúrio.
      expect(document.activeElement).toBe(keeperCard);
    } finally {
      consoleError.mockRestore();
    }
  });

  it('mantém o foco no board ao mover via menu de contexto (issue #177)', async () => {
    render(<ControlledBoard initialTasks={makeTasks()} />);

    const board = screen.getByRole('grid');
    fireEvent.focus(board); // foca col 0, row 0 ("Tarefa Alpha")

    // Abre o menu de contexto do card focado e captura os itens passados ao menu.
    const alphaCard = screen.getByText('Tarefa Alpha').closest('.kanban-card');
    fireEvent.keyDown(alphaCard!, { key: 'ContextMenu' });
    await waitFor(() => expect(mockOpenForTrigger).toHaveBeenCalled());

    type MenuItemLike = {
      id: string;
      action?: () => void;
      submenu?: MenuItemLike[];
    };
    const calls = mockOpenForTrigger.mock.calls;
    const items = calls[calls.length - 1][2] as MenuItemLike[];
    const moveTo = items.find((i) => i.id === 'move-to');
    const moveToProgress = moveTo?.submenu?.find((s) => s.id === 'move-2');
    expect(moveToProgress).toBeTruthy();

    // Invoca "Mover para Em Progresso" (status 2), como faria o clique no menu.
    act(() => {
      moveToProgress!.action!();
    });

    await waitFor(() => {
      expect(mockUpdateTaskStatus).toHaveBeenCalledWith('10', 2);
    });

    // O foco permanece no board, no próximo card da coluna de origem ("Tarefa Beta").
    const betaCard = screen.getByText('Tarefa Beta').closest('.kanban-card');
    await waitFor(() => {
      expect(board.contains(document.activeElement)).toBe(true);
      expect(document.activeElement).toBe(betaCard);
    });
    expect(document.activeElement).not.toBe(document.body);
  });

  it('segue o card focado quando um job externo o move de coluna (issue #177)', async () => {
    const controlRef: { current: TasksUpdater | null } = { current: null };
    render(<ControlledBoard initialTasks={makeTasks()} controlRef={controlRef} />);

    const board = screen.getByRole('grid');
    fireEvent.focus(board); // foca "Tarefa Alpha" (id 10, col 0)
    const alphaCard = screen.getByText('Tarefa Alpha').closest('.kanban-card');
    await waitFor(() => expect(document.activeElement).toBe(alphaCard));

    // Um job externo muda o status de "Tarefa Alpha" para a coluna 2 (status 2),
    // sem nenhum gesto do usuário — o card focado é desmontado e remontado lá.
    act(() => {
      controlRef.current?.((prev) =>
        prev.map((t) => (t.id === '10' ? { ...t, statusId: 2 } : t)),
      );
    });

    // O foco SEGUE o card até a nova coluna, sem cair no body (sem precisar de Tab).
    await waitFor(() => {
      const movedAlpha = screen.getByText('Tarefa Alpha').closest('.kanban-card');
      expect(board.contains(document.activeElement)).toBe(true);
      expect(document.activeElement).toBe(movedAlpha);
    });
    expect(document.activeElement).not.toBe(document.body);
  });

  it('em update externo, se o card focado some, o foco fica no board (fallback)', async () => {
    const controlRef: { current: TasksUpdater | null } = { current: null };
    render(<ControlledBoard initialTasks={makeTasks()} controlRef={controlRef} />);

    const board = screen.getByRole('grid');
    fireEvent.focus(board); // foca "Tarefa Alpha" (id 10, col 0, row 0)
    const alphaCard = screen.getByText('Tarefa Alpha').closest('.kanban-card');
    await waitFor(() => expect(document.activeElement).toBe(alphaCard));

    // Um job externo REMOVE o card focado (ex.: consolidação/conclusão).
    act(() => {
      controlRef.current?.((prev) => prev.filter((t) => t.id !== '10'));
    });

    // Fallback: o foco vai para o card que ocupou a posição na mesma coluna
    // ("Tarefa Beta"), permanecendo no board.
    const betaCard = screen.getByText('Tarefa Beta').closest('.kanban-card');
    await waitFor(() => {
      expect(board.contains(document.activeElement)).toBe(true);
      expect(document.activeElement).toBe(betaCard);
    });
    expect(document.activeElement).not.toBe(document.body);
  });

  it('não rouba o foco em update externo se o board não tinha foco', async () => {
    const controlRef: { current: TasksUpdater | null } = { current: null };
    render(<ControlledBoard initialTasks={makeTasks()} controlRef={controlRef} />);

    // O foco está num elemento FORA do board (o board nunca teve foco).
    const outside = document.createElement('button');
    document.body.appendChild(outside);
    try {
      outside.focus();
      expect(document.activeElement).toBe(outside);

      // Um job externo move um card.
      act(() => {
        controlRef.current?.((prev) =>
          prev.map((t) => (t.id === '10' ? { ...t, statusId: 2 } : t)),
        );
      });

      // O foco NÃO deve ser puxado para o board — permanece no elemento externo.
      expect(document.activeElement).toBe(outside);
    } finally {
      // Garante a remoção do botão mesmo se um expect acima falhar, para não
      // vazar o elemento para outros testes (flakiness).
      outside.remove();
    }
  });

  // ── Coluna vazia ──────────────────────────────────────────

  it('mostra texto de coluna vazia para status sem cards', async () => {
    await renderBoard();

    // Concluído não tem tarefas
    const emptyTexts = screen.getAllByText('coluna vazia');
    expect(emptyTexts.length).toBeGreaterThanOrEqual(1);
  });
});
