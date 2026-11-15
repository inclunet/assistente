import { useEffect, useRef, useState, useSyncExternalStore, type RefObject } from 'react';
import { announce } from '../../hooks/useAnnouncer';
import { useTranslation } from 'react-i18next';
import { Modal, useModalId } from '../ui/Modal';
import { Button } from '../ui/Button';
import { ToolbarButton } from '../ui/Toolbar';
import { DecisionDialog, type DecisionAction } from '../ui/DecisionDialog';
import { getTopmostModalID, subscribeModalRegistry } from '../../lib/modalRegistry';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { useChatTimelineRecoveryStore } from '../../store/chatTimelineRecoveryStore';
import { clearRecoveryOrigin, rememberRecoveryOrigin, useChatTimelineRecoveryDialogStore } from '../../store/chatTimelineRecoveryDialogStore';
import { requestConfirm } from '../../store/confirmStore';
import type { ChatTimelinePresentation } from '../../services/chatTimelinePresentation';
import type { TimelineItem } from '../../types/chatTimelineEvents';
import { recoveryItemKey, type RecoveredTimelineItem } from '../../services/timelineRecoveryItem';

export function ChatTimelineRecoveryHost() {
  const pending = useChatTimelineRecoveryStore(state => state.pending);
  const deferred = useChatTimelineRecoveryStore(state => state.deferred);
  const presentation = pending.find(item => !deferred.includes(item));
  return presentation ? <ChatTimelineRecovery key={presentation.surfaceId} presentation={presentation} closing /> : null;
}

export function ChatTimelineRecovery({ presentation, closing = false, active = true, inputRef }: { presentation: ChatTimelinePresentation; closing?: boolean; active?: boolean; inputRef?: RefObject<HTMLTextAreaElement> }) {
  const { t } = useTranslation();
  const parentModalId = useModalId();
  const topModalId = useSyncExternalStore(subscribeModalRegistry, getTopmostModalID);
  const owner = useRef(Symbol('timeline-recovery')).current;
  const dialogOwner = useChatTimelineRecoveryDialogStore(state => state.owner);
  useSyncExternalStore(presentation.subscribe, presentation.getRevision);
  const [open, setOpen] = useState(false);
  const [decisionOpen, setDecisionOpen] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const [content, setContent] = useState<{ key: string; item: TimelineItem }>();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const buttonFocused = useRef(false);
  const origin = useRef<HTMLElement | null>(null);
  const recovered = presentation.recoveredItems;
  const severe = closing || recovered.length > 0 || (!!presentation.error && (presentation.hasProtectedItems || failed));
  const needsRecovery = severe || !!presentation.error;
  useEffect(() => {
    if (!severe || !active || dismissed) {
      useChatTimelineRecoveryDialogStore.getState().release(owner);
      if (!severe) {
        if (decisionOpen || open) restoreOrigin();
        setDismissed(false); setDecisionOpen(false); setOpen(false);
      }
      if (!active) { setDecisionOpen(false); setOpen(false); }
      return;
    }
    if (!busy && !open && !decisionOpen && (!topModalId || topModalId === parentModalId) &&
        useChatTimelineRecoveryDialogStore.getState().acquire(owner)) {
      origin.current = rememberRecoveryOrigin(document.activeElement instanceof HTMLElement ? document.activeElement : null);
      setDecisionOpen(true);
    }
  }, [active, dismissed, busy, open, severe, decisionOpen, topModalId, parentModalId, dialogOwner, owner]);
  useEffect(() => () => useChatTimelineRecoveryDialogStore.getState().release(owner), [owner]);
  useEffect(() => { if (active && needsRecovery && !severe) announce(t('chat.timelineSyncNotice')); }, [active, needsRecovery, severe, t]);
  useEffect(() => { if (failed) announce(t('chat.timelineRecoveryError')); }, [failed, t]);
  useEffect(() => {
    if (!needsRecovery && buttonFocused.current) {
      buttonFocused.current = false;
      if ((document.activeElement === document.body || document.activeElement === document.documentElement) && topModalId === parentModalId) {
        if (parentModalId) inputRef?.current?.focus();
        else restoreDefaultFocus();
      }
    }
  }, [needsRecovery, topModalId, parentModalId, inputRef]);
  function restoreOrigin() {
    const target = origin.current;
    requestAnimationFrame(() => {
      if (getTopmostModalID() !== parentModalId) return;
      if (document.activeElement !== document.body && document.activeElement !== document.documentElement) return;
      const targetOwner = target?.closest<HTMLElement>('[data-modal-id]')?.dataset.modalId ?? null;
      if (target && target !== document.body && target !== document.documentElement && target.isConnected &&
          targetOwner === parentModalId && !target.closest('[hidden], [inert], [aria-hidden="true"]') && !target.matches(':disabled')) target.focus();
      if (document.activeElement === document.body || document.activeElement === document.documentElement) {
        if (parentModalId) inputRef?.current?.focus();
        else restoreDefaultFocus();
      }
      if (!useChatTimelineRecoveryDialogStore.getState().owner) clearRecoveryOrigin();
    });
  }
  const run = async (action: () => Promise<void>, reopenNew = false) => {
    const before = new Set(presentation.recoveredItems.map(item => `${item.windowId}:${recoveryItemKey(item.identity)}`));
    setBusy(true); setFailed(false);
    try {
      await action();
      if (reopenNew && presentation.recoveredItems.some(item => !before.has(`${item.windowId}:${recoveryItemKey(item.identity)}`))) setDismissed(false);
    } catch { setFailed(true); if (!open) setDismissed(false); } finally { setBusy(false); }
  };
  const finish = async () => {
    await presentation.close();
    useChatTimelineRecoveryStore.getState().remove(presentation);
    restoreOrigin();
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
  if (!needsRecovery) return null;
  const actions: [DecisionAction, ...DecisionAction[]] = recovered.length
    ? [{ id: 'view', label: t('chat.timelineRecoveryOpen'), primary: true }]
    : [{ id: closing ? 'close' : 'sync', label: t(closing ? 'chat.timelineRecoveryRetryClose' : 'chat.timelineRefresh'), primary: true }];
  if (!closing && recovered.length && presentation.error) actions.push({ id: 'sync', label: t('chat.timelineRefresh') });
  if (!closing && presentation.error) actions.push({ id: 'end', label: t('chat.timelineRecoverAtEnd') });
  actions.push({ id: 'later', label: t('chat.timelineRecoveryLater'), variant: 'outline' });
  const description = t(recovered.length || presentation.hasProtectedItems ? 'chat.timelineRecoveryDecision'
    : closing ? 'chat.timelineRecoveryClosingDecision' : 'chat.timelineRecoverySyncDecision');
  return <>
    {!closing && <ToolbarButton label={t('chat.timelineRefresh')} disabled={busy || presentation.loading}
      onFocus={() => { buttonFocused.current = true; }} onBlur={event => { if (event.currentTarget.isConnected && !event.currentTarget.disabled) buttonFocused.current = false; }}
      onClick={() => { if (severe) { setDismissed(false); } else void run(() => presentation.refresh()); }} />}
    <DecisionDialog isOpen={active && decisionOpen && !open} title={t('chat.timelineRecoveryTitle')}
      description={failed ? `${description} ${t('chat.timelineRecoveryError')}` : description} severity="info"
      actions={actions} returnFocusOnClose={false} onCancel={() => { setDismissed(true); setDecisionOpen(false); if (closing) useChatTimelineRecoveryStore.getState().defer(presentation); restoreOrigin(); }}
      onAction={id => {
        setDecisionOpen(false);
        if (id === 'view') setOpen(true);
        else if (id === 'later') { setDismissed(true); if (closing) useChatTimelineRecoveryStore.getState().defer(presentation); restoreOrigin(); }
        else if (id === 'close') void run(finish);
        else if (id === 'end') { setDismissed(true); restoreOrigin(); void run(() => presentation.recoverAtEnd(), true); }
        else if (id === 'sync') { setDismissed(true); restoreOrigin(); void run(() => presentation.refresh(), true); }
      }} />
    <Modal isOpen={active && open} onClose={() => { setOpen(false); setDismissed(!closing); if (!closing) restoreOrigin(); }}
      returnFocusOnClose={false} title={t('chat.timelineRecoveryTitle')} readingMode>
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
