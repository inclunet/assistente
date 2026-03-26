import { describe, expect, it, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';

/* ── mocks de dependências externas ────────────────────────── */

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({
      t: (_key: string, fallback?: string) => fallback ?? _key,
    }),
  };
});

const mockDeleteTask = vi.fn();
const mockPromoteTask = vi.fn();
const mockDemoteTask = vi.fn();

vi.mock('../../store/taskListStore', () => ({
  useTaskListStore: () => ({
    deleteTask: mockDeleteTask,
    promoteTask: mockPromoteTask,
    demoteTask: mockDemoteTask,
  }),
}));

vi.mock('../ui/DataGrid', () => ({
  DataGrid: ({
    items,
    getRowActions,
  }: {
    items?: Array<{ id: number; title: string; parentId?: number }>;
    getRowActions?: (item: { id: number; title: string }) => Array<{ id: string; label: string; action: () => void }>;
  }) => (
    <div data-testid="data-grid">
      {items?.map((item) => (
        <div key={item.id} data-testid={`row-${item.id}`}>
          <span>{item.title}</span>
          {getRowActions?.(item)?.map((a) => (
            <button key={a.id} onClick={a.action}>
              {a.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
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

vi.mock('../ui/Button', () => ({
  Button: ({ onClick, children, ...rest }: { onClick?: () => void; children?: ReactNode }) => (
    <button onClick={onClick} {...rest}>
      {children}
    </button>
  ),
}));

vi.mock('./TaskForm', () => ({
  default: ({ onSuccess, onCancel }: { onSuccess?: (t: unknown) => void; onCancel?: () => void }) => (
    <div data-testid="task-form">
      <button onClick={() => onSuccess?.({ id: 99, title: 'Nova tarefa', taskListId: 1, statusId: 1, order: 0, description: '', createdAt: '', updatedAt: '' })}>
        salvar
      </button>
      <button onClick={onCancel}>cancelar</button>
    </div>
  ),
}));

/* ── dados de teste ─────────────────────────────────────────── */

const baseWorkflow = {
  id: 1,
  taskListId: 1,
  statuses: [
    { id: 1, order: 0, label: 'Todo', color: 'gray', icon: '⬜' },
    { id: 2, order: 1, label: 'Done', color: 'green', icon: '✅' },
  ],
  allowedTransitions: { 1: [2], 2: [1] },
  initialStatusId: 1,
  createdAt: '2024-01-01T00:00:00Z',
  updatedAt: '2024-01-01T00:00:00Z',
};

const makeTasks = () => [
  {
    id: 10,
    taskListId: 1,
    title: 'Tarefa raiz',
    description: '',
    statusId: 1,
    order: 0,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
  },
  {
    id: 11,
    taskListId: 1,
    title: 'Subtarefa',
    description: '',
    statusId: 1,
    parentId: 10,
    order: 1,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
  },
];

/* ── suíte de testes ────────────────────────────────────────── */

describe('TasksTable', () => {
  beforeEach(() => {
    mockDeleteTask.mockReset();
    mockPromoteTask.mockReset();
    mockDemoteTask.mockReset();
    // confirm nativo
    vi.spyOn(window, 'confirm').mockReturnValue(true);
  });

  async function renderTable(
    tasks = makeTasks(),
    props: Record<string, unknown> = {},
  ) {
    const { default: TasksTable } = await import('./TasksTable');
    return render(
      <MemoryRouter>
        <TasksTable
          taskListId={1}
          tasks={tasks}
          taskList={{
            id: 1,
            title: 'Lista 1',
            description: '',
            preferredViewMode: 'list' as const,
            createdAt: '2024-01-01T00:00:00Z',
            updatedAt: '2024-01-01T00:00:00Z',
            workflow: baseWorkflow,
            tasks,
          }}
          {...props}
        />
      </MemoryRouter>,
    );
  }

  it('renderiza as tarefas no DataGrid', async () => {
    await renderTable();

    expect(screen.getByText('Tarefa raiz')).toBeInTheDocument();
    expect(screen.getByText('Subtarefa')).toBeInTheDocument();
  });

  it('mostra estado vazio quando sem tarefas', async () => {
    await renderTable([]);

    expect(screen.getByText('Nenhuma tarefa nesta lista')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Criar Tarefa/i })).toBeInTheDocument();
  });

  it('abre modal de criar tarefa', async () => {
    const user = userEvent.setup();
    await renderTable();

    const createBtn = screen.getAllByRole('button', { name: /Criar Tarefa/i })[0];
    await user.click(createBtn);

    expect(screen.getByTestId('task-form')).toBeInTheDocument();
  });

  it('abre modal de edição ao clicar em Editar na row action', async () => {
    const user = userEvent.setup();
    await renderTable();

    const editButtons = screen.getAllByRole('button', { name: 'Editar' });
    await user.click(editButtons[0]);

    expect(screen.getByTestId('task-form')).toBeInTheDocument();
  });

  it('chama deleteTask ao confirmar exclusão', async () => {
    const user = userEvent.setup();
    mockDeleteTask.mockResolvedValue(undefined);
    await renderTable();

    const deleteButtons = screen.getAllByRole('button', { name: 'Deletar' });
    await user.click(deleteButtons[0]);

    await waitFor(() => {
      expect(mockDeleteTask).toHaveBeenCalledWith(10);
    });
  });

  it('não chama deleteTask se cancelar confirm', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(false);
    const user = userEvent.setup();
    await renderTable();

    const deleteButtons = screen.getAllByRole('button', { name: 'Deletar' });
    await user.click(deleteButtons[0]);

    expect(mockDeleteTask).not.toHaveBeenCalled();
  });

  it('exibe ação Promover apenas para subtasks (com parentId)', async () => {
    await renderTable();

    // Subtarefa (parentId=10) deve ter "Promover (remover pai)"
    const row11 = screen.getByTestId('row-11');
    expect(row11.querySelector('button')).toBeTruthy();

    const promoteButtons = screen.getAllByRole('button', { name: /Promover/i });
    expect(promoteButtons.length).toBe(1); // só a subtarefa
  });

  it('chama promoteTask ao clicar em Promover', async () => {
    const user = userEvent.setup();
    mockPromoteTask.mockResolvedValue(undefined);
    await renderTable();

    const promoteBtn = screen.getByRole('button', { name: /Promover/i });
    await user.click(promoteBtn);

    await waitFor(() => {
      expect(mockPromoteTask).toHaveBeenCalledWith(11);
    });
  });

  it('abre modal de rebaixar e chama demoteTask ao selecionar pai', async () => {
    const user = userEvent.setup();
    mockDemoteTask.mockResolvedValue(undefined);
    await renderTable();

    // Clica em "Tornar subtarefa..." na tarefa raiz (id 10)
    const demoteButtons = screen.getAllByRole('button', { name: /Tornar subtarefa/i });
    await user.click(demoteButtons[0]);

    // Modal deve abrir com candidatos (todas as tasks exceto a que está sendo rebaixada)
    await waitFor(() => {
      expect(screen.getByTestId('modal')).toBeInTheDocument();
    });

    // Seleciona "Subtarefa" como pai
    const candidateBtn = screen.getByRole('button', { name: 'Subtarefa' });
    await user.click(candidateBtn);

    await waitFor(() => {
      expect(mockDemoteTask).toHaveBeenCalledWith(10, 11);
    });
  });

  it('chama callback onTaskCreated ao salvar nova tarefa', async () => {
    const user = userEvent.setup();
    const onTaskCreated = vi.fn();
    await renderTable(makeTasks(), { onTaskCreated });

    // Abre modal
    const createBtn = screen.getAllByRole('button', { name: /Criar Tarefa/i })[0];
    await user.click(createBtn);

    // Salva via mock do TaskForm
    const salvarBtn = screen.getByRole('button', { name: 'salvar' });
    await user.click(salvarBtn);

    expect(onTaskCreated).toHaveBeenCalledWith(
      expect.objectContaining({ id: 99, title: 'Nova tarefa' }),
    );
  });
});
