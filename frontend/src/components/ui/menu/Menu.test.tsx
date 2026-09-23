import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { MenuItem } from './types';
import { Menu } from './Menu';

const buildItems = (): MenuItem[] => [
  { id: 'item-1', label: 'Primeiro', action: vi.fn() },
  { id: 'item-2', label: 'Segundo', action: vi.fn() },
  { id: 'item-3', label: 'Terceiro', action: vi.fn() },
];

describe('Menu', () => {
  it('busca metadados sem mudar o nome acessível e executa somente o resultado escolhido', () => {
    const action = vi.fn();
    render(<Menu visible searchable items={[
      { id: 'settings', label: 'Abrir configurações', searchText: 'Preferências aparência ajustes', action },
      { id: 'help', label: 'Ajuda' },
    ]} />);
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '  AJUSTES  ' } });
    expect(screen.getByRole('menuitem', { name: 'Abrir configurações' })).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', { name: 'Ajuda' })).not.toBeInTheDocument();
    expect(action).not.toHaveBeenCalled();
    fireEvent.keyDown(screen.getByRole('textbox'), { key: 'ArrowDown' });
    fireEvent.keyDown(screen.getByRole('menu'), { key: 'Enter' });
    expect(action).toHaveBeenCalledTimes(1);
  });

  it('ArrowDown do campo de busca foca somente o primeiro resultado', async () => {
    const user = userEvent.setup();
    const firstAction = vi.fn();
    const secondAction = vi.fn();
    render(<Menu visible searchable items={[
      { id: 'first', label: 'Primeiro', action: firstAction },
      { id: 'second', label: 'Segundo', action: secondAction },
    ]} />);

    const input = screen.getByRole('textbox');
    await user.click(input);
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('menuitem', { name: 'Primeiro' })).toHaveFocus();
    expect(screen.getByRole('menuitem', { name: 'Segundo' })).not.toHaveFocus();
    expect(firstAction).not.toHaveBeenCalled();
    expect(secondAction).not.toHaveBeenCalled();
  });

  it('Enter no campo de busca executa o resultado uma única vez', async () => {
    const user = userEvent.setup();
    const action = vi.fn();
    render(<Menu visible searchable items={[
      { id: 'first-enter', label: 'Primeiro', action },
    ]} />);
    const input = screen.getByRole('textbox');
    await user.click(input);
    await user.keyboard('{Enter}');
    expect(action).toHaveBeenCalledTimes(1);
  });

  it('espaço no campo de busca não ativa o item focado', () => {
    const action = vi.fn();
    render(<Menu visible searchable items={[
      { id: 'first-space', label: 'Primeiro', action },
    ]} />);
    const input = screen.getByRole('textbox');
    fireEvent.keyDown(input, { key: ' ' });
    expect(action).not.toHaveBeenCalled();
    expect(screen.getByRole('menu')).toBeInTheDocument();
  });

  it('foca item inicial e executa acao ao pressionar Enter', () => {
    const items = buildItems();
    const onClose = vi.fn();
    const onSelect = vi.fn();

    render(
      <Menu
        visible={true}
        items={items}
        x={10}
        y={10}
        initialFocusItemId="item-2"
        onClose={onClose}
        onSelect={onSelect}
      />
    );

    const menu = screen.getByRole('menu');
    const focused = screen.getByRole('menuitem', { name: 'Segundo' });
    expect(focused).toHaveFocus();

    fireEvent.keyDown(menu, { key: 'Enter' });

    expect(items[1].action).toHaveBeenCalled();
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'item-2' }));
    expect(onClose).toHaveBeenCalled();
  });

  it.each([
    ['selection', (menu: HTMLElement) => fireEvent.keyDown(menu, { key: 'Enter' })],
    ['escape', (menu: HTMLElement) => fireEvent.keyDown(menu, { key: 'Escape' })],
  ])('não agenda restauração tardia com restoreFocusOnClose=false em %s', (_reason, closeMenu) => {
    const requestAnimationFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 1);
    const onClose = vi.fn();
    render(
      <Menu
        visible
        items={buildItems()}
        initialFocusItemId="item-1"
        restoreFocusOnClose={false}
        onClose={onClose}
      />,
    );

    closeMenu(screen.getByRole('menu'));

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(requestAnimationFrame).not.toHaveBeenCalled();
    requestAnimationFrame.mockRestore();
  });

  it('não agenda restauração tardia ao fechar por clique fora com restoreFocusOnClose=false', () => {
    const requestAnimationFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 1);
    const onClose = vi.fn();
    render(<Menu visible items={buildItems()} restoreFocusOnClose={false} onClose={onClose} />);

    fireEvent.mouseDown(document.body);

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(requestAnimationFrame).not.toHaveBeenCalled();
    requestAnimationFrame.mockRestore();
  });

  it('preserva restauração tardia por padrão', () => {
    const requestAnimationFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 1);
    render(
      <Menu
        visible
        items={buildItems()}
        initialFocusItemId="item-1"
      />,
    );

    fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' });

    expect(requestAnimationFrame).toHaveBeenCalledTimes(1);
    requestAnimationFrame.mockRestore();
  });

  it('abre submenu com seta direita e executa acao do submenu', () => {
    const submenuAction = vi.fn();
    const items: MenuItem[] = [
      {
        id: 'parent',
        label: 'Pai',
        submenu: [
          { id: 'child', label: 'Filho', action: submenuAction },
        ],
      },
      { id: 'other', label: 'Outro', action: vi.fn() },
    ];

    render(
      <Menu
        visible={true}
        items={items}
        x={10}
        y={10}
        initialFocusItemId="parent"
      />
    );

    const menu = screen.getByRole('menu');
    fireEvent.keyDown(menu, { key: 'ArrowRight' });

    const child = screen.getByRole('menuitem', { name: 'Filho' });
    expect(child).toBeInTheDocument();

    fireEvent.keyDown(menu, { key: 'Enter' });
    expect(submenuAction).toHaveBeenCalled();
  });

  it('navega pelas setas sem pular itens quando disabled fica intercalado', async () => {
    const user = userEvent.setup();
    render(<Menu visible items={[
      { id: 'first-enabled', label: 'Primeiro', action: vi.fn() },
      { id: 'disabled-middle', label: 'Indisponível', disabled: true, action: vi.fn() },
      { id: 'second-enabled', label: 'Segundo', action: vi.fn() },
      { id: 'third-enabled', label: 'Terceiro', action: vi.fn() },
    ]} />);

    const menu = screen.getByRole('menu');
    expect(screen.getByRole('menuitem', { name: 'Primeiro' })).toHaveFocus();
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('menuitem', { name: 'Segundo' })).toHaveFocus();
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('menuitem', { name: 'Terceiro' })).toHaveFocus();
    await user.keyboard('{ArrowUp}');
    expect(screen.getByRole('menuitem', { name: 'Segundo' })).toHaveFocus();
    expect(screen.getByRole('menuitem', { name: 'Indisponível' })).not.toHaveFocus();
    expect(menu).toBeInTheDocument();
  });

  it('mantém a navegação coerente com busca e disabled intercalado', async () => {
    const user = userEvent.setup();
    render(<Menu visible searchable items={[
      { id: 'disabled-match', label: 'Comando indisponível', disabled: true },
      { id: 'enabled-match-a', label: 'Comando alfa', action: vi.fn() },
      { id: 'disabled-other', label: 'Outro indisponível', disabled: true },
      { id: 'enabled-match-b', label: 'Comando beta', action: vi.fn() },
    ]} />);

    const input = screen.getByRole('textbox');
    await user.type(input, 'comando');
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('menuitem', { name: 'Comando alfa' })).toHaveFocus();
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('menuitem', { name: 'Comando beta' })).toHaveFocus();
    expect(screen.getByRole('menuitem', { name: 'Comando indisponível' })).not.toHaveFocus();
  });

  it('não deixa hover disabled fazer a seta pular um item enabled', async () => {
    const user = userEvent.setup();
    render(<Menu visible items={[
      { id: 'mouse-a', label: 'A', action: vi.fn() },
      { id: 'mouse-b', label: 'B', action: vi.fn() },
      { id: 'mouse-c', label: 'C indisponível', disabled: true },
      { id: 'mouse-d', label: 'D', action: vi.fn() },
    ]} />);
    const first = screen.getByRole('menuitem', { name: 'A' });
    const disabled = screen.getByRole('menuitem', { name: 'C indisponível' });
    await user.hover(disabled);
    expect(first).toHaveFocus();
    expect(disabled).not.toHaveClass('context-menu__item--focused');
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('menuitem', { name: 'B' })).toHaveFocus();
  });

});
