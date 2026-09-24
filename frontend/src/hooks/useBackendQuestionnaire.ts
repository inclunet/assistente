import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { EventsOn } from '@wailsjs/runtime/runtime';

import { useUIStore } from '../store/uiStore';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';
import type { DialogCommandScope } from '../lib/commandBridge';
import { createDecisionQuestionnaireScope } from '../lib/decisionQuestionnaireScope';
import {
  questionnaireClosedMessage,
  QUESTIONNAIRE_CLOSED_EVENT,
  type QuestionnaireClosedEvent,
} from '../lib/questionnaireClosed';

const QUESTIONNAIRE_EVENT = 'tool:questionnaire';

export interface BackendQuestionnaire {
  /** Pergunta na tela, ou nulo quando não há nenhuma. */
  data: QuestionnairePayload | null;
  /** Restrição do dispatcher para decisão backend, vinculada ao mesmo payload. */
  scope: DialogCommandScope | null;
  /** Marca a pergunta como resolvida por quem respondeu. */
  /** Sem ID limpa tudo (logout); com ID só limpa o pedido ainda atual. */
  clear: (expectedDialogId?: string) => boolean;
}

interface BackendQuestionnaireSnapshot {
  data: QuestionnairePayload | null;
  scope: DialogCommandScope | null;
}

/**
 * Diálogos que o backend abre (permissão do agente, shell, atualização,
 * confirmação de edição) e o fechamento de quem perdeu o dono.
 *
 * O id do diálogo aberto vive em ref, e não em estado: um pedido cancelado
 * logo depois de aberto chega antes do render, e comparar com o estado antigo
 * descartaria o fechamento para sempre — a pessoa ficaria diante de um
 * diálogo pedindo decisão sobre algo que já não existe.
 */
export function useBackendQuestionnaire(onOpen?: () => void): BackendQuestionnaire {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const [snapshot, setSnapshot] = useState<BackendQuestionnaireSnapshot>({ data: null, scope: null });
  const openIdRef = useRef<string | null>(null);
  const onOpenRef = useRef(onOpen);
  onOpenRef.current = onOpen;

  useEffect(() => {
    const unsub = EventsOn(QUESTIONNAIRE_EVENT, (payload: QuestionnairePayload) => {
      onOpenRef.current?.();
      openIdRef.current = payload?.id ?? null;
      // Publica o dado e seu scope no mesmo snapshot React para que o host
      // nunca observe a pergunta nova com a restrição do diálogo anterior.
      setSnapshot({
        data: payload,
        scope: createDecisionQuestionnaireScope(payload, payload?.id),
      });
    });
    return unsub;
  }, []);

  useEffect(() => {
    // Só fecha o diálogo do próprio pedido, para não derrubar a pergunta
    // seguinte, que reusa a mesma tela.
    const unsub = EventsOn(QUESTIONNAIRE_CLOSED_EVENT, (event: QuestionnaireClosedEvent) => {
      if (!event?.id || event.id !== openIdRef.current) {
        return;
      }
      openIdRef.current = null;
      setSnapshot({ data: null, scope: null });
      addToast(questionnaireClosedMessage(t, event), 'warning', 8000);
    });
    return unsub;
  }, [addToast, t]);

  const clear = useCallback((expectedDialogId?: string) => {
    if (expectedDialogId !== undefined && openIdRef.current !== expectedDialogId) return false;
    openIdRef.current = null;
    setSnapshot({ data: null, scope: null });
    return true;
  }, []);

  return { ...snapshot, clear };
}
