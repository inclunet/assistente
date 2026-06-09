import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import TaskDetailModal from './TaskDetailModal';
import type { Task, TaskListWorkflowStatus } from '../../types/tasklist';

/* ── Mocks ─────────────────────────────────────────────────── */

const mockLoadTaskNotes = vi.fn();
const mockListCardCustomActions = vi.fn();

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({ t: (_key: string, fallback?: string) => fallback ?? _key }),
  };
});

vi.mock('../../store/taskListStore', () => ({
  useTaskListStore: () => ({
    loadTaskNotes: mockLoadTaskNotes,
    createTaskNote: vi.fn(),
    updateTaskNote: vi.fn(),
    deleteTaskNote: vi.fn(),
    listCardCustomActions: mockListCardCustomActions,
  }),
}));

vi.mock('../../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn().mockResolvedValue(true),
}));

vi.mock('./useCustomActions', () => ({
  useCustomActions: () => ({ runCustomAction: vi.fn() }),
}));

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}));

/* ── Dados ─────────────────────────────────────────────────── */

const statuses: TaskListWorkflowStatus[] = [
  { id: 1, order: 0, label: 'A Fazer', color: 'gray', icon: '⌛' },
];

const task = {
  id: '10',
  taskListId: '1',
  title: 'Tarefa Alpha',
  description: 'Descrição da tarefa',
  statusId: 1,
  order: 0,
  createdAt: '2024-01-01',
  updatedAt: '2024-01-01',
} as unknown as Task;

/* ── Testes ────────────────────────────────────────────────── */

describe('TaskDetailModal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockLoadTaskNotes.mockResolvedValue([]);
    mockListCardCustomActions.mockResolvedValue([]);
  });

  it('usa readingMode (role="document") para permitir leitura linear no leitor de tela', async () => {
    render(
      <MemoryRouter>
        <TaskDetailModal isOpen onClose={vi.fn()} task={task} statuses={statuses} />
      </MemoryRouter>,
    );

    // O corpo do modal de leitura deve ser role="document" (modo navegação do
    // NVDA), e NÃO role="application" (que prenderia o usuário em modo foco).
    const body = await screen.findByRole('document');
    expect(body).toHaveClass('modal-body');
    expect(screen.queryByRole('application')).toBeNull();
  });
});
