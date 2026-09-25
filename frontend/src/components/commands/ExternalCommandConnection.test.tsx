import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { axe } from '../../test/a11yAxe';
import {
  type ExternalUIConnectionService,
  type ExternalUIConnectionStatus,
  type ExternalUIInvitation,
  type ExternalUIOwnerProof,
  type ExternalUIDestination,
} from '../../services/externalUIConnection';
import { ExternalCommandConnection } from './ExternalCommandConnection';

const announce = vi.hoisted(() => vi.fn());
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));

const owner: ExternalUIOwnerProof = { userId: 'user-1', sessionId: 'session-1', workspaceId: 'workspace-1' };
const target: ExternalUIDestination = {
  workspaceId: owner.workspaceId,
  tabId: 'tab-1',
  surface: { surfaceType: 'editor', surfaceId: 'tab-1', snapshotVersion: 'editor:tab-1:1' },
};
const connected: ExternalUIConnectionStatus = {
  state: 'connected', owner, target,
  connectionId: '0190f7b2-7c00-7000-8000-000000000001',
  generation: '1',
  targetSnapshotId: '0190f7b2-7c00-7000-8000-000000000002',
  contextVersion: '0190f7b2-7c00-7000-8000-000000000003',
  expiresAt: '2099-01-01T00:00:00Z',
};

function createService(initial: ExternalUIConnectionStatus | null = null, invite?: ExternalUIInvitation) {
  let snapshot = initial;
  const listeners = new Set<() => void>();
  const emit = () => listeners.forEach(listener => listener());
  const service: ExternalUIConnectionService = {
    refresh: vi.fn(async () => snapshot),
    begin: vi.fn(async () => invite ?? { invitation: 'single-use-code', expiresAt: '2099-01-01T00:00:00Z' }),
    publishContext: vi.fn(async () => snapshot ?? connected),
    heartbeat: vi.fn(async () => snapshot ?? connected),
    disconnect: vi.fn(async () => {
      snapshot = { state: 'disconnected', owner, target: null };
      emit();
    }),
    take: vi.fn(async () => ({
      invocationId: '0190f7b2-7c00-7000-8000-000000000004', commandId: 'test.command', arguments: {},
      receiptId: '0190f7b2-7c00-7000-8000-000000000005', targetSnapshotId: connected.targetSnapshotId!,
      contextVersion: connected.contextVersion!, target,
    })),
    complete: vi.fn(async () => true),
    getSnapshot: () => snapshot,
    subscribe: listener => { listeners.add(listener); return () => listeners.delete(listener); },
    subscribeReady: () => () => undefined,
    dispose: vi.fn(),
  };
  return {
    service,
    setSnapshot(next: ExternalUIConnectionStatus | null) { snapshot = next; emit(); },
  };
}

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
  document.querySelectorAll('[data-testid="external-consent-target"]').forEach(element => element.remove());
});

describe('ExternalCommandConnection', () => {
  it('exige consentimento e um serviço/destino explícitos; estado desconectado é acessível', async () => {
    const { container } = render(<ExternalCommandConnection service={null} target={target} />);
    const consent = screen.getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' });
    const begin = screen.getByRole('button', { name: 'commandSettings.externalConnection.begin' });
    expect(consent).not.toBeChecked();
    expect(begin).toBeDisabled();
    expect(await axe(container)).toHaveNoViolations();
  });

  it('mantém o início bloqueado sem destino mesmo após marcar consentimento', async () => {
    const user = userEvent.setup();
    const fake = createService();
    render(<ExternalCommandConnection service={fake.service} target={null} />);
    const consent = screen.getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' });
    const begin = screen.getByRole('button', { name: 'commandSettings.externalConnection.begin' });

    await user.click(consent);
    expect(consent).toBeChecked();
    expect(begin).toBeDisabled();
    expect(fake.service.begin).not.toHaveBeenCalled();
  });

  it('porta somente o checkbox ao alvo, preserva aria-describedby e exige ação explícita após consentir', async () => {
    const user = userEvent.setup();
    const fake = createService();
    const portalTarget = document.createElement('div');
    portalTarget.dataset.testid = 'external-consent-target';
    document.body.appendChild(portalTarget);
    const view = render(<ExternalCommandConnection service={fake.service} target={target} />);
    const inlineConsent = screen.getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' });
    const descriptionId = inlineConsent.getAttribute('aria-describedby');
    expect(descriptionId).toBeTruthy();
    expect(document.getElementById(descriptionId!)!).toHaveTextContent('commandSettings.externalConnection.description');

    view.rerender(<ExternalCommandConnection service={fake.service} target={target} consentTarget={portalTarget} />);
    const consent = within(portalTarget).getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' });
    expect(consent).toHaveAttribute('aria-describedby', descriptionId);
    expect(view.container.querySelector('input[type="checkbox"]')).not.toBeInTheDocument();
    expect(within(portalTarget).queryByRole('button')).not.toBeInTheDocument();

    const begin = screen.getByRole('button', { name: 'commandSettings.externalConnection.begin' });
    expect(begin).toBeDisabled();
    await user.click(consent);
    expect(consent).toBeChecked();
    expect(fake.service.begin).not.toHaveBeenCalled();
    expect(screen.getByText('commandSettings.externalConnection.state.disconnected')).toBeInTheDocument();
    expect(begin).toBeEnabled();

    await user.click(begin);
    await waitFor(() => expect(fake.service.begin).toHaveBeenCalledExactlyOnceWith(target));
    expect(within(portalTarget).getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' })).not.toBeChecked();
    expect(begin).toBeDisabled();
  });

  it('permite marcar consentimento com Espaço quando o checkbox está portado', async () => {
    const user = userEvent.setup();
    const portalTarget = document.createElement('div');
    portalTarget.dataset.testid = 'external-consent-target';
    document.body.appendChild(portalTarget);
    render(<ExternalCommandConnection service={createService().service} target={target} consentTarget={portalTarget} />);
    const consent = within(portalTarget).getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' });

    consent.focus();
    await user.keyboard(' ');

    expect(consent).toBeChecked();
  });

  it('cria convite somente após consentimento e apresenta o código sem persistir', async () => {
    const user = userEvent.setup();
    const fake = createService(null);
    const { container } = render(<ExternalCommandConnection service={fake.service} target={target} />);
    await user.click(screen.getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' }));
    const begin = screen.getByRole('button', { name: 'commandSettings.externalConnection.begin' });
    expect(begin).toBeEnabled();
    await user.click(begin);
    expect(fake.service.begin).toHaveBeenCalledExactlyOnceWith(target);
    expect(screen.getByRole('textbox', { name: 'commandSettings.externalConnection.invitationLabel' })).toHaveValue('single-use-code');
    expect(screen.getByText('commandSettings.externalConnection.invitationCannotCancel')).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' })).not.toBeChecked();
    expect(begin).toBeDisabled();
    expect(await axe(container)).toHaveNoViolations();
  });

  it('expira e remove o convite temporário', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-09-24T12:00:00Z'));
    const fake = createService(null, {
      invitation: 'short-lived-code',
      expiresAt: new Date(Date.now() + 3_000).toISOString(),
    });
    render(<ExternalCommandConnection service={fake.service} target={target} />);
    fireEvent.click(screen.getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' }));
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'commandSettings.externalConnection.begin' }));
      await Promise.resolve();
    });
    expect(screen.getByRole('textbox', { name: 'commandSettings.externalConnection.invitationLabel' })).toHaveValue('short-lived-code');
    await act(async () => { vi.advanceTimersByTime(3_001); await Promise.resolve(); });
    expect(screen.queryByRole('textbox', { name: 'commandSettings.externalConnection.invitationLabel' })).not.toBeInTheDocument();
    expect(screen.getByText('commandSettings.externalConnection.expired')).toBeInTheDocument();
  });

  it('reflete waiting_claim e permite desconexão quando a conexão está ativa', async () => {
    const fake = createService({ ...connected, state: 'waiting_claim', connectionId: undefined, generation: undefined });
    const user = userEvent.setup();
    const { rerender } = render(<ExternalCommandConnection service={fake.service} target={target} />);
    expect(screen.getByText('commandSettings.externalConnection.state.waiting_claim')).toBeInTheDocument();
    act(() => fake.setSnapshot(connected));
    rerender(<ExternalCommandConnection service={fake.service} target={target} />);
    const disconnect = screen.getByRole('button', { name: 'commandSettings.externalConnection.disconnect' });
    await user.click(disconnect);
    await waitFor(() => expect(fake.service.disconnect).toHaveBeenCalledOnce());
    expect(screen.getByText('commandSettings.externalConnection.state.disconnected')).toBeInTheDocument();
  });
});
