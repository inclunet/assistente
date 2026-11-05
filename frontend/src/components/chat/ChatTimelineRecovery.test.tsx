import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { ChatTimelineRecovery } from './ChatTimelineRecovery';
import { useChatTimelineRecoveryStore } from '../../store/chatTimelineRecoveryStore';
import type { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';
import { requestConfirm } from '../../store/confirmStore';
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
  fireEvent.click(screen.getByText('chat.timelineRecoveryOpen'));
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
