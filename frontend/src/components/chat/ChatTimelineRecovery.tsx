import { useEffect, useState, useSyncExternalStore } from 'react';
import { announce } from '../../hooks/useAnnouncer';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { Button } from '../ui/Button';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { useChatTimelineRecoveryStore } from '../../store/chatTimelineRecoveryStore';
import { requestConfirm } from '../../store/confirmStore';
import type { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';
import type { TimelineItem } from '../../types/chatTimelineEvents';
import { recoveryItemKey, type RecoveredTimelineItem } from '../../services/timelineRecoveryItem';

export function ChatTimelineRecoveryHost() {
  const pending = useChatTimelineRecoveryStore(state => state.pending);
  return <>{pending.map(presentation => <ChatTimelineRecovery key={presentation.surfaceId} presentation={presentation} closing />)}</>;
}

export function ChatTimelineRecovery({ presentation, closing = false }: { presentation: ChatTimelinePresentation; closing?: boolean }) {
  const { t } = useTranslation();
  useSyncExternalStore(presentation.subscribe, presentation.getRevision);
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState<{ key: string; item: TimelineItem }>();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const recovered = presentation.recoveredItems;
  const needsRecovery = closing || !!presentation.error || recovered.length > 0;
  useEffect(() => { if (needsRecovery) announce(t('chat.timelineRecoveryNotice')); }, [needsRecovery, t]);
  useEffect(() => { if (failed) announce(t('chat.timelineRecoveryError')); }, [failed, t]);
  const run = async (action: () => Promise<void>) => {
    setBusy(true); setFailed(false);
    try { await action(); } catch { setFailed(true); } finally { setBusy(false); }
  };
  const finish = async () => {
    await presentation.close();
    useChatTimelineRecoveryStore.getState().remove(presentation);
  };
  const read = (descriptor: RecoveredTimelineItem) => run(async () => {
    const item = await presentation.client.readRecoveredItem(presentation.conversationId, descriptor);
    setContent({ key: `${descriptor.windowId}:${recoveryItemKey(descriptor.identity)}`, item });
  });
  const release = (descriptor: RecoveredTimelineItem) => run(async () => {
    if (!await requestConfirm({ title: t('chat.timelineRecoveryTitle'), message: t('chat.timelineRecoveryReleaseConfirm'), confirmText: t('chat.timelineRecoveryRelease') })) return;
    await presentation.client.releaseRecoveredItem(presentation.conversationId, descriptor);
    setContent(undefined);
    if (closing && !presentation.recoveredItems.length) await finish();
  });
  if (!closing && !presentation.error && !recovered.length) return null;
  return <>
    <div>
      <span>{t('chat.timelineRecoveryNotice')}</span>
      <Button onClick={() => setOpen(true)}>{t('chat.timelineRecoveryOpen')}</Button>
      {!closing && <Button disabled={busy || presentation.loading} onClick={() => { void run(() => presentation.refresh()); }}>{t('chat.timelineRefresh')}</Button>}
      {!closing && <Button disabled={busy || presentation.loading} onClick={() => { void run(() => presentation.recoverAtEnd()); }}>{t('chat.timelineRecoverAtEnd')}</Button>}
    </div>
    <Modal isOpen={open} onClose={() => setOpen(false)} title={t('chat.timelineRecoveryTitle')} readingMode>
      {failed && <p>{t('chat.timelineRecoveryError')}</p>}
      {recovered.map(descriptor => <section key={`${descriptor.windowId}:${recoveryItemKey(descriptor.identity)}`}>
        <Button disabled={busy} onClick={() => { void read(descriptor); }}>{t('chat.timelineRecoveryRead')}</Button>
        <Button disabled={busy} onClick={() => { void release(descriptor); }}>{t('chat.timelineRecoveryRelease')}</Button>
        {content?.key === `${descriptor.windowId}:${recoveryItemKey(descriptor.identity)}` && content.item.segments.map(segment =>
          <div key={segment.segmentId}>
            {segment.text && <MarkdownRenderer content={segment.text} />}
            {segment.invocations?.map(call => <p key={call.callId}>{call.name}: {call.outputPreview ?? call.inputPreview}</p>)}
          </div>)}
      </section>)}
      {closing && !recovered.length && <Button disabled={busy} onClick={() => { void run(finish); }}>{t('chat.timelineRecoveryRetryClose')}</Button>}
    </Modal>
  </>;
}
