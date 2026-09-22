import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import WorkflowEditor from './WorkflowEditor';
import type { TaskListWorkflow } from '../../types/tasklist';

const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();
const mockRequestConfirm = vi.fn();

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({ t: (key: string, fallback?: string, options?: Record<string, string>) => {
      let text = fallback ?? key;
      for (const [name, value] of Object.entries(options ?? {})) text = text.replace(`{{${name}}}`, value);
      return text;
    } }),
  };
});

vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: ReturnType<typeof vi.fn> }) => unknown) => selector({
    addToast: mockAddToast,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce }),
}));

vi.mock('../../hooks/useConfirm', () => ({
  useConfirm: () => mockRequestConfirm,
}));

const workflow: TaskListWorkflow = {
  id: 'w1',
  taskListId: '1',
  statuses: [
    { id: 1, order: 0, label: 'A Fazer', color: 'var(--color-info)', icon: '⌛' },
    { id: 2, order: 1, label: 'Em Progresso', color: 'var(--color-warning)', icon: '🔄' },
  ],
  allowedTransitions: { 1: [2], 2: [] },
  initialStatusId: 1,
  createdAt: '2024-01-01',
  updatedAt: '2024-01-01',
};

describe('WorkflowEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockRequestConfirm.mockResolvedValue(true);
  });

  it('lista os status no grid com toolbar Novo/Editar/Apagar', async () => {
    render(<WorkflowEditor workflow={workflow} onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(await screen.findByRole('grid', { name: 'Lista de status do workflow' })).toBeInTheDocument();
    expect(screen.getByRole('row', { name: /A Fazer/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Adicionar Status' })).toBeInTheDocument();
    // O grid foca a primeira linha ao montar: Editar/Apagar já nascem prontos,
    // e o foco já está dentro do grid ao entrar na tela.
    await waitFor(() => expect(screen.getByRole('button', { name: 'Editar' })).toBeEnabled());
    expect(screen.getByRole('button', { name: 'Deletar' })).toBeEnabled();
    expect(screen.getByRole('grid').contains(document.activeElement)).toBe(true);
  });

  it('cria status pelo modal e persiste no Salvar', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<WorkflowEditor workflow={workflow} onSave={onSave} onCancel={vi.fn()} />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: 'Adicionar Status' }));
    expect(await screen.findByRole('heading', { name: 'Novo status' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/Nome/), { target: { value: 'Revisão' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    expect(await screen.findByRole('row', { name: /Revisão/ })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Salvar Workflow' }));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [statuses] = onSave.mock.calls[0];
    expect((statuses as { id: number; label: string }[]).map((s) => [s.id, s.label]))
      .toEqual([[1, 'A Fazer'], [2, 'Em Progresso'], [3, 'Revisão']]);
  });

  it('barra Aplicar sem nome', async () => {
    const user = userEvent.setup();
    render(<WorkflowEditor workflow={workflow} onSave={vi.fn()} onCancel={vi.fn()} />);
    await screen.findByRole('grid');
    await user.click(screen.getByRole('button', { name: 'Adicionar Status' }));
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));
    expect(mockAddToast).toHaveBeenCalledWith('Dê um nome ao status', 'error');
    expect(screen.getByRole('heading', { name: 'Novo status' })).toBeInTheDocument();
  });

  it('edita nome pelo modal via toolbar após focar a linha', async () => {
    const user = userEvent.setup();
    render(<WorkflowEditor workflow={workflow} onSave={vi.fn()} onCancel={vi.fn()} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Editar' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Editar' }));
    expect(await screen.findByRole('heading', { name: 'Editar status: A Fazer' })).toBeInTheDocument();
    const nameInput = await screen.findByLabelText(/Nome/);
    expect(nameInput).toHaveValue('A Fazer');
    fireEvent.change(nameInput, { target: { value: 'Na Fila' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));
    expect(await screen.findByRole('row', { name: /Na Fila/ })).toBeInTheDocument();
  });

  it('Alt+Setas reordena o status focado', async () => {
    render(<WorkflowEditor workflow={workflow} onSave={vi.fn()} onCancel={vi.fn()} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    const rows = await screen.findAllByRole('row', { name: /Fazer|Progresso/ });
    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toMatch(/Em Progresso/);
    expect(rows[1].textContent).toMatch(/A Fazer/);
  });

  it('apaga status com tarefas pedindo migração e persiste', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(
      <WorkflowEditor workflow={workflow} taskCountsByStatus={{ 2: 3 }} onSave={onSave} onCancel={vi.fn()} />,
    );
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    await user.click(screen.getByRole('button', { name: 'Deletar' }));
    expect(mockRequestConfirm).toHaveBeenCalled();
    // Seção de migração aparece e o grid perde a linha.
    const migrationGroup = await screen.findByRole('group', { name: /Migração do status/ });
    expect(migrationGroup).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/Migrar tarefas para/), { target: { value: '1' } });
    await user.click(screen.getByRole('button', { name: 'Salvar Workflow' }));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [statuses, , , migration] = onSave.mock.calls[0];
    expect((statuses as { id: number }[]).map((s) => s.id)).toEqual([1]);
    expect(migration).toEqual({ 2: 1 });
  });

  it('chips de transição expõem estado e alternam', async () => {
    const user = userEvent.setup();
    render(<WorkflowEditor workflow={workflow} onSave={vi.fn()} onCancel={vi.fn()} />);
    expect(await screen.findByRole('group', { name: 'Transições de A Fazer' })).toBeInTheDocument();
    const chips = screen.getAllByRole('button', { name: /Em Progresso/ });
    expect(chips[0]).toHaveAttribute('aria-pressed', 'true');
    await user.click(chips[0]);
    expect(chips[0]).toHaveAttribute('aria-pressed', 'false');
  });
});
