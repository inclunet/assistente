import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { useRef } from 'react';
import { useTabsKeyboardNav } from './useTabsKeyboardNav';

function TabsNavFixture({
  activationMode = 'auto',
  onValueChange,
  onDelete,
}: {
  activationMode?: 'auto' | 'manual';
  onValueChange?: (value: string) => void;
  onDelete?: (value: string) => void;
}) {
  const listRef = useRef<HTMLDivElement>(null);
  const { onKeyDown } = useTabsKeyboardNav({
    tabListRef: listRef,
    activationMode,
    onValueChange,
    onDelete,
  });

  return (
    <div ref={listRef} onKeyDown={onKeyDown} role="tablist">
      <button role="tab" data-tab-value="a" aria-selected="true">A</button>
      <button role="tab" data-tab-value="b">B</button>
    </div>
  );
}

describe('useTabsKeyboardNav', () => {
  it('altera valor no modo auto', () => {
    const onValueChange = vi.fn();

    render(<TabsNavFixture onValueChange={onValueChange} />);

    const tabA = screen.getByRole('tab', { name: 'A' });
    fireEvent.keyDown(tabA, { key: 'ArrowRight' });

    expect(onValueChange).toHaveBeenCalledWith('b');
  });

  it('fecha aba ao pressionar Delete', () => {
    const onDelete = vi.fn();

    render(<TabsNavFixture onDelete={onDelete} />);

    const tabA = screen.getByRole('tab', { name: 'A' });
    fireEvent.keyDown(tabA, { key: 'Delete' });

    expect(onDelete).toHaveBeenCalledWith('a');
  });

  it('no modo manual apenas move foco', () => {
    const onValueChange = vi.fn();

    render(<TabsNavFixture activationMode="manual" onValueChange={onValueChange} />);

    const tabA = screen.getByRole('tab', { name: 'A' });
    fireEvent.keyDown(tabA, { key: 'ArrowRight' });

    expect(onValueChange).not.toHaveBeenCalled();
    expect(screen.getByRole('tab', { name: 'B' })).toHaveFocus();
  });
});
