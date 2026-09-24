import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import WorkflowEditor from './WorkflowEditor';
import { Modal } from '../ui/Modal';
import { DATAGRID_ENTRY_SELECTOR } from '../ui/DataGrid';
import { whenSavesSettled } from '../../lib/serialSaveQueue';
import ptBR from '../../locales/pt-BR';
import en from '../../locales/en';
import es from '../../locales/es';
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

const mockAnnounceRequest = vi.fn();

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce, announceRequest: mockAnnounceRequest }),
}));

vi.mock('../../services/audioFeedback', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../services/audioFeedback')>()),
  playSound: vi.fn(),
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

type SavedStatus = { id: number; label: string; order: number };

function rowTexts(): string[] {
  return screen.getAllByRole('row', { name: /Fazer|Progresso|Revisão/ }).map((r) => r.textContent ?? '');
}

describe('WorkflowEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockRequestConfirm.mockResolvedValue(true);
  });

  it('lista os status no grid com toolbar Novo/Editar/Apagar, sem Salvar/Cancelar', async () => {
    render(<WorkflowEditor workflow={workflow} onSave={vi.fn()} />);
    expect(await screen.findByRole('grid', { name: 'Lista de status do workflow' })).toBeInTheDocument();
    expect(screen.getByRole('row', { name: /A Fazer/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Adicionar Status, Ctrl+N' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Salvar/ })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Cancelar' })).not.toBeInTheDocument();
    // O grid foca a primeira linha ao montar: Editar/Apagar já nascem prontos,
    // e o foco já está dentro do grid ao entrar na tela.
    await waitFor(() => expect(screen.getByRole('button', { name: 'Editar' })).toBeEnabled());
    expect(screen.getByRole('button', { name: 'Deletar' })).toBeEnabled();
    expect(screen.getByRole('grid').contains(document.activeElement)).toBe(true);
  });

  it('cria status pelo modal e persiste na hora', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: /Adicionar Status/ }));
    expect(await screen.findByRole('heading', { name: 'Novo status' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/Nome/), { target: { value: 'Revisão' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    expect(await screen.findByRole('row', { name: /Revisão/ })).toBeInTheDocument();
    expect(onSave).toHaveBeenCalledTimes(1);
    const [statuses, transitions, initialId, migration] = onSave.mock.calls[0];
    expect((statuses as SavedStatus[]).map((s) => [s.id, s.label]))
      .toEqual([[1, 'A Fazer'], [2, 'Em Progresso'], [3, 'Revisão']]);
    expect(transitions).toEqual({ 1: [2], 2: [], 3: [] });
    expect(initialId).toBe(1);
    expect(migration).toEqual({});
    expect(mockAnnounce).toHaveBeenCalledWith('Status adicionado');
  });

  it('falha ao salvar mantém o modal aberto e não altera o grid', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockRejectedValue(new Error('banco travado'));
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    await screen.findByRole('grid');

    await user.click(screen.getByRole('button', { name: /Adicionar Status/ }));
    fireEvent.change(screen.getByLabelText(/Nome/), { target: { value: 'Revisão' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    await waitFor(() => expect(mockAddToast).toHaveBeenCalledWith('Erro ao salvar workflow: banco travado', 'error'));
    expect(screen.getByRole('heading', { name: 'Novo status' })).toBeInTheDocument();
    expect(screen.getByLabelText(/Nome/)).toHaveValue('Revisão');
    expect(screen.queryByRole('row', { name: /Revisão/ })).not.toBeInTheDocument();
  });

  it('enquanto o Aplicar salva, Esc e Cancelar não fecham o formulário; a falha mantém o rascunho', async () => {
    const user = userEvent.setup();
    let rejectSave!: (error: Error) => void;
    const onSave = vi.fn(() => new Promise<void>((_res, rej) => { rejectSave = rej; }));
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    await screen.findByRole('grid');
    await user.click(screen.getByRole('button', { name: /Adicionar Status/ }));
    fireEvent.change(screen.getByLabelText(/Nome/), { target: { value: 'Revisão' } });
    fireEvent.click(screen.getByRole('button', { name: 'Aplicar' }));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));

    fireEvent.keyDown(screen.getByLabelText(/Nome/), { key: 'Escape' });
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(screen.getByRole('heading', { name: 'Novo status' })).toBeInTheDocument();

    rejectSave(new Error('banco travado'));
    await waitFor(() => expect(mockAddToast).toHaveBeenCalledWith('Erro ao salvar workflow: banco travado', 'error'));
    expect(screen.getByLabelText(/Nome/)).toHaveValue('Revisão');
    // Sem salvamento pendente, o Cancelar volta a fechar.
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));
    expect(screen.queryByRole('heading', { name: 'Novo status' })).not.toBeInTheDocument();
  });

  it('fechar e reabrir o editor da mesma tasklist não reaproveita IDs de status', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn((_statuses: SavedStatus[]) => Promise.resolve());
    const key = 'teste:workflow-ids';
    const createStatus = async (label: string) => {
      await user.click(screen.getByRole('button', { name: /Adicionar Status/ }));
      fireEvent.change(screen.getByLabelText(/Nome/), { target: { value: label } });
      await user.click(screen.getByRole('button', { name: 'Aplicar' }));
    };

    const { unmount } = render(<WorkflowEditor workflow={workflow} onSave={onSave} saveQueueKey={key} />);
    await screen.findByRole('grid');
    await createStatus('Revisão');
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave.mock.calls[0][0].map((s) => s.id)).toEqual([1, 2, 3]);
    unmount();

    // Reaberto sem o status 3 (removido nesse meio-tempo): o próximo é o 4.
    render(<WorkflowEditor workflow={workflow} onSave={onSave} saveQueueKey={key} />);
    await screen.findByRole('grid');
    await createStatus('Homologação');
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2));
    expect(onSave.mock.calls[1][0].map((s) => s.id)).toEqual([1, 2, 4]);
  });

  it.each([['pt-BR', ptBR], ['en', en], ['es', es]])(
    'mensagem de nome vazio em %s não depende de interpolação',
    (_lang, locale) => {
      // O editor chama a chave sem parâmetros; um placeholder apareceria cru.
      expect(locale.translation.tasklist.workflow.emptyStatusName).not.toMatch(/\{\{/);
    },
  );

  it('barra Aplicar sem nome', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    await screen.findByRole('grid');
    await user.click(screen.getByRole('button', { name: /Adicionar Status/ }));
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));
    expect(mockAddToast).toHaveBeenCalledWith('Dê um nome ao status', 'error');
    expect(screen.getByRole('heading', { name: 'Novo status' })).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it('edita nome pelo modal via toolbar e persiste', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
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
    expect(onSave).toHaveBeenCalledTimes(1);
    expect((onSave.mock.calls[0][0] as SavedStatus[])[0].label).toBe('Na Fila');
  });

  it('Alt+Setas reordena o status focado e persiste a nova ordem', async () => {
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    await waitFor(() => expect(rowTexts()[0]).toMatch(/Em Progresso/));
    expect(rowTexts()[1]).toMatch(/A Fazer/);
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect((onSave.mock.calls[0][0] as SavedStatus[]).map((s) => [s.id, s.order])).toEqual([[2, 0], [1, 1]]);
  });

  it('reordenação recusada pelo backend volta à ordem salva', async () => {
    const onSave = vi.fn().mockRejectedValue(new Error('offline'));
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    await waitFor(() => expect(mockAddToast).toHaveBeenCalledWith('Erro ao salvar workflow: offline', 'error'));
    await waitFor(() => expect(rowTexts()[0]).toMatch(/A Fazer/));
  });

  it('salvamentos rápidos em sequência são enviados em ordem, um por vez', async () => {
    const resolvers: Array<() => void> = [];
    const onSave = vi.fn((_statuses: SavedStatus[]) => new Promise<void>((res) => { resolvers.push(res); }));
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    fireEvent.keyDown(grid, { key: 'ArrowUp', altKey: true });
    // O segundo só sai depois que o primeiro termina.
    await new Promise<void>((r) => { window.setTimeout(r, 20); });
    expect(onSave).toHaveBeenCalledTimes(1);
    resolvers[0]();
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2));
    resolvers[1]();
    expect(onSave.mock.calls[1][0].map((s) => s.id)).toEqual([1, 2]);
  });

  it('a fila compartilhada inclui as alterações que ainda aguardam a vez', async () => {
    const resolvers: Array<() => void> = [];
    const onSave = vi.fn(() => new Promise<void>((res) => { resolvers.push(res); }));
    const key = 'teste:workflow-fila';
    const { unmount } = render(<WorkflowEditor workflow={workflow} onSave={onSave} saveQueueKey={key} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    fireEvent.keyDown(grid, { key: 'ArrowUp', altKey: true });
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    // Fechar o editor não descarta o que ainda não saiu.
    unmount();
    let settled = false;
    void whenSavesSettled(key).then(() => { settled = true; });
    resolvers[0]();
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2));
    expect(settled).toBe(false);
    resolvers[1]();
    await waitFor(() => expect(settled).toBe(true));
  });

  it('reordenar enquanto uma remoção está salvando não traz o status de volta', async () => {
    const user = userEvent.setup();
    const resolvers: Array<() => void> = [];
    const onSave = vi.fn((_statuses: SavedStatus[]) => new Promise<void>((res) => { resolvers.push(res); }));
    const threeStatuses: TaskListWorkflow = {
      ...workflow,
      statuses: [...workflow.statuses, { id: 3, order: 2, label: 'Feito', color: 'var(--color-success)', icon: '✅' }],
    };
    render(<WorkflowEditor workflow={threeStatuses} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    await user.click(screen.getByRole('button', { name: 'Deletar' }));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    fireEvent.keyDown(grid, { key: 'ArrowUp' });
    fireEvent.keyDown(grid, { key: 'ArrowUp' });
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    resolvers[0]();
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(2));
    resolvers[1]();
    expect(onSave.mock.calls[0][0].map((s) => s.id)).toEqual([1, 2]);
    expect(onSave.mock.calls[1][0].map((s) => s.id)).toEqual([2, 1]);
    await waitFor(() => expect(screen.queryByRole('row', { name: /Feito/ })).not.toBeInTheDocument());
    expect(rowTexts()[0]).toMatch(/Em Progresso/);
    expect(rowTexts()[1]).toMatch(/A Fazer/);
  });

  it('apaga status sem tarefas com confirmação e persiste', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    await user.click(screen.getByRole('button', { name: 'Deletar' }));
    expect(mockRequestConfirm).toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole('row', { name: /Em Progresso/ })).not.toBeInTheDocument());
    const [statuses, transitions, , migration] = onSave.mock.calls[0];
    expect((statuses as SavedStatus[]).map((s) => s.id)).toEqual([1]);
    expect(transitions).toEqual({ 1: [] });
    expect(migration).toEqual({});
  });

  it('apagar status com tarefas pergunta o destino da migração e persiste', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    const threeStatuses: TaskListWorkflow = {
      ...workflow,
      statuses: [...workflow.statuses, { id: 3, order: 2, label: 'Feito', color: 'var(--color-success)', icon: '✅' }],
    };
    render(<WorkflowEditor workflow={threeStatuses} taskCountsByStatus={{ 2: 3 }} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    await user.click(screen.getByRole('button', { name: 'Deletar' }));

    // Não usa a confirmação simples: abre o diálogo de migração.
    expect(mockRequestConfirm).not.toHaveBeenCalled();
    // Contrato de decisão do AEP-0091: alertdialog anunciado na abertura.
    const dialog = await screen.findByRole('alertdialog', { name: 'Remover Status' });
    expect(dialog).toHaveAccessibleDescription(/tem 3 tarefa\(s\)/);
    await waitFor(() => expect(mockAnnounceRequest).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.stringMatching(/^Remover Status\. O status "Em Progresso" tem 3 tarefa\(s\)/),
      announcePriority: 'assertive',
    })));
    const select = screen.getByLabelText('Migrar tarefas para');
    expect(select).toHaveValue('1');
    fireEvent.change(select, { target: { value: '3' } });
    await user.click(screen.getByRole('button', { name: 'Remover e migrar' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [statuses, , , migration] = onSave.mock.calls[0];
    expect((statuses as SavedStatus[]).map((s) => s.id)).toEqual([1, 3]);
    expect(migration).toEqual({ 2: 3 });
    await waitFor(() => expect(screen.queryByRole('row', { name: /Em Progresso/ })).not.toBeInTheDocument());
    expect(mockAnnounce).toHaveBeenCalledWith('Status removido');
  });

  it('cancelar a migração não remove nem persiste', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    render(<WorkflowEditor workflow={workflow} taskCountsByStatus={{ 2: 1 }} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    await user.click(screen.getByRole('button', { name: 'Deletar' }));
    await user.click(await screen.findByRole('button', { name: 'Cancelar' }));
    expect(screen.getByRole('row', { name: /Em Progresso/ })).toBeInTheDocument();
    expect(onSave).not.toHaveBeenCalled();
  });

  it('transições configuradas no modal por status e persistem', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Editar' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Editar' }));

    // Grupo de transições do status 1 reflete o workflow (1→2 ativo).
    expect(await screen.findByRole('group', { name: 'Pode transicionar para' })).toBeInTheDocument();
    const target = screen.getByRole('checkbox', { name: '🔄 Em Progresso' });
    expect(target).toBeChecked();
    await user.click(target);
    expect(target).not.toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [, transitions] = onSave.mock.calls[0];
    expect(transitions).toEqual({ 1: [], 2: [] });
  });

  it('checkbox define o status inicial e persiste', async () => {
    const user = userEvent.setup();
    const onSave = vi.fn().mockResolvedValue(undefined);
    render(<WorkflowEditor workflow={workflow} onSave={onSave} />);
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Editar' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Editar' }));

    const initialBox = await screen.findByRole('checkbox', { name: 'Status Inicial' });
    expect(initialBox).not.toBeChecked();
    await user.click(initialBox);
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const [, , initialId] = onSave.mock.calls[0];
    expect(initialId).toBe(2);
  });

  it('Ctrl+N abre Novo status, e não empilha com o modal do item já aberto', async () => {
    render(<WorkflowEditor workflow={workflow} onSave={vi.fn()} />);
    await screen.findByRole('grid');
    fireEvent.keyDown(document, { key: 'n', ctrlKey: true });
    expect(await screen.findByRole('heading', { name: 'Novo status' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/Nome/), { target: { value: 'rascunho' } });
    fireEvent.keyDown(document, { key: 'N', ctrlKey: true });
    expect(screen.getByLabelText(/Nome/)).toHaveValue('rascunho');
  });

  it('abre o modal de item aninhado com formulário completo', async () => {
    // O foco inicial em modal aninhado depende de visibilidade real (jsdom
    // marca tudo como invisível e o Modal cai no container); o mecanismo de
    // auto-focus em si é coberto pelo Modal.test. Aqui provamos a abertura
    // aninhada com o formulário completo por cima do modal pai.
    const user = userEvent.setup();
    render(
      <Modal isOpen title="Editar Workflow" onClose={vi.fn()}>
        <WorkflowEditor workflow={workflow} onSave={vi.fn()} />
      </Modal>,
    );
    const grid = await screen.findByRole('grid');
    fireEvent.focus(grid);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Editar' })).toBeEnabled());
    await user.click(screen.getByRole('button', { name: 'Editar' }));
    expect(await screen.findByRole('heading', { name: 'Editar status: A Fazer' })).toBeInTheDocument();
    expect(screen.getByLabelText(/Nome/)).toHaveValue('A Fazer');
    expect(screen.getByRole('checkbox', { name: '🔄 Em Progresso' })).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'Status Inicial' })).toBeChecked();
  });

  describe('dentro do Modal', () => {
    let originalOffsetParent: PropertyDescriptor | undefined;

    beforeEach(() => {
      originalOffsetParent = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetParent');
      // Torna os controles "visíveis" para a heurística de foco do Modal.
      Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
        configurable: true,
        get() { return document.body; },
      });
      return () => {
        if (originalOffsetParent) {
          Object.defineProperty(HTMLElement.prototype, 'offsetParent', originalOffsetParent);
        } else {
          delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
        }
      };
    });

    it('o foco inicial fica no grid e não no Fechar', async () => {
      render(
        <Modal isOpen title="Editar Workflow" onClose={vi.fn()} initialFocusSelector={DATAGRID_ENTRY_SELECTOR}>
          <WorkflowEditor workflow={workflow} onSave={vi.fn()} />
        </Modal>,
      );
      const grid = await screen.findByRole('grid');
      await waitFor(() => expect(document.activeElement).toHaveAttribute('role', 'gridcell'));
      // Passada a verificação de ~150ms do Modal, o foco continua no grid.
      await new Promise<void>((r) => { window.setTimeout(r, 250); });
      expect(grid.contains(document.activeElement)).toBe(true);
    });

    it('Esc com o foco no grid fecha o modal', async () => {
      const onClose = vi.fn();
      render(
        <Modal isOpen title="Editar Workflow" onClose={onClose} initialFocusSelector={DATAGRID_ENTRY_SELECTOR}>
          <WorkflowEditor workflow={workflow} onSave={vi.fn()} />
        </Modal>,
      );
      await waitFor(() => expect(document.activeElement).toHaveAttribute('role', 'gridcell'));
      fireEvent.keyDown(document.activeElement as HTMLElement, { key: 'Escape' });
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('Esc no modal do item fecha só ele, não o do workflow', async () => {
      const onClose = vi.fn();
      render(
        <Modal isOpen title="Editar Workflow" onClose={onClose} initialFocusSelector={DATAGRID_ENTRY_SELECTOR}>
          <WorkflowEditor workflow={workflow} onSave={vi.fn()} />
        </Modal>,
      );
      await waitFor(() => expect(document.activeElement).toHaveAttribute('role', 'gridcell'));
      fireEvent.keyDown(document.activeElement as HTMLElement, { key: 'n', ctrlKey: true });
      expect(await screen.findByRole('heading', { name: 'Novo status' })).toBeInTheDocument();
      fireEvent.keyDown(screen.getByLabelText(/Nome/), { key: 'Escape' });
      await waitFor(() => expect(screen.queryByRole('heading', { name: 'Novo status' })).not.toBeInTheDocument());
      expect(onClose).not.toHaveBeenCalled();
    });
  });
});
