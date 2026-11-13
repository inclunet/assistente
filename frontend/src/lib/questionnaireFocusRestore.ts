import { getModalRegistrySnapshot } from './modalRegistry';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';

export interface QuestionnaireFocusOrigin {
  target: HTMLElement | null;
  ownerModalId: string | null;
  readingRoot: HTMLElement | null;
}

/** Captura antes da abertura: o alvo pode ser removido durante a pergunta. */
export function captureQuestionnaireFocusOrigin(target: HTMLElement | null): QuestionnaireFocusOrigin {
  return {
    target,
    ownerModalId: target?.closest<HTMLElement>('[data-modal-id]')?.dataset.modalId ?? null,
    readingRoot: target?.closest<HTMLElement>('[data-message-node][role="dialog"]') ?? null,
  };
}

/** Captura antes de publicar a abertura e antes de qualquer autoFocus do formulário. */
export function subscribeUIQuestionnaireFocusOrigin(
  isQuestionnaireOpen: () => boolean,
  onOrigin: (origin: QuestionnaireFocusOrigin) => void,
): () => void {
  return useQuestionnaireUIStore.subscribe((state, previous) => {
    if (state.active && !previous.active && !isQuestionnaireOpen()) {
      onOrigin(captureQuestionnaireFocusOrigin(document.activeElement as HTMLElement | null));
    }
  });
}

/** Não escolher um compositor pertencente a uma aba mantida oculta. */
export function findActiveChatComposer(): HTMLTextAreaElement | null {
  const inputs = [...document.querySelectorAll<HTMLTextAreaElement>('.chat-input__textarea')]
    .filter(input => !input.closest('[hidden], [inert], [aria-hidden="true"]'));
  return inputs.find(input => input.closest('[data-active="true"]')) ?? (inputs.length === 1 ? inputs[0] : null);
}

/** Agenda retorno de foco apenas se o frame ainda não pertence a outro diálogo. */
export function scheduleQuestionnaireFocusRestore(
  isQuestionnaireOpen: () => boolean,
  origin: QuestionnaireFocusOrigin | null,
  fallbackTarget: () => HTMLElement | null,
  scheduleFrame: (callback: FrameRequestCallback) => number = window.requestAnimationFrame.bind(window),
): void {
  // Capture both the actual node and its modal owner now. The ref may be
  // overwritten by a later questionnaire before this frame runs.
  const { target, ownerModalId, readingRoot } = origin ?? captureQuestionnaireFocusOrigin(null);

  const focus = (element: HTMLElement | null): boolean => {
    if (!element || element === document.body || element === document.documentElement || !element.isConnected ||
        element.closest('[hidden], [inert], [aria-hidden="true"]') ||
        element.matches(':disabled')) return false;
    element.focus();
    return document.activeElement === element;
  };

  scheduleFrame(() => {
    if (isQuestionnaireOpen()) return;
    const currentTopID = getModalRegistrySnapshot().topID;
    const active = document.activeElement;
    // Uma interação posterior já assumiu o foco; não a sobrescrever.
    if (active && active !== document.body && active !== document.documentElement && active !== target) return;

    const restoreReading = () => {
      if (!readingRoot?.isConnected || readingRoot.getAttribute('role') !== 'dialog') return false;
      return focus(readingRoot.querySelector<HTMLElement>('[role="document"]') ?? readingRoot);
    };

    if (ownerModalId !== null) {
      if (currentTopID !== ownerModalId) return;
      if (!focus(target)) restoreReading();
      return;
    }

    // Page-level fallback is safe only when no modal appeared since scheduling.
    if (currentTopID !== null) return;

    if (focus(target) || restoreReading()) return;
    focus(fallbackTarget());
  });
}
