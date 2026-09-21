import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import TaskDetailModal from './TaskDetailModal';
import type { Task, TaskListWorkflowStatus } from '../../types/tasklist';

/* ── Mocks ─────────────────────────────────────────────────── */

const mockAnnounce = vi.fn();
const mockAddToast = vi.fn();
const mockOpenTaskLink = vi.fn();
vi.mock('../../lib/deepLinks', () => ({ openTaskLink: (...args: unknown[]) => mockOpenTaskLink(...args) }));

const mockLoadTaskNotes = vi.fn();
const mockListCardCustomActions = vi.fn();
const mockSetTaskConversation = vi.fn();
const mockTaskLists = vi.hoisted(() => new Map());
const mockGetConversations = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({ t: (key: string, fallback?: string, options?: { code?: string }) => (fallback ?? key).replace('{{code}}', options?.code ?? '') }),
  };
});

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  GetConversations: mockGetConversations,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce }),
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: ReturnType<typeof vi.fn> }) => unknown) => selector({
    addToast: mockAddToast,
  }),
}));

vi.mock('../../store/taskListStore', () => ({
  useTaskListStore: () => ({
    loadTaskNotes: mockLoadTaskNotes,
    createTaskNote: vi.fn(),
    updateTaskNote: vi.fn(),
    deleteTaskNote: vi.fn(),
    listCardCustomActions: mockListCardCustomActions,
    setTaskConversation: mockSetTaskConversation,
    taskLists: mockTaskLists,
  }),
}));

vi.mock('../../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn().mockResolvedValue(true),
}));

vi.mock('./useCustomActions', () => ({
  useCustomActions: () => ({ runCustomAction: vi.fn() }),
}));

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({
    content,
    tabNavigation,
  }: {
    content: string;
    tabNavigation?: string;
  }) => <div data-testid="task-markdown" data-tab-navigation={tabNavigation}>{content}</div>,
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
    mockTaskLists.clear();
    mockLoadTaskNotes.mockResolvedValue([]);
    mockListCardCustomActions.mockResolvedValue([]);
    mockSetTaskConversation.mockResolvedValue(undefined);
    mockGetConversations.mockResolvedValue([
      { id: '5', title: 'Conversa X', updatedAt: '2024-01-02' },
    ]);
  });

  it.each(['click', 'Enter', ' '])('copia o código exato via %s sem abrir o link', async (activation) => {
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue(undefined);
    render(<MemoryRouter><TaskDetailModal isOpen onClose={vi.fn()} task={{ ...task, code: 'EXT-0042', link: 'https://example.com/card/42' }} statuses={statuses} /></MemoryRouter>);
    const button = await screen.findByRole('button', { name: 'Copiar código EXT-0042' });
    button.focus();
    expect(button).toHaveFocus();
    if (activation === 'click') await user.click(button);
    else await user.keyboard(activation === 'Enter' ? '{Enter}' : ' ');
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('EXT-0042'));
    expect(mockOpenTaskLink).not.toHaveBeenCalled();
    expect(mockAnnounce).toHaveBeenCalledWith('Código copiado');
    expect(mockAddToast).toHaveBeenCalledWith('Código copiado', 'success', undefined, undefined, { suppressAnnounce: true });
    expect(button).toHaveAccessibleName('Copiar código EXT-0042');
    await user.click(screen.getByRole('button', { name: 'Abrir link do card' }));
    expect(mockOpenTaskLink).toHaveBeenCalledWith('https://example.com/card/42', expect.any(Object));
  });

  it('informa falha de cópia sem anunciar sucesso', async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('denied'));
    render(<MemoryRouter><TaskDetailModal isOpen onClose={vi.fn()} task={{ ...task, code: 'EXT-0042' }} statuses={statuses} /></MemoryRouter>);
    await user.click(screen.getByRole('button', { name: 'Copiar código EXT-0042' }));
    expect(mockAnnounce).toHaveBeenCalledWith('Não foi possível copiar o código. Tente novamente.');
    expect(mockAnnounce).not.toHaveBeenCalledWith('Código copiado');
    expect(mockAddToast).toHaveBeenCalledWith('Não foi possível copiar o código. Tente novamente.', 'error', undefined, undefined, { suppressAnnounce: true });
    expect(screen.queryByRole('button', { name: 'Abrir link do card' })).not.toBeInTheDocument();
  });

  it('mantém link sem código e omite copiar quando não há referência', async () => {
    render(<MemoryRouter><TaskDetailModal isOpen onClose={vi.fn()} task={{ ...task, link: 'https://example.com' }} statuses={statuses} /></MemoryRouter>);
    expect(await screen.findByRole('button', { name: 'Abrir link do card' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Copiar código/ })).not.toBeInTheDocument();
  });

  it('usa readingMode (role="document") para permitir leitura linear no leitor de tela', async () => {
    mockLoadTaskNotes.mockResolvedValue([{
      id: 'note-1',
      taskId: task.id,
      type: 1,
      content: 'Nota com [link](https://example.com)',
      createdAt: '2024-01-01',
      updatedAt: '2024-01-01',
    }]);
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
    const markdownRegions = await screen.findAllByTestId('task-markdown');
    expect(markdownRegions).toHaveLength(2);
    markdownRegions.forEach((region) => {
      expect(region).toHaveAttribute('data-tab-navigation', 'enabled');
    });
  });

  it('vincula conversa pelo HistoryPicker e chama setTaskConversation com o ID', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <TaskDetailModal isOpen onClose={vi.fn()} task={task} statuses={statuses} />
      </MemoryRouter>,
    );

    await user.click(screen.getByRole('button', { name: /Vincular conversa/ }));
    // Opções chegam de forma assíncrona via GetConversations(); aguarda renderizar.
    const option = await screen.findByRole('option', { name: /Conversa X/ });
    fireEvent.mouseDown(option);

    expect(mockSetTaskConversation).toHaveBeenCalledWith('10', '5');
  });

  it('desvincula conversa ao escolher "Nenhuma" (setTaskConversation com null)', async () => {
    const user = userEvent.setup();
    const linkedTask = { ...task, conversationId: '5' } as unknown as Task;
    render(
      <MemoryRouter>
        <TaskDetailModal isOpen onClose={vi.fn()} task={linkedTask} statuses={statuses} />
      </MemoryRouter>,
    );

    await user.click(screen.getByRole('button', { name: /Alterar conversa vinculada/ }));
    const noneOption = await screen.findByRole('option', { name: 'Nenhuma' });
    fireEvent.mouseDown(noneOption);

    expect(mockSetTaskConversation).toHaveBeenCalledWith('10', null);
  });

  it('reflete o vínculo do cache mesmo com a prop desatualizada (snapshot do clique)', async () => {
    // KanbanBoard/TasksTable passam a task como snapshot em useState; o update
    // otimista do store atualiza o cache, e o modal deve preferir a versão viva.
    mockTaskLists.set('1', { tasks: [{ ...task, conversationId: '5' }] });
    render(
      <MemoryRouter>
        <TaskDetailModal isOpen onClose={vi.fn()} task={task} statuses={statuses} />
      </MemoryRouter>,
    );

    // Badge de conversa e picker passam a refletir o vínculo do cache.
    expect(await screen.findByRole('button', { name: 'Conversa vinculada' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Alterar conversa vinculada/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Vincular conversa/ })).not.toBeInTheDocument();
  });
});
