import { afterEach, describe, it, expect, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from '../../test/a11yAxe';
import { Combobox, ComboboxItem } from './Combobox';

const announceMock = vi.hoisted(() => vi.fn());

const mockItems: ComboboxItem[] = [
  { value: 'gpt-4', label: 'GPT-4' },
  { value: 'gpt-3.5', label: 'GPT-3.5 Turbo' },
  { value: 'claude-3', label: 'Claude 3' },
];

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: { value?: string }) => values?.value ? `${key}:${values.value}` : key,
  }),
}));

describe('Combobox - allowFreeInput', () => {
  afterEach(() => {
    announceMock.mockClear();
  });

  it('renderiza com items normais mostrando label selecionado', () => {
    const onSelect = vi.fn();
    render(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={onSelect}
        label="Selecionar Modelo"
      />
    );

    expect(screen.getByText('GPT-4')).toBeInTheDocument();
  });

  it('exibe label do valor digitado manualmente quando allowFreeInput está ativo', () => {
    const onSelect = vi.fn();

    // Quando allowFreeInput=true e selected contém um valor não na lista,
    // deve exibir esse valor ao invés do label padrão
    const { container } = render(
      <Combobox
        items={[]}
        selected="gemini-pro"
        onSelect={onSelect}
        label="Modelo"
        allowFreeInput={true}
      />
    );

    // Procura pelo aria-label que contém o valor selecionado
    const button = container.querySelector('button');
    expect(button).toHaveAttribute('aria-label', expect.stringContaining('gemini-pro'));
  });

  it('permite seleção de itens da lista mesmo com allowFreeInput ativo', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={onSelect}
        placeholder="Filtrar..."
        allowFreeInput={true}
      />
    );

    const button = screen.getByRole('button');
    await user.click(button);

    const input = screen.getByRole('combobox');
    await user.click(input);

    const gpt4Item = await screen.findByText('GPT-4');
    await user.click(gpt4Item);

    expect(onSelect).toHaveBeenCalledWith('gpt-4', expect.objectContaining({ value: 'gpt-4' }));
  });

  it('seleciona items normalmente quando allowFreeInput não está ativo', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={onSelect}
        placeholder="Filtrar..."
        allowFreeInput={false}
      />
    );

    const button = screen.getByRole('button');
    await user.click(button);

    const gpt4Item = await screen.findByText('GPT-4');
    await user.click(gpt4Item);

    expect(onSelect).toHaveBeenCalledWith('gpt-4', expect.objectContaining({ value: 'gpt-4' }));
  });

  it('mostra label padrão quando não há seleção', () => {
    const onSelect = vi.fn();

    const { container } = render(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={onSelect}
        label="Modelo"
        allowFreeInput={false}
      />
    );

    const button = container.querySelector('button');
    expect(button).toHaveAttribute('aria-label', expect.stringContaining('Modelo'));
  });

  it('filtra items ao digitar', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={onSelect}
        placeholder="Filtrar..."
      />
    );

    const button = screen.getByRole('button');
    await user.click(button);

    const input = screen.getByRole('combobox');
    await user.type(input, 'gpt');

    // Deve mostrar apenas items que contêm "gpt"
    expect(screen.getByText('GPT-4')).toBeInTheDocument();
    expect(screen.getByText('GPT-3.5 Turbo')).toBeInTheDocument();
    expect(screen.queryByText('Claude 3')).not.toBeInTheDocument();
  });

  it('atualiza item destacado quando a seleção controlada muda com dropdown aberto', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    const { rerender } = render(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={onSelect}
        placeholder="Filtrar..."
      />
    );

    await user.click(screen.getByRole('button'));
    rerender(
      <Combobox
        items={mockItems}
        selected="claude-3"
        onSelect={onSelect}
        placeholder="Filtrar..."
      />
    );

    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'Claude 3' })).toHaveClass('highlighted');
    });
  });

  it('anuncia quando o filtro não retorna resultados', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={onSelect}
        placeholder="Filtrar..."
      />
    );

    await user.click(screen.getByRole('button'));
    await user.type(screen.getByRole('combobox'), 'sem-match');

    expect(screen.getByText('pickers.combobox.noResults')).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('pickers.combobox.noResults', 'polite');
  });

  it('não repete anúncio de entrada livre a cada tecla sem resultados', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(
      <Combobox
        items={[]}
        selected=""
        onSelect={onSelect}
        placeholder="Filtrar..."
        allowFreeInput={true}
      />
    );

    await user.click(screen.getByRole('button'));
    await waitFor(() => {
      expect(announceMock).toHaveBeenCalledWith('pickers.combobox.typeToCreate', 'polite');
    });
    announceMock.mockClear();

    await user.type(screen.getByRole('combobox'), 'abc');

    expect(screen.getByText('pickers.combobox.pressEnterToUse:abc')).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledTimes(1);
    expect(announceMock).toHaveBeenCalledWith('pickers.combobox.pressEnterToUse:a', 'polite');
  });

  it('fecha dropdown ao pressionar Escape', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    const { container } = render(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={onSelect}
        placeholder="Filtrar..."
      />
    );

    const button = screen.getByRole('button');
    await user.click(button);

    // Dropdown está aberto
    let dropdown = container.querySelector('.picker-dropdown');
    expect(dropdown).toBeInTheDocument();

    // Aguarda input ser focado (setTimeout de 10ms no open())
    await waitFor(() => {
      const input = container.querySelector('input');
      expect(input).toHaveFocus();
    });

    // Pressiona Escape no input focado
    await user.keyboard('{Escape}');

    // Dropdown é fechado
    await waitFor(() => {
      dropdown = container.querySelector('.picker-dropdown');
      expect(dropdown).not.toBeInTheDocument();
    });
  });

  it('expõe o atalho, anuncia uma vez, foca a busca e restaura o trigger no Escape', async () => {
    const onAnnounce = vi.fn();
    const user = userEvent.setup();
    const { container } = render(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={vi.fn()}
        label="Modelo"
        description="Selecionar modelo do chat ativo"
        shortcut="Ctrl+M"
        onAnnounce={onAnnounce}
      />,
    );

    const trigger = screen.getByRole('button', { name: 'Modelo, GPT-4' });
    expect(trigger).toHaveAttribute('title', 'Selecionar modelo do chat ativo (Ctrl+M)');
    await user.click(trigger);

    await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus());
    expect(onAnnounce).toHaveBeenCalledOnce();
    expect(onAnnounce).toHaveBeenCalledWith('GPT-4, 1 common.of 3');
    expect(await axe(container)).toHaveNoViolations();

    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Modelo, GPT-4' })).toHaveFocus());
  });

  it('desabilita entrada quando disabled=true', () => {
    const onSelect = vi.fn();

    render(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={onSelect}
        disabled={true}
      />
    );

    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
  });

  it('expõe busy sem desabilitar o trigger', () => {
    render(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={vi.fn()}
        busy
      />,
    );
    const button = screen.getByRole('button');
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button).not.toBeDisabled();
  });

  it('aguarda open=true controlado antes de focar a busca', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    const triggerRef = { current: null as HTMLButtonElement | null };
    const { rerender } = render(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={vi.fn()}
        open={false}
        onOpenChange={onOpenChange}
        triggerRef={triggerRef}
        triggerAriaLabel="Abrir seletor de comando"
      />,
    );
    const trigger = screen.getByRole('button', { name: 'Abrir seletor de comando' });
    expect(triggerRef.current).toBe(trigger);
    await user.click(trigger);
    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();

    rerender(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={vi.fn()}
        open
        onOpenChange={onOpenChange}
        triggerRef={triggerRef}
        triggerAriaLabel="Abrir seletor de comando"
      />,
    );
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus());
  });

  it('notifica dismiss controlado somente depois de open virar false', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    const onAfterDismiss = vi.fn();
    const triggerRef = { current: null as HTMLButtonElement | null };
    const { rerender } = render(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={vi.fn()}
        open
        onOpenChange={onOpenChange}
        onAfterDismiss={onAfterDismiss}
        triggerRef={triggerRef}
      />,
    );
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus());
    await user.keyboard('{Escape}');
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onAfterDismiss).not.toHaveBeenCalled();
    expect(screen.getByRole('combobox')).toBeInTheDocument();

    rerender(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={vi.fn()}
        open={false}
        onOpenChange={onOpenChange}
        onAfterDismiss={onAfterDismiss}
        triggerRef={triggerRef}
      />,
    );
    await waitFor(() => expect(onAfterDismiss).toHaveBeenCalledOnce());
    expect(triggerRef.current).not.toHaveFocus();
  });

  it('notifica seleção controlada somente depois do fechamento externo', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    const onAfterSelect = vi.fn();
    const onSelect = vi.fn();
    const { rerender } = render(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={onSelect}
        open
        onOpenChange={onOpenChange}
        onAfterSelect={onAfterSelect}
      />,
    );
    await user.click(screen.getByRole('option', { name: 'GPT-4' }));
    expect(onSelect).toHaveBeenCalledWith('gpt-4', expect.objectContaining({ value: 'gpt-4' }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onAfterSelect).not.toHaveBeenCalled();
    expect(screen.getByRole('combobox')).toBeInTheDocument();

    rerender(
      <Combobox
        items={mockItems}
        selected="gpt-4"
        onSelect={onSelect}
        open={false}
        onOpenChange={onOpenChange}
        onAfterSelect={onAfterSelect}
      />,
    );
    await waitFor(() => expect(onAfterSelect).toHaveBeenCalledOnce());
  });

  it('reseta filtro ao reabrir externamente sem refocar em rerender aberto', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    const { rerender } = render(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={vi.fn()}
        open
        onOpenChange={onOpenChange}
      />,
    );
    const input = screen.getByRole('combobox');
    await user.type(input, 'gpt');
    expect(input).toHaveValue('gpt');
    const outside = document.createElement('button');
    document.body.append(outside);
    outside.focus();
    rerender(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={vi.fn()}
        open
        onOpenChange={onOpenChange}
      />,
    );
    expect(input).toHaveValue('gpt');
    expect(document.activeElement).toBe(outside);

    rerender(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={vi.fn()}
        open={false}
        onOpenChange={onOpenChange}
      />,
    );
    rerender(
      <Combobox
        items={mockItems}
        selected=""
        onSelect={vi.fn()}
        open
        onOpenChange={onOpenChange}
      />,
    );
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveValue(''));
  });
});

describe('Combobox - ações da opção destacada', () => {
  const actions = () => <>
    <button type="button">Favoritar</button>
    <button type="button">Configurar</button>
  </>;

  it('preserva o picker quando outro componente muda o foco programaticamente', async () => {
    const onSelect = vi.fn();
    const onAfterDismiss = vi.fn();
    const user = userEvent.setup();
    render(<><Combobox items={mockItems} selected="" onSelect={onSelect} label="Picker" renderActiveItemActions={actions} onAfterDismiss={onAfterDismiss} /><button>Outro alvo</button></>);
    await user.click(screen.getByRole('button', { name: /Picker/ }));
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus());
    act(() => screen.getByRole('button', { name: 'Outro alvo' }).focus());
    expect(screen.getByRole('combobox')).toBeInTheDocument();
    expect(onAfterDismiss).not.toHaveBeenCalled();
    await user.click(screen.getByRole('option', { name: 'GPT-4' }));
    expect(onSelect).toHaveBeenCalledExactlyOnceWith('gpt-4', expect.objectContaining({ value: 'gpt-4' }));
  });

  it('mantém a seleção por clique na opção ao renderizar ações fora do listbox', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(<Combobox items={mockItems} selected="" onSelect={onSelect} label="Picker" renderActiveItemActions={actions} />);
    await user.click(screen.getByRole('button', { name: /Picker/ }));
    await user.click(screen.getByRole('option', { name: 'GPT-4' }));
    expect(onSelect).toHaveBeenCalledWith('gpt-4', expect.objectContaining({ value: 'gpt-4' }));
    await waitFor(() => expect(screen.getByRole('button', { name: /Picker/ })).toHaveFocus());
  });

  it('permite Tab pelas ações e fecha sem roubar o foco ao sair delas', async () => {
    const user = userEvent.setup();
    const onAfterDismiss = vi.fn();
    render(<><Combobox items={mockItems} selected="" onSelect={vi.fn()} label="Picker" renderActiveItemActions={actions} onAfterDismiss={onAfterDismiss} /><button>Depois</button></>);
    await user.click(screen.getByRole('button', { name: /Picker/ }));
    const input = screen.getByRole('combobox');
    await waitFor(() => expect(input).toHaveFocus());
    await user.tab();
    expect(screen.getByRole('button', { name: 'Favoritar' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Configurar' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Depois' })).toHaveFocus();
    await waitFor(() => expect(screen.queryByRole('combobox')).not.toBeInTheDocument());
    await waitFor(() => expect(onAfterDismiss).toHaveBeenCalledExactlyOnceWith('focus-leave'));
  });

  it('Escape nas ações fecha e restaura foco ao acionador', async () => {
    const user = userEvent.setup();
    render(<Combobox items={mockItems} selected="" onSelect={vi.fn()} label="Picker" renderActiveItemActions={actions} />);
    const trigger = screen.getByRole('button', { name: /Picker/ });
    await user.click(trigger);
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus());
    await user.tab();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.getByRole('button', { name: /Picker/ })).toHaveFocus());
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  });
});
