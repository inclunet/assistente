import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import type { MenuItem } from './types';
import { ContextMenu } from './ContextMenu';

describe('ContextMenu', () => {
  it('usa aria-label padrao quando nao informado', () => {
    const items: MenuItem[] = [{ id: 'a', label: 'Acao' }];

    render(
      <ContextMenu
        visible={true}
        items={items}
        x={0}
        y={0}
      />
    );

    expect(screen.getByRole('menu', { name: 'Menu de contexto' })).toBeInTheDocument();
  });
});
