import { create } from 'zustand';
import type { ChatTimelinePresentation } from '../services/chatTimelinePresentation';
import { useAuthStore } from './authStore';

/** Inscrições cujo fechamento foi recusado continuam acessíveis fora da aba. */
export const useChatTimelineRecoveryStore = create<{
  pending: ChatTimelinePresentation[];
  hold(presentation: ChatTimelinePresentation): void;
  remove(presentation: ChatTimelinePresentation): void;
}>(set => ({
  pending: [],
  hold: presentation => set(state => state.pending.includes(presentation) ? state : { pending: [...state.pending, presentation] }),
  remove: presentation => set(state => ({ pending: state.pending.filter(value => value !== presentation) })),
}));

useAuthStore.subscribe((state, previous) => {
  if (!state.isAuthenticated || state.user?.userId !== previous.user?.userId || state.user?.sessionId !== previous.user?.sessionId) {
    useChatTimelineRecoveryStore.setState({ pending: [] });
  }
});
