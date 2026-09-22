import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CustomActionsEditor from './CustomActionsEditor';

const mockGetTaskListCustomActions = vi.fn();
const mockSetTaskListCustomActions = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();
const mockRequestConfirm = vi.fn();

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({ t: (_key: string, fallback?: string) => fallback ?? _key }),
  };
});

vi.mock('../../store/taskListStore', () => ({
  useTaskListStore: (selector: (state: Record<string, unknown>) => unknown) => selector({
    getTaskListCustomActions: mockGetTaskListCustomActions,
    setTaskListCustomActions: mockSetTaskListCustomActions,
  }),
}));

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

const seedActions = [
  {
    id: 'investigar',
    label: 'Investigar',
    icon: '🔍',
    surfaces: ['card_menu'],
    event: 'tasklist.card.investigate_requested',
  },
];

describe('CustomActionsEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetTaskListCustomActions.mockResolvedValue({ actions: seedActions });
    mockSetTaskListCustomActions.mockResolvedValue(undefined);
    mockRequestConfirm.mockResolvedValue(true);
  });

  it('lista as ações no grid com toolbar Nova/Editar/Apagar', async () => {
    render(<CustomActionsEditor taskListId="1" onClose={vi.fn()} />);
    expect(await screen.findByRole('grid', { name: 'Lista de ações customizadas' })).toBeInTheDocument();
    expect(screen.getByRole('row', { name: /Investigar/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Nova ação' })).toBeInTheDocument();
    // Sem linha focada, Editar/Apagar começam desabilitados.
    expect(screen.getByRole('button', { name: 'Editar' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Deletar' })).toBeDisabled();
  });

  it('cria ação pelo modal e persiste no Salvar', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<CustomActionsEditor taskListId="1" onClose={onClose} />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: 'Nova ação' }));
    // fireEvent (não user.type): o auto-focus do Modal rouba o foco no jsdom
    // (offsetParent sempre null) e os caracteres se perderiam.
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'atualizar' } });
    fireEvent.change(screen.getByLabelText(/Rótulo/), { target: { value: 'Atualizar' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    expect(await screen.findByRole('row', { name: /Atualizar/ })).toBeInTheDocument();
    expect(mockSetTaskListCustomActions).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Salvar' }));
    await waitFor(() => expect(mockSetTaskListCustomActions).toHaveBeenCalledTimes(1));
    const [listId, json] = mockSetTaskListCustomActions.mock.calls[0];
    expect(listId).toBe('1');
    const parsed = JSON.parse(json as string);
    expect(parsed.actions.map((a: { id: string }) => a.id).sort()).toEqual(['atualizar', 'investigar']);
    expect(parsed.actions[1]).not.toHaveProperty('_uiId');
    expect(onClose).toHaveBeenCalled();
  });

  it('barra Aplicar sem ID/Rótulo e acusa ID duplicado', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" onClose={vi.fn()} />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: 'Nova ação' }));
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));
    expect(mockAddToast).toHaveBeenCalledWith('Preencha ID e Rótulo da ação', 'error');
    // Modal segue aberto.
    expect(screen.getByLabelText(/Rótulo/)).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'investigar' } });
    fireEvent.change(screen.getByLabelText(/Rótulo/), { target: { value: 'Outra' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));
    expect(mockAddToast).toHaveBeenCalledWith('Já existe uma ação com este ID', 'error');
    expect(screen.getByLabelText(/Rótulo/)).toBeInTheDocument();
    expect(mockSetTaskListCustomActions).not.toHaveBeenCalled();
  });

  it('edita ação pelo menu da linha', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" onClose={vi.fn()} />);
    await screen.findByRole('grid');

    const rowButtons = await screen.findAllByRole('button', { name: 'Ações' });
    await user.click(rowButtons[0]);
    await user.click(await screen.findByRole('menuitem', { name: 'Editar' }));

    const labelInput = screen.getByLabelText(/Rótulo/);
    expect(labelInput).toHaveValue('Investigar');
    fireEvent.change(labelInput, { target: { value: 'Investigar fundo' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    expect(await screen.findByRole('row', { name: /Investigar fundo/ })).toBeInTheDocument();
  });

  it('apaga com confirmação e persiste a remoção no Salvar', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" onClose={vi.fn()} />);
    await screen.findByRole('grid');

    const rowButtons = await screen.findAllByRole('button', { name: 'Ações' });
    await user.click(rowButtons[0]);
    await user.click(await screen.findByRole('menuitem', { name: 'Deletar' }));

    expect(mockRequestConfirm).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole('row', { name: /Investigar/ })).not.toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'Salvar' }));
    await waitFor(() => expect(mockSetTaskListCustomActions).toHaveBeenCalledTimes(1));
    expect(mockSetTaskListCustomActions.mock.calls[0][1]).toBe('');
  });

  it('Cancelar fecha sem persistir', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<CustomActionsEditor taskListId="1" onClose={onClose} />);
    await screen.findByRole('grid');
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(onClose).toHaveBeenCalled();
    expect(mockSetTaskListCustomActions).not.toHaveBeenCalled();
  });
});
