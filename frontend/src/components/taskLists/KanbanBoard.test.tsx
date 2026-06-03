import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { useEffect, useState, type ReactNode } from 'react';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
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
function ControlledBoard({ initialTasks }: { initialTasks: TestTask[] }) {
  const [tasks, setTasks] = useState<TestTask[]>(initialTasks);

  // Configura o mock uma única vez (no mount) e o limpa no unmount, evitando
  // side-effect durante o render. O updater funcional do setState garante que
  // a atualização sempre parte do estado mais recente (sem capturar stale).
  useEffect(() => {
    mockUpdateTaskStatus.mockImplementation(async (taskId: string, statusId: number) => {
      setTasks((prev) => prev.map((t) => (t.id === taskId ? { ...t, statusId } : t)));
    });
    return () => {
      mockUpdateTaskStatus.mockReset();
    };
  }, []);

  return (
    <MemoryRouter>
      <KanbanBoard taskListId={"1"} tasks={tasks} taskList={makeTaskList(tasks)} />
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

  // ── Coluna vazia ──────────────────────────────────────────

  it('mostra texto de coluna vazia para status sem cards', async () => {
    await renderBoard();

    // Concluído não tem tarefas
    const emptyTexts = screen.getAllByText('coluna vazia');
    expect(emptyTexts.length).toBeGreaterThanOrEqual(1);
  });
});
