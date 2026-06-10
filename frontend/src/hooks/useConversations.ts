import { useEffect, useState } from 'react';
import { GetConversations } from '@wailsjs/go/app/App';
import type { database } from '@wailsjs/go/models';
import { logger } from '../utils/logger';

export interface UseConversationsResult {
  conversations: database.Conversation[];
  loading: boolean;
  error: string | null;
}

/**
 * Carrega as conversas (ordenadas por updatedAt desc) para popular seletores de
 * vínculo. Centraliza o fetch + sort + stale guard que antes era duplicado em
 * TaskForm, TaskDetailModal e TaskListView.
 *
 * @param enabled quando false, não dispara o carregamento (ex.: editor/modal
 *                fechado). Recarrega sempre que passa de false para true.
 */
export function useConversations(enabled: boolean = true): UseConversationsResult {
  const [conversations, setConversations] = useState<database.Conversation[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    setLoading(true);
    setError(null);
    void (async () => {
      try {
        const result = await GetConversations();
        if (!active) return;
        const sorted = [...result].sort((a, b) => {
          const dateA = new Date(a.updatedAt as string | number | Date).getTime();
          const dateB = new Date(b.updatedAt as string | number | Date).getTime();
          return dateB - dateA;
        });
        setConversations(sorted);
      } catch (err) {
        if (!active) return;
        logger.error('[useConversations] erro ao carregar conversas:', err);
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [enabled]);

  return { conversations, loading, error };
}
