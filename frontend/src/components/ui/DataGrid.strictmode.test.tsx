import { StrictMode } from 'react';
import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DataGrid } from './DataGrid';
import type { GridFocusRequest } from './DataGrid';

const items = [{ id: 'first', name: 'First row' }];
const columns = [{ key: 'name', label: 'Name' }];

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('DataGrid under StrictMode', () => {
  it('accepts requestGridFocus after the effect replay and focuses the requested cell', () => {
    vi.useFakeTimers();
    let requestFocus: GridFocusRequest | undefined;

    render(
      <StrictMode>
        <DataGrid
          items={items}
          columns={columns}
          autoFocusOnMount={false}
          onGridReady={(request) => { requestFocus = request; }}
        />
      </StrictMode>,
    );

    expect(requestFocus).toBeDefined();
    act(() => {
      expect(requestFocus?.({ itemId: 'first', colIndex: 0 })).toBe(true);
      vi.runOnlyPendingTimers();
    });

    expect(screen.getByRole('gridcell')).toHaveFocus();
  });

  it('does not focus a cell later if unmounted with a focus request pending', () => {
    vi.useFakeTimers();
    let requestFocus: GridFocusRequest | undefined;
    const view = render(
      <StrictMode>
        <DataGrid
          items={items}
          columns={columns}
          autoFocusOnMount={false}
          onGridReady={(request) => { requestFocus = request; }}
        />
      </StrictMode>,
    );
    const cell = screen.getByRole('gridcell');
    const focus = vi.spyOn(cell, 'focus');

    act(() => {
      expect(requestFocus?.({ itemId: 'first', colIndex: 0 })).toBe(true);
    });
    view.unmount();

    act(() => {
      vi.runOnlyPendingTimers();
    });
    expect(focus).not.toHaveBeenCalled();
  });
});
