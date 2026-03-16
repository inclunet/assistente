import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import type { MenuItem } from './types';
import { Menu } from './Menu';

const buildItems = (): MenuItem[] => [
  { id: 'item-1', label: 'Primeiro', action: vi.fn() },
  { id: 'item-2', label: 'Segundo', action: vi.fn() },
  { id: 'item-3', label: 'Terceiro', action: vi.fn() },
];

describe('Menu', () => {
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
});
