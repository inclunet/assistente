import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import PluginsPage from './PluginsPage';

const mocks = vi.hoisted(() => ({ list: vi.fn(), components: vi.fn(), toggle: vi.fn(), preview: vi.fn(), install: vi.fn(), discard: vi.fn(), announce: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({ InstalledComponents: mocks.components, SetSkillActive: mocks.toggle, DiscardPreview: mocks.discard, ListInstalled: mocks.list, PreviewLocal: mocks.preview, InstallReviewed: mocks.install }));
vi.mock('../store/authStore', () => ({ useAuthStore: () => 'session' }));
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mocks.announce }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

describe('PluginsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.list.mockResolvedValue([]);
    mocks.preview.mockResolvedValue({ token: 'reviewed', name: 'demo', origin: 'local', digest: '123', files: ['plugin.json'], diagnostics: [] });
    mocks.install.mockResolvedValue({});
    mocks.discard.mockResolvedValue(undefined);
  });

  it('opens a preview from the keyboard and installs only after confirmation', async () => {
    const user = userEvent.setup();
    const { container } = render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    expect(await axe(container)).toHaveNoViolations();
    choose.focus();
    await user.keyboard('{Enter}');
    const install = await screen.findByRole('button', { name: 'plugins.install' });
    const cancel = screen.getByRole('button', { name: 'common.cancel' });
    expect(install.compareDocumentPosition(cancel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(mocks.install).not.toHaveBeenCalled();
    install.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(mocks.install).toHaveBeenCalledWith('reviewed'));
    expect(mocks.announce).toHaveBeenCalled();
  });

  it('keeps the preview open during refresh and restores focus after publication', async () => {
    const user = userEvent.setup();
    let finish!: () => void;
    mocks.list.mockResolvedValueOnce([]).mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve([]); }));
    mocks.preview.mockResolvedValueOnce({ token: 'revision', name: 'demo', digest: 'new', previousDigest: 'old', files: [], diagnostics: [] });
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    expect(await screen.findByText('old')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'plugins.update' }));
    expect(choose).toBeDisabled();
    expect(screen.getByRole('button', { name: 'plugins.update' })).toBeDisabled();
    finish();
    await waitFor(() => expect(choose).toHaveFocus());
  });

  it('disables publication of the installed revision', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({ token: 'revision', name: 'demo', digest: 'same', previousDigest: 'same', files: [], diagnostics: [] });
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    expect(await screen.findByText('plugins.current')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'plugins.update' })).toBeDisabled();
  });

  it('shows discard errors inside the modal and allows retry', async () => {
    const user = userEvent.setup();
    mocks.discard.mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    const cancel = await screen.findByRole('button', { name: 'common.cancel' });
    await user.click(cancel);
    expect(screen.getByRole('dialog')).toContainElement(await screen.findByText('plugins.discardError'));
    expect(mocks.announce).toHaveBeenCalledWith('plugins.discardError', 'assertive');
    await user.click(cancel);
    await waitFor(() => expect(choose).toHaveFocus());
    expect(mocks.install).not.toHaveBeenCalled();
  });

  it('opens installed components with Enter and applies the selected revision', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', origin: 'folder', version: 'v1', digest: 'sha' }]);
    const note = { name: 'note', key: 'skills/note', kind: 'skill', active: false };
    mocks.components.mockResolvedValueOnce([note]).mockResolvedValueOnce([{ ...note, active: true, slug: 'note-local' }]);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.activate' }));
    expect(mocks.toggle).toHaveBeenCalledWith('one', 'sha', 'skills/note', true);
    expect(await screen.findByText('/note-local')).toBeInTheDocument();
  });

  it('closes the preview without installing', async () => {
    const user = userEvent.setup();
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(await screen.findByRole('button', { name: 'common.cancel' }));
    expect(mocks.install).not.toHaveBeenCalled();
    expect(mocks.discard).toHaveBeenCalledWith('reviewed');
    expect(choose).toHaveFocus();
    expect(screen.queryByRole('button', { name: 'plugins.install' })).toBeNull();
  });
});
