import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CustomActionsEditor from './CustomActionsEditor';
import { Modal } from '../ui/Modal';
import { DATAGRID_ENTRY_SELECTOR } from '../ui/DataGrid';

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
    render(<CustomActionsEditor taskListId="1" />);
    expect(await screen.findByRole('grid', { name: 'Lista de ações customizadas' })).toBeInTheDocument();
    expect(screen.getByRole('row', { name: /Investigar/ })).toBeInTheDocument();
    // Grid enxuto: sem ID/evento técnico; ação sem link mostra texto amigável.
    expect(screen.getByRole('columnheader', { name: 'Ação' })).toBeInTheDocument();
    expect(screen.queryByRole('columnheader', { name: 'ID' })).not.toBeInTheDocument();
    expect(screen.getByText('Publica evento')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Nova ação/ })).toBeInTheDocument();
    // O grid foca a primeira linha ao montar: Editar/Apagar já nascem prontos.
    await waitFor(() => expect(screen.getByRole('button', { name: 'Editar' })).toBeEnabled());
    expect(screen.getByRole('button', { name: 'Deletar' })).toBeEnabled();
    // E o foco já está dentro do grid ao entrar na tela.
    expect(screen.getByRole('grid').contains(document.activeElement)).toBe(true);
  });

  it('cria ação pelo modal e persiste na hora, sem Salvar/Cancelar na tela', async () => {
    const user = userEvent.setup();
    const onSaved = vi.fn();
    render(<CustomActionsEditor taskListId="1" onSaved={onSaved} />);
    await screen.findByRole('grid');
    expect(screen.queryByRole('button', { name: 'Salvar' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Cancelar' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Nova ação/ }));
    // fireEvent (não user.type): o auto-focus do Modal rouba o foco no jsdom
    // (offsetParent sempre null) e os caracteres se perderiam.
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'atualizar' } });
    fireEvent.change(screen.getByLabelText(/Rótulo/), { target: { value: 'Atualizar' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    expect(await screen.findByRole('row', { name: /Atualizar/ })).toBeInTheDocument();
    expect(mockSetTaskListCustomActions).toHaveBeenCalledTimes(1);
    const [listId, json] = mockSetTaskListCustomActions.mock.calls[0];
    expect(listId).toBe('1');
    const parsed = JSON.parse(json as string);
    expect(parsed.actions.map((a: { id: string }) => a.id).sort()).toEqual(['atualizar', 'investigar']);
    expect(parsed.actions[1]).not.toHaveProperty('_uiId');
    expect(onSaved).toHaveBeenCalledTimes(1);
    expect(mockAnnounce).toHaveBeenCalledWith('Ação adicionada');
  });

  it('falha ao salvar mantém o modal aberto e não altera o grid', async () => {
    const user = userEvent.setup();
    mockSetTaskListCustomActions.mockRejectedValueOnce(new Error('disco cheio'));
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: /Nova ação/ }));
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'x' } });
    fireEvent.change(screen.getByLabelText(/Rótulo/), { target: { value: 'Xis' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    await waitFor(() => expect(mockAddToast).toHaveBeenCalledWith('Falha ao salvar ações: disco cheio', 'error'));
    expect(screen.getByLabelText(/Rótulo/)).toHaveValue('Xis');
    expect(screen.queryByRole('row', { name: /Xis/ })).not.toBeInTheDocument();
  });

  it('Ctrl+N abre Nova ação, e não empilha com o modal do item já aberto', async () => {
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');

    fireEvent.keyDown(document, { key: 'n', ctrlKey: true });
    expect(await screen.findByRole('heading', { name: 'Nova ação' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'rascunho' } });

    fireEvent.keyDown(document, { key: 'n', ctrlKey: true });
    // O rascunho não foi descartado por uma nova abertura.
    expect(screen.getByLabelText(/ID/)).toHaveValue('rascunho');
    expect(screen.getAllByRole('heading', { name: 'Nova ação' })).toHaveLength(1);
  });

  it('barra Aplicar sem ID/Rótulo e acusa ID duplicado', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: /Nova ação/ }));
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
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');

    const rowButtons = await screen.findAllByRole('button', { name: 'Ações' });
    await user.click(rowButtons[0]);
    await user.click(await screen.findByRole('menuitem', { name: 'Editar' }));

    const labelInput = screen.getByLabelText(/Rótulo/);
    expect(labelInput).toHaveValue('Investigar');
    fireEvent.change(labelInput, { target: { value: 'Investigar fundo' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    expect(await screen.findByRole('row', { name: /Investigar fundo/ })).toBeInTheDocument();
    expect(mockSetTaskListCustomActions).toHaveBeenCalledTimes(1);
    const parsed = JSON.parse(mockSetTaskListCustomActions.mock.calls[0][1] as string);
    expect(parsed.actions[0].label).toBe('Investigar fundo');
  });

  it('apaga com confirmação e persiste a remoção na hora', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');

    const rowButtons = await screen.findAllByRole('button', { name: 'Ações' });
    await user.click(rowButtons[0]);
    await user.click(await screen.findByRole('menuitem', { name: 'Deletar' }));

    expect(mockRequestConfirm).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.stringContaining('"Investigar"'),
    }));
    await waitFor(() => expect(screen.queryByRole('row', { name: /Investigar/ })).not.toBeInTheDocument());
    expect(mockSetTaskListCustomActions).toHaveBeenCalledTimes(1);
    expect(mockSetTaskListCustomActions.mock.calls[0][1]).toBe('');
  });

  it('não apaga quando a confirmação é recusada', async () => {
    const user = userEvent.setup();
    mockRequestConfirm.mockResolvedValueOnce(false);
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');

    const rowButtons = await screen.findAllByRole('button', { name: 'Ações' });
    await user.click(rowButtons[0]);
    await user.click(await screen.findByRole('menuitem', { name: 'Deletar' }));

    await waitFor(() => expect(mockRequestConfirm).toHaveBeenCalled());
    expect(screen.getByRole('row', { name: /Investigar/ })).toBeInTheDocument();
    expect(mockSetTaskListCustomActions).not.toHaveBeenCalled();
  });

  it('Cancelar do modal do item descarta o rascunho sem persistir', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');
    await user.click(screen.getByRole('button', { name: /Nova ação/ }));
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'descartar' } });
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(screen.queryByLabelText(/ID/)).not.toBeInTheDocument();
    expect(mockSetTaskListCustomActions).not.toHaveBeenCalled();
  });

  it('devolve o foco ao grid ao fechar o modal de item', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" />);
    const grid = await screen.findByRole('grid');
    await user.click(screen.getByRole('button', { name: /Nova ação/ }));
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'x' } });
    fireEvent.change(screen.getByLabelText(/Rótulo/), { target: { value: 'X' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));
    await waitFor(() => expect(grid.contains(document.activeElement)).toBe(true));
  });

  it('apagando a última ação, o foco vai para Nova ação', async () => {
    const user = userEvent.setup();
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');

    const rowButtons = await screen.findAllByRole('button', { name: 'Ações' });
    await user.click(rowButtons[0]);
    await user.click(await screen.findByRole('menuitem', { name: 'Deletar' }));

    await waitFor(() => expect(screen.queryByRole('grid')).not.toBeInTheDocument());
    await waitFor(() => expect(document.activeElement).toHaveAccessibleName('Nova ação, Ctrl+N'));
  });

  it('Enter na linha abre a edição (atalho de teclado do grid)', async () => {
    render(<CustomActionsEditor taskListId="1" />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'Enter' });
    expect(await screen.findByRole('heading', { name: 'Editar ação: Investigar' })).toBeInTheDocument();
    expect(screen.getByLabelText(/Rótulo/)).toHaveValue('Investigar');
  });

  it('enquanto salva, Aplicar repetido não duplica e a toolbar fica desabilitada', async () => {
    const user = userEvent.setup();
    let resolveSave!: () => void;
    mockSetTaskListCustomActions.mockImplementationOnce(() => new Promise<void>((res) => { resolveSave = res; }));
    render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: /Nova ação/ }));
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'x' } });
    fireEvent.change(screen.getByLabelText(/Rótulo/), { target: { value: 'X' } });
    const apply = screen.getByRole('button', { name: 'Aplicar' });
    fireEvent.click(apply);
    fireEvent.click(apply);
    await waitFor(() => expect(screen.getByRole('button', { name: /Nova ação/ })).toBeDisabled());
    // O botão mantém o nome acessível (sem spinner mudo) durante o salvamento.
    expect(screen.getByRole('button', { name: 'Aplicar' })).toBeInTheDocument();
    expect(mockSetTaskListCustomActions).toHaveBeenCalledTimes(1);

    resolveSave();
    await waitFor(() => expect(screen.getByRole('button', { name: /Nova ação/ })).toBeEnabled());
    expect(await screen.findByRole('row', { name: /X/ })).toBeInTheDocument();
  });

  it('reaberto durante um salvamento em voo, só lê as ações depois que ele termina', async () => {
    const user = userEvent.setup();
    let resolveSave!: () => void;
    mockSetTaskListCustomActions.mockImplementationOnce(() => new Promise<void>((res) => { resolveSave = res; }));
    const { unmount } = render(<CustomActionsEditor taskListId="1" />);
    await screen.findByRole('grid');
    await user.click(screen.getByRole('button', { name: /Nova ação/ }));
    fireEvent.change(screen.getByLabelText(/ID/), { target: { value: 'x' } });
    fireEvent.change(screen.getByLabelText(/Rótulo/), { target: { value: 'X' } });
    fireEvent.click(screen.getByRole('button', { name: 'Aplicar' }));
    await waitFor(() => expect(mockSetTaskListCustomActions).toHaveBeenCalledTimes(1));

    unmount();
    render(<CustomActionsEditor taskListId="1" />);
    await new Promise<void>((r) => { window.setTimeout(r, 20); });
    expect(mockGetTaskListCustomActions).toHaveBeenCalledTimes(1);

    resolveSave();
    await waitFor(() => expect(mockGetTaskListCustomActions).toHaveBeenCalledTimes(2));
  });

  it('não rouba foco de fora do editor ao carregar', async () => {
    let resolveLoad!: (value: { actions: unknown[] }) => void;
    mockGetTaskListCustomActions.mockImplementationOnce(
      () => new Promise<{ actions: unknown[] }>((res) => { resolveLoad = res; }),
    );
    render(
      <div>
        <button type="button">Externo</button>
        <CustomActionsEditor taskListId="1" />
      </div>,
    );
    const ext = screen.getByRole('button', { name: 'Externo' });
    ext.focus();
    await waitFor(() => expect(mockGetTaskListCustomActions).toHaveBeenCalled());
    resolveLoad({ actions: [{ id: 'x', label: 'X' }] });
    await screen.findByRole('grid');
    await new Promise<void>((r) => { window.setTimeout(r, 50); });
    expect(document.activeElement).toBe(ext);
  });

  describe('dentro do Modal', () => {
    const originalOffsetParent = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetParent');

    beforeEach(() => {
      // Torna os controles "visíveis" para a heurística de foco do Modal, que
      // no jsdom os descartaria (offsetParent sempre null).
      Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
        configurable: true,
        get() { return document.body; },
      });
    });

    afterEach(() => {
      if (originalOffsetParent) {
        Object.defineProperty(HTMLElement.prototype, 'offsetParent', originalOffsetParent);
      } else {
        delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
      }
    });

    function renderInModal() {
      return render(
        <Modal isOpen onClose={vi.fn()} title="Ações customizadas" initialFocusSelector={DATAGRID_ENTRY_SELECTOR}>
          <CustomActionsEditor taskListId="1" />
        </Modal>,
      );
    }

    it('foca o grid quando os dados chegam, mesmo com o foco provisório no Fechar', async () => {
      let resolveLoad!: (value: { actions: unknown[] }) => void;
      mockGetTaskListCustomActions.mockImplementationOnce(
        () => new Promise<{ actions: unknown[] }>((res) => { resolveLoad = res; }),
      );
      renderInModal();
      // Enquanto carrega, o Modal só tem o Fechar para focar.
      const close = screen.getByRole('button', { name: 'ui.modal.close' });
      await waitFor(() => expect(close).toHaveFocus());

      resolveLoad({ actions: seedActions });
      const grid = await screen.findByRole('grid');
      await waitFor(() => expect(grid.contains(document.activeElement)).toBe(true));
      expect(document.activeElement).toHaveAttribute('role', 'gridcell');
    });

    it('com dados já disponíveis, o foco inicial fica no grid', async () => {
      renderInModal();
      const grid = await screen.findByRole('grid');
      await waitFor(() => expect(document.activeElement).toHaveAttribute('role', 'gridcell'));
      // Passada a verificação de ~150ms do Modal, o foco continua no grid.
      await new Promise<void>((r) => { window.setTimeout(r, 250); });
      expect(grid.contains(document.activeElement)).toBe(true);
    });

    it('sem ações, o foco inicial vai para Nova ação', async () => {
      mockGetTaskListCustomActions.mockResolvedValueOnce({ actions: [] });
      renderInModal();
      await screen.findByText('Nenhuma ação customizada definida.');
      await waitFor(() => expect(document.activeElement).toHaveAccessibleName('Nova ação, Ctrl+N'));
    });

    it('Esc com o foco no grid fecha o modal', async () => {
      const onClose = vi.fn();
      render(
        <Modal isOpen onClose={onClose} title="Ações customizadas" initialFocusSelector={DATAGRID_ENTRY_SELECTOR}>
          <CustomActionsEditor taskListId="1" />
        </Modal>,
      );
      await waitFor(() => expect(document.activeElement).toHaveAttribute('role', 'gridcell'));
      fireEvent.keyDown(document.activeElement as HTMLElement, { key: 'Escape' });
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('Ctrl+N no grid abre Nova ação por cima do modal', async () => {
      renderInModal();
      await waitFor(() => expect(document.activeElement).toHaveAttribute('role', 'gridcell'));
      fireEvent.keyDown(document.activeElement as HTMLElement, { key: 'n', ctrlKey: true });
      expect(await screen.findByRole('heading', { name: 'Nova ação' })).toBeInTheDocument();
    });
  });
});
