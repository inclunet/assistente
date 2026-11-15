import { useTranslation } from 'react-i18next';
import { useChatTimelineRecoveryStore } from '../../store/chatTimelineRecoveryStore';
import { ToolbarButton } from '../ui/Toolbar';

/** Acesso persistente às recuperações adiadas após o fechamento da aba. */
export function ChatTimelineRecoveryPendingButton() {
  const { t } = useTranslation();
  const deferred = useChatTimelineRecoveryStore(state => state.deferred);
  if (!deferred.length) return null;
  return <ToolbarButton label={t('chat.timelineRecoveryResume')} onClick={() => useChatTimelineRecoveryStore.getState().resume(deferred[0])} />;
}
