import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CheckboxGrid } from './CheckboxGrid';

interface TestItem {
  id: string;
  name: string;
  description: string;
  badge?: string;
}

const testItems: TestItem[] = [
  {
    id: 'item-1',
    name: 'Item 1',
    description: 'Description 1',
    badge: 'NEW',
  },
  {
    id: 'item-2',
    name: 'Item 2',
    description: 'Description 2',
  },
  {
    id: 'item-3',
    name: 'Item 3',
    description: 'Description 3',
  },
];

describe('CheckboxGrid', () => {
  it('renderiza lista de checkboxes', () => {
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        getItemDescription={(item) => item.description}
        ariaLabel="Test Grid"
      />
    );

    expect(screen.getByText('Item 1')).toBeInTheDocument();
    expect(screen.getByText('Item 2')).toBeInTheDocument();
    expect(screen.getByText('Item 3')).toBeInTheDocument();
  });

  it('renderiza descrições quando fornecidas', () => {
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        getItemDescription={(item) => item.description}
        ariaLabel="Test Grid"
      />
    );

    expect(screen.getAllByText('Description 1')[0]).toBeInTheDocument();
    expect(screen.getAllByText('Description 2')[0]).toBeInTheDocument();
  });

  it('renderiza badges quando fornecidos', () => {
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        getItemDescription={(item) => item.description}
        getBadge={(item) => item.badge || ''}
        ariaLabel="Test Grid"
      />
    );

    expect(screen.getByTestId('badge-item-1')).toHaveTextContent('NEW');
  });

  it('marca checkboxes selecionados', () => {
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={['item-1', 'item-3']}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        ariaLabel="Test Grid"
      />
    );

    const checkbox1 = screen.getByTestId('checkbox-item-1') as HTMLInputElement;
    const checkbox2 = screen.getByTestId('checkbox-item-2') as HTMLInputElement;
    const checkbox3 = screen.getByTestId('checkbox-item-3') as HTMLInputElement;

    expect(checkbox1.checked).toBe(true);
    expect(checkbox2.checked).toBe(false);
    expect(checkbox3.checked).toBe(true);
  });

  it('chama onToggle ao clicar em checkbox', async () => {
    const user = userEvent.setup();
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        ariaLabel="Test Grid"
      />
    );

    const checkbox1 = screen.getByTestId('checkbox-item-1');
    await user.click(checkbox1);

    expect(handleToggle).toHaveBeenCalledWith('item-1');
  });

  it('renderiza estado vazio', () => {
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={[]}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item: TestItem) => item.id}
        getItemLabel={(item: TestItem) => item.name}
        ariaLabel="Test Grid"
      />
    );

    expect(screen.getByTestId('empty-state')).toBeInTheDocument();
  });

  it('navega com setas do teclado', async () => {
    const user = userEvent.setup();
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        ariaLabel="Test Grid"
      />
    );

    const checkbox1 = screen.getByTestId('checkbox-item-1') as HTMLInputElement;
    const checkbox2 = screen.getByTestId('checkbox-item-2') as HTMLInputElement;

    checkbox1.focus();
    expect(checkbox1).toHaveFocus();

    await user.keyboard('{ArrowDown}');
    expect(checkbox2).toHaveFocus();
  });

  it('usa Home para ir ao primeiro item', async () => {
    const user = userEvent.setup();
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        ariaLabel="Test Grid"
      />
    );

    const checkbox1 = screen.getByTestId('checkbox-item-1') as HTMLInputElement;
    const checkbox3 = screen.getByTestId('checkbox-item-3') as HTMLInputElement;

    checkbox3.focus();
    await user.keyboard('{Home}');
    expect(checkbox1).toHaveFocus();
  });

  it('usa End para ir ao último item', async () => {
    const user = userEvent.setup();
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        ariaLabel="Test Grid"
      />
    );

    const checkbox1 = screen.getByTestId('checkbox-item-1') as HTMLInputElement;
    const checkbox3 = screen.getByTestId('checkbox-item-3') as HTMLInputElement;

    checkbox1.focus();
    await user.keyboard('{End}');
    expect(checkbox3).toHaveFocus();
  });

  it('usa Space para toggle checkbox', async () => {
    const user = userEvent.setup();
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        ariaLabel="Test Grid"
      />
    );

    const checkbox1 = screen.getByTestId('checkbox-item-1');
    checkbox1.focus();

    await user.keyboard(' ');
    expect(handleToggle).toHaveBeenCalledWith('item-1');
  });

  it('possui aria-label no container', () => {
    const handleToggle = vi.fn();
    render(
      <CheckboxGrid
        items={testItems}
        selectedIds={[]}
        onToggle={handleToggle}
        getItemId={(item) => item.id}
        getItemLabel={(item) => item.name}
        ariaLabel="Test Grid Label"
      />
    );

    const grid = screen.getByTestId('checkbox-grid');
    expect(grid).toHaveAttribute('aria-label', 'Test Grid Label');
  });
});
