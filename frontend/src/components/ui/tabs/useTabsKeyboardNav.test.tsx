import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { useRef } from 'react';
import { useTabsKeyboardNav } from './useTabsKeyboardNav';

vi.mock('../../../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: vi.fn(() => true),
}));

import { restoreDefaultFocus } from '../../../hooks/useDefaultFocus';

function TabsNavFixture({
  activationMode = 'auto',
  onValueChange,
  onDelete,
  onActivate,
}: {
  activationMode?: 'auto' | 'manual';
  onValueChange?: (value: string) => void;
  onDelete?: (value: string) => void;
  onActivate?: () => boolean;
}) {
  const listRef = useRef<HTMLDivElement>(null);
  const { onKeyDown } = useTabsKeyboardNav({
    tabListRef: listRef,
    activationMode,
    onValueChange,
    onDelete,
    onActivate,
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

  it('Enter restaura foco na default area', () => {
    vi.mocked(restoreDefaultFocus).mockClear();

    render(<TabsNavFixture />);

    const tabA = screen.getByRole('tab', { name: 'A' });
    tabA.focus();
    fireEvent.keyDown(tabA, { key: 'Enter' });

    expect(restoreDefaultFocus).toHaveBeenCalled();
  });

  it('Enter suprimido quando onActivate retorna true', () => {
    vi.mocked(restoreDefaultFocus).mockClear();
    const onActivate = vi.fn(() => true);

    render(<TabsNavFixture onActivate={onActivate} />);

    const tabA = screen.getByRole('tab', { name: 'A' });
    tabA.focus();
    fireEvent.keyDown(tabA, { key: 'Enter' });

    expect(onActivate).toHaveBeenCalled();
    expect(restoreDefaultFocus).not.toHaveBeenCalled();
  });

  it('Ctrl+W fecha aba', () => {
    const onDelete = vi.fn();

    render(<TabsNavFixture onDelete={onDelete} />);

    const tabA = screen.getByRole('tab', { name: 'A' });
    tabA.focus();
    fireEvent.keyDown(tabA, { key: 'w', ctrlKey: true });

    expect(onDelete).toHaveBeenCalledWith('a');
  });

  it('Ctrl+F4 fecha aba', () => {
    const onDelete = vi.fn();

    render(<TabsNavFixture onDelete={onDelete} />);

    const tabA = screen.getByRole('tab', { name: 'A' });
    tabA.focus();
    fireEvent.keyDown(tabA, { key: 'F4', ctrlKey: true });

    expect(onDelete).toHaveBeenCalledWith('a');
  });

  it('Delete NÃO chama restoreDefaultFocus', () => {
    vi.mocked(restoreDefaultFocus).mockClear();
    const onDelete = vi.fn();

    render(<TabsNavFixture onDelete={onDelete} />);

    const tabA = screen.getByRole('tab', { name: 'A' });
    tabA.focus();
    fireEvent.keyDown(tabA, { key: 'Delete' });

    expect(onDelete).toHaveBeenCalled();
    expect(restoreDefaultFocus).not.toHaveBeenCalled();
  });
});
