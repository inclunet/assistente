import { StrictMode } from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { WelcomeSetup } from './WelcomeSetup';

const mocks = vi.hoisted(() => ({ needs: vi.fn(), prepare: vi.fn(), toast: vi.fn(), t: (key: string) => key }));
vi.mock('@wailsjs/go/wailsapi/Welcome', () => ({ NeedsWelcomeWizard: mocks.needs, RunWelcomeWizard: mocks.prepare }));
vi.mock('../../lib/waitForWailsBridge', () => ({ waitForWailsBridge: () => Promise.resolve() }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (select: (state: unknown) => unknown) => select({ addToast: mocks.toast }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: mocks.t }) }));
vi.mock('../ui/Modal', () => ({ Modal: ({ isOpen, children, allowClose, onClose }: { isOpen: boolean; children: React.ReactNode; allowClose: boolean; onClose: () => void }) => isOpen ? <div role="dialog" aria-label="setup">
  <button disabled={!allowClose} onClick={onClose}>close</button>{children}
</div> : null }));
vi.mock('./ProviderForm', () => ({ ProviderForm: ({ onSave, onCancel, onBusyChange }: { onSave: () => void; onCancel: () => void; onBusyChange: (busy: boolean) => void }) => <>
  <p>shared provider form</p>
  <button onClick={onSave}>saved</button>
  <button onClick={onCancel}>cancel</button>
  <button onClick={() => onBusyChange(true)}>saving</button>
  <button onClick={() => onBusyChange(false)}>save failed</button>
</> }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.needs.mockReset().mockResolvedValue(true);
  mocks.prepare.mockReset().mockResolvedValue(true);
  mocks.t = (key: string) => key;
});

describe('welcome shared provider configuration', () => {
  it.each(['saved', 'cancel'])('does not reopen after %s when the language changes', async action => {
    const view = render(<WelcomeSetup />);
    await screen.findByRole('dialog');
    fireEvent.click(screen.getByText(action));
    mocks.t = (key: string) => `new-language:${key}`;
    view.rerender(<WelcomeSetup />);
    await act(async () => { await Promise.resolve(); });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(mocks.prepare).toHaveBeenCalledTimes(1);
    expect(mocks.toast).toHaveBeenCalledTimes(1);
  });

  it('prepares once in Strict Mode and only reports success after the shared form saves', async () => {
    render(<StrictMode><WelcomeSetup /></StrictMode>);
    await screen.findByText('shared provider form');
    expect(mocks.prepare).toHaveBeenCalledTimes(1);
    expect(mocks.toast).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText('saved'));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(mocks.toast).toHaveBeenCalledWith('app.wizard.success', 'success', 5000);
  });

  it('does not prepare an already configured installation', async () => {
    mocks.needs.mockResolvedValue(false);
    render(<WelcomeSetup />);
    await waitFor(() => expect(mocks.needs).toHaveBeenCalledOnce());
    expect(mocks.prepare).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('does not open provider configuration after cancelled vault preparation', async () => {
    mocks.prepare.mockResolvedValue(false);
    render(<WelcomeSetup />);
    await waitFor(() => expect(mocks.toast).toHaveBeenCalledWith('app.wizard.cancelled', 'warning'));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('preserves the form on failure and blocks closure while saving', async () => {
    render(<WelcomeSetup />);
    await screen.findByRole('dialog');
    fireEvent.click(screen.getByText('saving'));
    expect(screen.getByText('close')).toBeDisabled();
    fireEvent.click(screen.getByText('cancel'));
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.click(screen.getByText('save failed'));
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(mocks.toast).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText('cancel'));
    expect(mocks.toast).toHaveBeenCalledWith('app.wizard.cancelled', 'warning');
  });

  it('does not prepare the vault after logout during the initial check', async () => {
    const pending = deferred<boolean>();
    mocks.needs.mockReturnValue(pending.promise);
    const view = render(<WelcomeSetup />);
    await waitFor(() => expect(mocks.needs).toHaveBeenCalledOnce());
    view.unmount();
    await act(async () => pending.resolve(true));
    expect(mocks.prepare).not.toHaveBeenCalled();
    expect(mocks.toast).not.toHaveBeenCalled();
  });

  it('discards an old session preparation result after session replacement', async () => {
    const pending = deferred<boolean>();
    mocks.prepare.mockReturnValueOnce(pending.promise).mockResolvedValue(true);
    const view = render(<WelcomeSetup key="old-session" />);
    await waitFor(() => expect(mocks.prepare).toHaveBeenCalledOnce());
    mocks.needs.mockResolvedValue(false);
    view.rerender(<WelcomeSetup key="new-session" />);
    await act(async () => pending.resolve(true));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(mocks.toast).not.toHaveBeenCalled();
  });
});
