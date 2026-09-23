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
    useTranslation: () => ({ t: (_key: string, fallback?: string, options?: Record<string, string>) => {
      let text = fallback ?? _key;
      for (const [name, value] of Object.entries(options ?? {})) text = text.replace(`{{${name}}}`, value);
      return text;
    } }),
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
    // Grid enxuto: sem ID/evento técnico; ação sem link mostra texto amigável.
    expect(screen.getByRole('columnheader', { name: 'Ação' })).toBeInTheDocument();
    expect(screen.queryByRole('columnheader', { name: 'ID' })).not.toBeInTheDocument();
    expect(screen.getByText('Publica evento')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Nova ação' })).toBeInTheDocument();
    // O grid foca a primeira linha ao montar: Editar/Apagar já nascem prontos.
    await waitFor(() => expect(screen.getByRole('button', { name: 'Editar' })).toBeEnabled());
    expect(screen.getByRole('button', { name: 'Deletar' })).toBeEnabled();
    // E o foco já está dentro do grid ao entrar na tela.
    expect(screen.getByRole('grid').contains(document.activeElement)).toBe(true);
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

    expect(mockRequestConfirm).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.stringContaining('"Investigar"'),
    }));
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

  it('devolve o foco ao grid ao fechar o modal de item', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" onClose={vi.fn()} />);
    const grid = await screen.findByRole('grid');
    await user.click(screen.getByRole('button', { name: 'Nova ação' }));
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'x' } });
    fireEvent.change(screen.getByLabelText(/Rótulo/), { target: { value: 'X' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));
    await waitFor(() => expect(grid.contains(document.activeElement)).toBe(true));
  });

  it('apagando a última ação, o foco vai para Nova ação', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" onClose={vi.fn()} />);
    await screen.findByRole('grid');

    const rowButtons = await screen.findAllByRole('button', { name: 'Ações' });
    await user.click(rowButtons[0]);
    await user.click(await screen.findByRole('menuitem', { name: 'Deletar' }));

    await waitFor(() => expect(screen.queryByRole('grid')).not.toBeInTheDocument());
    await waitFor(() => expect(document.activeElement).toHaveAccessibleName('Nova ação'));
  });

  it('Enter na linha abre a edição (atalho de teclado do grid)', async () => {
    render(<CustomActionsEditor taskListId="1" onClose={vi.fn()} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'Enter' });
    expect(await screen.findByRole('heading', { name: 'Editar ação: Investigar' })).toBeInTheDocument();
    expect(screen.getByLabelText(/Rótulo/)).toHaveValue('Investigar');
  });

  it('desabilita a toolbar durante o salvamento', async () => {
    const user = userEvent.setup();
    let resolveSave!: () => void;
    mockSetTaskListCustomActions.mockImplementationOnce(() => new Promise<void>((res) => { resolveSave = res; }));
    render(<CustomActionsEditor taskListId="1" onClose={vi.fn()} />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: 'Salvar' }));
    expect(screen.getByRole('button', { name: 'Nova ação' })).toBeDisabled();

    resolveSave();
    await waitFor(() => expect(screen.getByRole('button', { name: 'Nova ação' })).toBeEnabled());
  });

  it('não rouba foco de fora do editor ao carregar', async () => {
    let resolveLoad!: (value: { actions: unknown[] }) => void;
    mockGetTaskListCustomActions.mockImplementationOnce(
      () => new Promise<{ actions: unknown[] }>((res) => { resolveLoad = res; }),
    );
    render(
      <div>
        <button type="button">Externo</button>
        <CustomActionsEditor taskListId="1" onClose={vi.fn()} />
      </div>,
    );
    const ext = screen.getByRole('button', { name: 'Externo' });
    ext.focus();
    resolveLoad({ actions: [{ id: 'x', label: 'X' }] });
    await screen.findByRole('grid');
    await new Promise<void>((r) => { window.setTimeout(r, 50); });
    expect(document.activeElement).toBe(ext);
  });
});
