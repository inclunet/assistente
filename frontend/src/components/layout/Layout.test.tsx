import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Layout } from './Layout';

vi.mock('./Topbar', () => ({
  Topbar: () => <div data-testid="topbar" />,
}));

vi.mock('../../hooks/useDocumentTitle', () => ({
  useDocumentTitle: vi.fn(),
}));

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return {
    ...actual,
    Outlet: () => <div data-testid="outlet" />,
  };
});

describe('Layout', () => {
  it('renderiza topbar e outlet', () => {
    render(<Layout />);

    expect(screen.getByTestId('topbar')).toBeInTheDocument();
    expect(screen.getByTestId('outlet')).toBeInTheDocument();
  });
});
