import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useWailsEvent } from './useWails';
import { useAnnouncer } from './useAnnouncer';
import { useAuthStore } from '../store/authStore';
import { useSubAgentRunsStore } from '../store/subAgentRunsStore';
import {
  SUBAGENT_RUN_FINISHED_EVENT,
  SUBAGENT_RUN_STARTED_EVENT,
  type SubAgentRunEvent,
} from '../types/subagentRuns';
import type { VoiceAccessibilityOrigin } from '../services/voiceAccessibility/types';

/**
 * Assina os eventos de run de sub-agente, mantém a lista de runs viva e anuncia
 * início e fim de trabalho em segundo plano (AEP-0068 F5 + AEP-0058).
 *
 * Só runs em **background** são anunciados: o sub-agente síncrono responde
 * dentro do turno do pai, que já é falado, e repetir isso seria ruído.
 *
 * NÃO cria live region própria — usa o announcer global único. Deve ser montado
 * uma única vez na árvore (App), nunca por aba.
 */
export function useSubAgentRunEvents() {
  const { t } = useTranslation();
  const { announceRequest } = useAnnouncer();
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const resetRuns = useSubAgentRunsStore((s) => s.reset);

  // O hook vive em App (fora do AuthGate). Sem limpar no logout, a toolbar do
  // histórico e o modal herdariam runs/contadores do usuário anterior.
  useEffect(() => {
    if (!isAuthenticated) {
      resetRuns();
    }
  }, [isAuthenticated, resetRuns]);

  // Um run em segundo plano não pertence a nenhuma aba: ele sobrevive ao turno
  // que o disparou e pode terminar com o usuário em qualquer superfície. Marcar
  // a origem como externa evita que a política de aba inativa silencie um aviso
  // que não tem aba dona.
  const buildOrigin = useCallback(
    (event: SubAgentRunEvent): VoiceAccessibilityOrigin => ({
      conversationId: event.conversationId,
      surfaceType: 'external',
      isExternal: true,
      title: event.title,
    }),
    [],
  );

  const runLabel = useCallback(
    (event: SubAgentRunEvent) => event.title?.trim() || t('subAgentRuns.untitled', 'Sub-agente'),
    [t],
  );

  const handleStarted = useCallback(
    (event: SubAgentRunEvent) => {
      if (!event?.runId) return;
      void useSubAgentRunsStore.getState().fetchRuns();
      if (!event.background) return;
      // waitsForReading: progress puro é descartado enquanto o broker protege a
      // leitura do assistente (AEP-0058 §2.1). O início de run em background
      // precisa chegar — espera a leitura terminar, sem atropelá-la.
      announceRequest({
        message: t('subAgentRuns.announce.started', { title: runLabel(event) }),
        origin: buildOrigin(event),
        eventType: 'progress',
        waitsForReading: true,
      });
    },
    [announceRequest, buildOrigin, runLabel, t],
  );

  const handleFinished = useCallback(
    (event: SubAgentRunEvent) => {
      if (!event?.runId) return;
      void useSubAgentRunsStore.getState().fetchRuns();
      if (!event.background) return;
      // `completion` de propósito, inclusive quando o run falhou: é um desfecho
      // de trabalho de fundo, não um erro que exige atenção imediata. Assim o
      // aviso espera a leitura do conteúdo do assistente terminar em vez de
      // atropelá-la (AEP-0058 §2.1), sem nunca ser descartado.
      announceRequest({
        message: t(`subAgentRuns.announce.finished.${event.status}`, {
          title: runLabel(event),
          defaultValue: t('subAgentRuns.announce.finished.unknown', { title: runLabel(event) }),
        }),
        origin: buildOrigin(event),
        eventType: 'completion',
      });
    },
    [announceRequest, buildOrigin, runLabel, t],
  );

  useWailsEvent<SubAgentRunEvent>(SUBAGENT_RUN_STARTED_EVENT, handleStarted);
  useWailsEvent<SubAgentRunEvent>(SUBAGENT_RUN_FINISHED_EVENT, handleFinished);
}
