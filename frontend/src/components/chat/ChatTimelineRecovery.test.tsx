import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { ChatTimelineRecovery, ChatTimelineRecoveryHost } from './ChatTimelineRecovery';
import { useChatTimelineRecoveryStore } from '../../store/chatTimelineRecoveryStore';
import type { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';
import { requestConfirm } from '../../store/confirmStore';
import { Toolbar, ToolbarButton } from '../ui/Toolbar';
import { registerDefaultFocus, unregisterDefaultFocus } from '../../hooks/useDefaultFocus';
import { Modal } from '../ui/Modal';
import { createRef } from 'react';
import { ChatTimelineRecoveryPendingButton } from './ChatTimelineRecoveryPendingButton';
import { clearRecoveryOrigin } from '../../store/chatTimelineRecoveryDialogStore';
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
  render(<MemoryRouter><ChatTimelineRecoveryHost /></MemoryRouter>);
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoveryOpen/ }));
  return { presentation, typed };
}
beforeEach(() => { vi.mocked(requestConfirm).mockReset(); useChatTimelineRecoveryStore.setState({ pending: [], deferred: [] }); clearRecoveryOrigin(); });
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

it('preserva o retorno de foco quando o botão focado é desabilitado durante sincronização', async () => {
  const fallback = document.createElement('textarea'); document.body.appendChild(fallback);
  const restore = () => { fallback.focus(); return true; }; registerDefaultFocus(restore);
  let finish!: () => void;
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [], hasProtectedItems: false,
    error: new Error('sync') as Error | undefined, subscribe: () => () => {}, getRevision: () => 1,
    refresh: vi.fn(() => new Promise<void>(resolve => { finish = () => { presentation.error = undefined; resolve(); }; })) };
  try {
    render(<MemoryRouter><Toolbar right={<ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} />} /></MemoryRouter>);
    const sync = screen.getByRole('button', { name: 'chat.timelineRefresh' });
    sync.focus(); fireEvent.click(sync);
    await waitFor(() => expect(sync).toBeDisabled());
    fireEvent.blur(sync); sync.blur();
    await act(async () => finish());
    await waitFor(() => expect(fallback).toHaveFocus());
  } finally { unregisterDefaultFocus(restore); fallback.remove(); }
});

it('retorna ao campo do chat embutido quando a sincronização remove o botão', async () => {
  const inputRef = createRef<HTMLTextAreaElement>();
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [], hasProtectedItems: false,
    error: new Error('sync') as Error | undefined, subscribe: () => () => {}, getRevision: () => 1, refresh: vi.fn().mockResolvedValue(undefined) };
  const view = () => <MemoryRouter><Modal isOpen title="chat" onClose={() => {}}><textarea ref={inputRef} aria-label="campo" /><Toolbar right={<ChatTimelineRecovery inputRef={inputRef} presentation={presentation as unknown as ChatTimelinePresentation} />} /></Modal></MemoryRouter>;
  const { rerender } = render(view());
  screen.getByRole('button', { name: 'chat.timelineRefresh' }).focus();
  presentation.error = undefined;
  rerender(view());
  await waitFor(() => expect(inputRef.current).toHaveFocus());
});

it('devolve o foco ao chat embutido ao adiar a decisão', async () => {
  const inputRef = createRef<HTMLTextAreaElement>();
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [] as typeof descriptor[], hasProtectedItems: false,
    subscribe: () => () => {}, getRevision: () => 1, client: {} };
  const view = () => <MemoryRouter><Modal isOpen title="chat" onClose={() => {}}><textarea ref={inputRef} aria-label="campo" /><Toolbar right={<ChatTimelineRecovery inputRef={inputRef} presentation={presentation as unknown as ChatTimelinePresentation} />} /></Modal></MemoryRouter>;
  const { rerender } = render(view());
  inputRef.current!.focus();
  presentation.recoveredItems = [descriptor];
  rerender(view());
  fireEvent.click(await screen.findByRole('button', { name: /chat.timelineRecoveryLater/ }));
  await waitFor(() => expect(inputRef.current).toHaveFocus());
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
  const origin = document.createElement('button'); document.body.appendChild(origin); origin.focus();
  const make = (surfaceId: string) => ({ surfaceId, conversationId: surfaceId, loading: false, recoveredItems: [descriptor], hasProtectedItems: true,
    subscribe: () => () => {}, getRevision: () => 1, client: {} } as unknown as ChatTimelinePresentation);
  render(<MemoryRouter><Toolbar right={<><ChatTimelineRecovery presentation={make('a')} /><ChatTimelineRecovery presentation={make('b')} /></>} /></MemoryRouter>);
  expect(await screen.findAllByRole('alertdialog')).toHaveLength(1);
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoveryLater/ }));
  await waitFor(() => expect(screen.getAllByRole('alertdialog')).toHaveLength(1));
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoveryLater/ }));
  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
  await waitFor(() => expect(origin).toHaveFocus());
  origin.remove();
});

it('restaura a origem se outro cliente resolver o problema enquanto a decisão está aberta', async () => {
  const origin = document.createElement('button'); document.body.appendChild(origin); origin.focus();
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [descriptor], hasProtectedItems: false,
    subscribe: () => () => {}, getRevision: () => 1, client: {} };
  const view = () => <MemoryRouter><Toolbar right={<ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} />} /></MemoryRouter>;
  const { rerender } = render(view());
  expect(await screen.findByRole('alertdialog')).toBeVisible();
  presentation.recoveredItems = [];
  rerender(view());
  await waitFor(() => expect(origin).toHaveFocus());
  origin.remove();
});

it('preserva a origem após falha e sucesso no fechamento do host', async () => {
  const origin = document.createElement('button'); document.body.appendChild(origin); origin.focus();
  const close = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(undefined);
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [],
    subscribe: () => () => {}, getRevision: () => 1, close } as unknown as ChatTimelinePresentation;
  useChatTimelineRecoveryStore.getState().hold(presentation);
  render(<MemoryRouter><ChatTimelineRecoveryHost /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('button', { name: /chat.timelineRecoveryRetryClose/ }));
  expect(await screen.findByRole('alertdialog')).toHaveTextContent('chat.timelineRecoveryError');
  fireEvent.click(screen.getByRole('button', { name: /chat.timelineRecoveryRetryClose/ }));
  await waitFor(() => expect(close).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(origin).toHaveFocus());
  origin.remove();
});

it('permite adiar recuperação do host e retomá-la na toolbar global sem liberar conteúdo', async () => {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [descriptor], hasProtectedItems: true,
    subscribe: () => () => {}, getRevision: () => 1, client: { releaseRecoveredItem: vi.fn() } } as unknown as ChatTimelinePresentation;
  useChatTimelineRecoveryStore.getState().hold(presentation);
  render(<MemoryRouter><Toolbar right={<ChatTimelineRecoveryPendingButton />} /><ChatTimelineRecoveryHost /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('button', { name: /chat.timelineRecoveryLater/ }));
  await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
  expect(useChatTimelineRecoveryStore.getState().pending).toContain(presentation);
  expect(presentation.client.releaseRecoveredItem).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'chat.timelineRecoveryResume' }));
  expect(await screen.findByRole('alertdialog')).toBeVisible();
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

it('permite sincronizar com cópias recuperadas e erro ativo', async () => {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [descriptor], hasProtectedItems: true,
    error: new Error('sync'), subscribe: () => () => {}, getRevision: () => 1, client: {}, refresh: vi.fn().mockResolvedValue(undefined) };
  render(<MemoryRouter><Toolbar right={<ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} />} /></MemoryRouter>);
  const dialog = await screen.findByRole('alertdialog');
  fireEvent.click(within(dialog).getByRole('button', { name: /chat.timelineRefresh/ }));
  await waitFor(() => expect(presentation.refresh).toHaveBeenCalledOnce());
});

it('avisa novamente quando a sincronização transfere uma nova cópia recuperável', async () => {
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [] as typeof descriptor[], hasProtectedItems: true,
    error: new Error('sync') as Error | undefined, subscribe: () => () => {}, getRevision: () => 1, client: {}, refresh: vi.fn() };
  presentation.refresh.mockImplementation(async () => { presentation.error = undefined; presentation.recoveredItems = [descriptor]; });
  render(<MemoryRouter><Toolbar right={<ChatTimelineRecovery presentation={presentation as unknown as ChatTimelinePresentation} />} /></MemoryRouter>);
  const dialog = await screen.findByRole('alertdialog');
  fireEvent.click(within(dialog).getByRole('button', { name: /chat.timelineRefresh/ }));
  await waitFor(() => expect(presentation.refresh).toHaveBeenCalledOnce());
  expect(await screen.findByRole('button', { name: /chat.timelineRecoveryOpen/ })).toBeVisible();
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

it('permite nova tentativa na leitura se fechar falhar depois da última liberação', async () => {
  const origin = document.createElement('button'); document.body.appendChild(origin); origin.focus();
  const { presentation, typed } = fixture();
  vi.mocked(requestConfirm).mockResolvedValue(true);
  presentation.client.releaseRecoveredItem.mockImplementationOnce(async () => { presentation.recoveredItems = []; });
  presentation.close.mockRejectedValueOnce(new Error('offline'));
  fireEvent.click(screen.getByRole('button', { name: 'chat.timelineRecoveryRelease' }));
  expect(await screen.findByText('chat.timelineRecoveryError')).toBeVisible();
  expect(useChatTimelineRecoveryStore.getState().pending).toContain(typed);
  fireEvent.click(screen.getByRole('button', { name: 'chat.timelineRecoveryRetryClose' }));
  await waitFor(() => expect(presentation.close).toHaveBeenCalledTimes(2));
  expect(useChatTimelineRecoveryStore.getState().pending).not.toContain(typed);
  await waitFor(() => expect(origin).toHaveFocus());
  origin.remove();
});

it('restaura foco após fechar pelo diálogo de recuperação pendente', async () => {
  const origin = document.createElement('button'); document.body.appendChild(origin); origin.focus();
  const presentation = { surfaceId: 'surface', conversationId: 'conversation', loading: false, recoveredItems: [],
    subscribe: () => () => {}, getRevision: () => 1, close: vi.fn().mockResolvedValue(undefined) } as unknown as ChatTimelinePresentation;
  useChatTimelineRecoveryStore.getState().hold(presentation);
  render(<MemoryRouter><ChatTimelineRecoveryHost /></MemoryRouter>);
  fireEvent.click(await screen.findByRole('button', { name: /chat.timelineRecoveryRetryClose/ }));
  await waitFor(() => expect(origin).toHaveFocus());
  origin.remove();
});
