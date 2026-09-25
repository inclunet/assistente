import { getModalRegistrySnapshot } from './modalRegistry';

/** Agenda retorno de foco apenas se o frame ainda não pertence a outro diálogo. */
export function scheduleQuestionnaireFocusRestore(
  isQuestionnaireOpen: () => boolean,
  preferredTarget: HTMLElement | null,
  fallbackTarget: () => HTMLElement | null,
  scheduleFrame: (callback: FrameRequestCallback) => number = window.requestAnimationFrame.bind(window),
): void {
  // Capture both the actual node and its modal owner now. The ref may be
  // overwritten by a later questionnaire before this frame runs.
  const target = preferredTarget;
  const ownerModalId = target?.closest<HTMLElement>('[data-modal-id]')?.dataset.modalId ?? null;

  scheduleFrame(() => {
    if (isQuestionnaireOpen()) return;
    const currentTopID = getModalRegistrySnapshot().topID;

    if (ownerModalId !== null) {
      if (currentTopID !== ownerModalId || !target || !document.contains(target)) return;
      target.focus();
      return;
    }

    // Page-level fallback is safe only when no modal appeared since scheduling.
    if (currentTopID !== null) return;

    if (target && document.contains(target)) {
      target.focus();
      return;
    }
    fallbackTarget()?.focus();
  });
}
