import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { ChatTimelineRecovery } from './ChatTimelineRecovery';
import { useChatTimelineRecoveryStore } from '../../store/chatTimelineRecoveryStore';
import type { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';
import { requestConfirm } from '../../store/confirmStore';
import { Toolbar, ToolbarButton } from '../ui/Toolbar';
import { registerDefaultFocus, unregisterDefaultFocus } from '../../hooks/useDefaultFocus';
vi.mock('react-i18next', async importOriginal => ({ ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en', resolvedLanguage: 'en' } }) }));
vi.mock('../../hooks/useAnnouncer', async importOriginal => ({ ...await importOriginal<typeof import('../../hooks/useAnnouncer')>(), announce: vi.fn() }));
vi.mock('../../store/confirmStore', () => ({ requestConfirm: vi.fn() }));
const descriptor = { surfaceId: 'surface', windowId: 'window', identity: { itemId: 'item', epoch: 'epoch', revision: '1',
  reference: { projectionId: 'projection', incarnation: 'incarnation', turnId: 'turn', executionId: 'execution' } } };
function fixture() {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false,
    recoveredItems: [descriptor], subscribe: () => () => {}, getRevision: () => 1,
    close: vi.fn().mockResolvedValue(undefined), client: {
      readRecoveredItem: vi.fn().mockResolvedValue({ segments: [{ segmentId: 'text', kind: 'text', text: 'Conteúdo preservado' }] }),
      releaseRecoveredItem: vi.fn().mockResolvedValue(undefined),
    } };
  const typed = presentation as unknown as ChatTimelinePresentation;
  useChatTimelineRecoveryStore.getState().hold(typed);
  render(<MemoryRouter><ChatTimelineRecovery presentation={typed} closing /></MemoryRouter>);
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoveryOpen/ }));
  return { presentation, typed };
}
beforeEach(() => { vi.mocked(requestConfirm).mockReset(); useChatTimelineRecoveryStore.setState({ pending: [] }); });
afterEach(cleanup);
it('cancelar a liberação mantém a cópia lida e o fechamento pendente', async () => {
  const { presentation, typed } = fixture();
  fireEvent.click(screen.getByText('chat.timelineRecoveryRead'));
  await screen.findByText('Conteúdo preservado');
  vi.mocked(requestConfirm).mockResolvedValue(false);
  fireEvent.click(screen.getByText('chat.timelineRecoveryRelease'));
  await waitFor(() => expect(requestConfirm).toHaveBeenCalledOnce());
  expect(presentation.client.releaseRecoveredItem).not.toHaveBeenCalled();
  expect(presentation.close).not.toHaveBeenCalled();
  expect(screen.getByText('Conteúdo preservado')).toBeVisible();
  expect(useChatTimelineRecoveryStore.getState().pending).toContain(typed);
});

it('expõe somente sincronização na toolbar, navegável com setas e sem abrir diálogo', async () => {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [], hasProtectedItems: false,
    error: new Error('sync'), subscribe: () => () => {}, getRevision: () => 1, refresh: vi.fn().mockResolvedValue(undefined) };
  render(<MemoryRouter><Toolbar ariaLabel="chat" right={<><ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} /><ToolbarButton label="limpar conversa" /></>} /></MemoryRouter>);
  const sync = screen.getByRole('button', { name: 'chat.timelineRefresh' });
  expect(sync.closest('[role="toolbar"]')).not.toBeNull();
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  sync.focus();
  fireEvent.keyDown(sync, { key: 'ArrowRight' });
  expect(screen.getByRole('button', { name: 'limpar conversa' })).toHaveFocus();
  fireEvent.click(sync);
  await waitFor(() => expect(presentation.refresh).toHaveBeenCalledOnce());
});

it('restaura o foco quando a sincronização remove o botão focado', async () => {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [], hasProtectedItems: false,
    error: new Error('sync') as Error | undefined, subscribe: () => () => {}, getRevision: () => 1, refresh: vi.fn().mockResolvedValue(undefined) };
  const fallback = document.createElement('textarea'); document.body.appendChild(fallback);
  const restore = () => { fallback.focus(); return true; };
  registerDefaultFocus(restore);
  try {
    const view = () => <MemoryRouter><Toolbar right={<ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} />} /></MemoryRouter>;
    const { rerender } = render(view());
    screen.getByRole('button', { name: 'chat.timelineRefresh' }).focus();
    presentation.error = undefined;
    rerender(view());
    await waitFor(() => expect(fallback).toHaveFocus());
  } finally { unregisterDefaultFocus(restore); fallback.remove(); }
});

it('abre diálogo proativamente com conteúdo recuperado e permite adiar sem liberá-lo', async () => {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [descriptor], hasProtectedItems: true,
    subscribe: () => () => {}, getRevision: () => 1, client: { releaseRecoveredItem: vi.fn() } };
  render(<MemoryRouter><Toolbar right={<ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} />} /></MemoryRouter>);
  expect(await screen.findByRole('alertdialog')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoveryLater/ }));
  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
  expect(presentation.client.releaseRecoveredItem).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'chat.timelineRefresh' }));
  expect(await screen.findByRole('alertdialog')).toBeVisible();
});

it('espera a superfície ficar ativa antes de abrir o diálogo grave', async () => {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [descriptor], hasProtectedItems: true,
    subscribe: () => () => {}, getRevision: () => 1, client: {} } as unknown as ChatTimelinePresentation;
  const { rerender } = render(<MemoryRouter><ChatTimelineRecovery presentation={presentation} active={false} /></MemoryRouter>);
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  rerender(<MemoryRouter><ChatTimelineRecovery presentation={presentation} active /></MemoryRouter>);
  expect(await screen.findByRole('alertdialog')).toBeVisible();
});

it('apresenta uma decisão por vez quando duas superfícies precisam de recuperação', async () => {
  const make = (surfaceId: string) => ({ surfaceId, conversationId: surfaceId, loading: false, recoveredItems: [descriptor], hasProtectedItems: true,
    subscribe: () => () => {}, getRevision: () => 1, client: {} } as unknown as ChatTimelinePresentation);
  render(<MemoryRouter><Toolbar right={<><ChatTimelineRecovery presentation={make('a')} /><ChatTimelineRecovery presentation={make('b')} /></>} /></MemoryRouter>);
  expect(await screen.findAllByRole('alertdialog')).toHaveLength(1);
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoveryLater/ }));
  await waitFor(() => expect(screen.getAllByRole('alertdialog')).toHaveLength(1));
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoveryLater/ }));
  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
});

it('oferece recuperar no fim após falha de sincronização', async () => {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [], hasProtectedItems: false,
    error: new Error('cursor'), subscribe: () => () => {}, getRevision: () => 1,
    refresh: vi.fn().mockRejectedValue(new Error('cursor')), recoverAtEnd: vi.fn().mockResolvedValue(undefined) };
  render(<MemoryRouter><Toolbar right={<ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} />} /></MemoryRouter>);
  fireEvent.click(screen.getByRole('button', { name: 'chat.timelineRefresh' }));
  expect(await screen.findByRole('alertdialog')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoverAtEnd/ }));
  await waitFor(() => expect(presentation.recoverAtEnd).toHaveBeenCalledOnce());
});

it('não reabre a decisão enquanto fechamento pendente está em andamento e permite retry após erro', async () => {
  let reject!: (error: Error) => void;
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [],
    subscribe: () => () => {}, getRevision: () => 1, close: vi.fn(() => new Promise<void>((_, fail) => { reject = fail; })) };
  render(<MemoryRouter><ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} closing /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('button', { name: /chat.timelineRecoveryRetryClose/ }));
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  expect(presentation.close).toHaveBeenCalledTimes(1);
  await act(async () => reject(new Error('offline')));
  expect(await screen.findByRole('alertdialog')).toHaveTextContent('chat.timelineRecoveryError');
});
it('falha conserva conteúdo para nova tentativa; só fecha após liberação confirmada', async () => {
  const { presentation, typed } = fixture();
  fireEvent.click(screen.getByText('chat.timelineRecoveryRead'));
  await screen.findByText('Conteúdo preservado');
  vi.mocked(requestConfirm).mockResolvedValue(true);
  presentation.client.releaseRecoveredItem.mockRejectedValueOnce(new Error('offline'));
  fireEvent.click(screen.getByText('chat.timelineRecoveryRelease'));
  await screen.findByText('chat.timelineRecoveryError');
  expect(screen.getByText('Conteúdo preservado')).toBeVisible();
  expect(presentation.close).not.toHaveBeenCalled();
  let finish!: () => void;
  presentation.client.releaseRecoveredItem.mockImplementationOnce(() => new Promise<void>(resolve => {
    finish = () => { presentation.recoveredItems = []; resolve(); };
  }));
  fireEvent.click(screen.getByText('chat.timelineRecoveryRelease'));
  await waitFor(() => expect(presentation.client.releaseRecoveredItem).toHaveBeenCalledTimes(2));
  expect(presentation.close).not.toHaveBeenCalled();
  expect(useChatTimelineRecoveryStore.getState().pending).toContain(typed);
  await act(async () => finish());
  await waitFor(() => expect(presentation.close).toHaveBeenCalledOnce());
  expect(useChatTimelineRecoveryStore.getState().pending).not.toContain(typed);
});
