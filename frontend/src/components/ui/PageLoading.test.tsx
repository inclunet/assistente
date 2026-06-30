import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PageLoading } from './PageLoading';

const announceMock = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

describe('PageLoading', () => {
  beforeEach(() => {
    announceMock.mockReset();
  });

  it('anuncia carregamento pelo broker global sem live region local', () => {
    render(<PageLoading message="Carregando dados..." />);

    expect(screen.getByText('Carregando dados...')).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('Carregando dados...');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
