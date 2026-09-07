/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { ConnectionStatusIndicator } from './ConnectionStatusIndicator';
import { useConnectionStore } from '../../store/connectionStore';
import type { ConnectionStatusPayload } from '../../types/connection';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

function payload(overrides: Partial<ConnectionStatusPayload>): ConnectionStatusPayload {
  return {
    state: 'online',
    providerId: 'p1',
    providerName: 'Prov',
    model: '',
    latencyMs: 0,
    avgLatencyMs: 0,
    error: '',
    errorType: '',
    timestamp: 0,
    ...overrides,
  };
}

describe('ConnectionStatusIndicator', () => {
  beforeEach(() => {
    act(() => useConnectionStore.getState().reset());
  });

  it('não renderiza nada antes da primeira verificação', () => {
    const { container } = render(<ConnectionStatusIndicator />);
    expect(container.firstChild).toBeNull();
  });

  it('reflete o estado online com latência média', () => {
    act(() => useConnectionStore.getState().setStatus(payload({ state: 'online', avgLatencyMs: 123 })));
    render(<ConnectionStatusIndicator />);

    const el = screen.getByLabelText('connectionStatus.aria.onlineLatency');
    expect(el).toHaveAttribute('data-state', 'online');
    expect(el).toHaveTextContent('connectionStatus.online');
    expect(el).toHaveTextContent('connectionStatus.latency');
  });

  it('reflete o estado offline', () => {
    act(() => useConnectionStore.getState().setStatus(payload({ state: 'offline', avgLatencyMs: 0 })));
    render(<ConnectionStatusIndicator />);

    const el = screen.getByLabelText('connectionStatus.aria.offline');
    expect(el).toHaveAttribute('data-state', 'offline');
    expect(el).toHaveTextContent('connectionStatus.offline');
  });

  it('reflete o estado sem login com rótulo próprio', () => {
    act(() =>
      useConnectionStore
        .getState()
        .setStatus(payload({ state: 'unauthenticated', errorType: 'agent_not_authenticated' })),
    );
    render(<ConnectionStatusIndicator />);

    const el = screen.getByLabelText('connectionStatus.aria.unauthenticated');
    expect(el).toHaveAttribute('data-state', 'unauthenticated');
    expect(el).toHaveTextContent('connectionStatus.unauthenticated');
  });

  it('reflete o estado checking (reconectando)', () => {
    act(() => useConnectionStore.getState().setStatus(payload({ state: 'checking' })));
    render(<ConnectionStatusIndicator />);

    const el = screen.getByLabelText('connectionStatus.aria.checking');
    expect(el).toHaveAttribute('data-state', 'checking');
    expect(el).toHaveTextContent('connectionStatus.checking');
  });
});
