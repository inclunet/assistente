import { afterEach, describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
    expect(announceMock).toHaveBeenCalledWith('pickers.combobox.noResults', 'assertive');
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
});
