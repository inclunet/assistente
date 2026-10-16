export type CommandCompositionState = 'active' | 'inactive' | 'unknown';

interface FocusTrackingState {
  refs: number;
  composition: CommandCompositionState;
  compositionTarget: Element | null;
  readonly onCompositionStart: (event: Event) => void;
  readonly onCompositionUpdate: (event: Event) => void;
  readonly onCompositionEnd: (event: Event) => void;
  readonly onFocusLost: () => void;
  readonly onWindowBlur: () => void;
  readonly onKeyDown: (event: KeyboardEvent) => void;
}

const trackingByDocument = new WeakMap<Document, FocusTrackingState>();

function resetAfterFocusLoss(state: FocusTrackingState): void {
  // Perder blur/focus invalida a prova anterior. Não declarar inactive aqui:
  // um compositionend perdido não pode ser tratado como composição encerrada.
  state.composition = 'unknown';
  state.compositionTarget = null;
}

function focusedElement(documentRef: Document): Element | null {
  const active = documentRef.activeElement;
  return active instanceof Element ? active : null;
}

function createTrackingState(documentRef: Document): FocusTrackingState {
  const state: FocusTrackingState = {
    refs: 0,
    composition: 'unknown' as CommandCompositionState,
    compositionTarget: null,
    onCompositionStart: (event: Event) => {
      if (!(event.target instanceof Element) || focusedElement(documentRef) !== event.target) return;
      state.composition = 'active';
      state.compositionTarget = event.target;
    },
    onCompositionUpdate: (event: Event) => {
      if (!(event.target instanceof Element) || focusedElement(documentRef) !== event.target) return;
      state.composition = 'active';
      state.compositionTarget = event.target;
    },
    onCompositionEnd: (event: Event) => {
      // Um compositionend atrasado do elemento anterior não é evidência de
      // que a composição do novo foco terminou.
      if (
        !(event.target instanceof Element) ||
        state.compositionTarget !== event.target ||
        focusedElement(documentRef) !== event.target
      ) return;
      state.composition = 'inactive';
    },
    onFocusLost: () => {
      resetAfterFocusLoss(state);
    },
    onWindowBlur: () => {
      resetAfterFocusLoss(state);
    },
    onKeyDown: (event: KeyboardEvent) => {
      if (
        (event.isComposing || event.keyCode === 229) &&
        event.target instanceof Element &&
        focusedElement(documentRef) === event.target
      ) {
        state.composition = 'active';
        state.compositionTarget = event.target;
      }
    },
  };

  documentRef.addEventListener('compositionstart', state.onCompositionStart, true);
  documentRef.addEventListener('compositionupdate', state.onCompositionUpdate, true);
  documentRef.addEventListener('compositionend', state.onCompositionEnd, true);
  documentRef.addEventListener('compositioncancel', state.onCompositionEnd, true);
  documentRef.addEventListener('blur', state.onFocusLost, true);
  documentRef.addEventListener('focusout', state.onFocusLost, true);
  documentRef.addEventListener('keydown', state.onKeyDown, true);
  documentRef.defaultView?.addEventListener('blur', state.onWindowBlur);
  return state;
}

function releaseTracking(documentRef: Document, state: FocusTrackingState): void {
  if (trackingByDocument.get(documentRef) !== state) return;
  state.refs -= 1;
  if (state.refs > 0) return;

  documentRef.removeEventListener('compositionstart', state.onCompositionStart, true);
  documentRef.removeEventListener('compositionupdate', state.onCompositionUpdate, true);
  documentRef.removeEventListener('compositionend', state.onCompositionEnd, true);
  documentRef.removeEventListener('compositioncancel', state.onCompositionEnd, true);
  documentRef.removeEventListener('blur', state.onFocusLost, true);
  documentRef.removeEventListener('focusout', state.onFocusLost, true);
  documentRef.removeEventListener('keydown', state.onKeyDown, true);
  documentRef.defaultView?.removeEventListener('blur', state.onWindowBlur);
  trackingByDocument.delete(documentRef);
}

/** Installs one shared local tracker and returns the matching release lease. */
export function acquireCommandFocusTracking(documentRef?: Document): () => void {
  const target = documentRef ?? (typeof document === 'undefined' ? undefined : document);
  if (!target) return () => undefined;

  let state = trackingByDocument.get(target);
  if (!state) {
    state = createTrackingState(target);
    trackingByDocument.set(target, state);
  }
  state.refs += 1;
  let released = false;
  return () => {
    if (released) return;
    released = true;
    releaseTracking(target, state as FocusTrackingState);
  };
}

/**
 * Amostra síncrona do keydown que será consumido pelo ingresso local. O
 * listener de captura da Window pode impedir que ele alcance o Document.
 * Um evento explicitamente não composing no foco atual fornece evidência
 * para unknown; nunca apaga uma composição ativa sem compositionend.
 */
export function observeCommandShortcutComposition(event: KeyboardEvent): void {
  if (event.type !== 'keydown' || !(event.target instanceof Element)) return;
  const documentRef = event.target.ownerDocument;
  const state = trackingByDocument.get(documentRef);
  if (!state || focusedElement(documentRef) !== event.target) return;
  if (event.isComposing || event.keyCode === 229) {
    state.composition = 'active';
    state.compositionTarget = event.target;
    return;
  }
  if (event.isComposing !== false || state.composition === 'active') return;
  state.composition = 'inactive';
  state.compositionTarget = event.target;
}

/** Returns only evidence observed by an acquired tracker; otherwise unknown. */
export function readCommandCompositionState(
  documentRef: Document | undefined,
  editable: boolean,
): CommandCompositionState {
  if (!editable) return 'inactive';
  if (!documentRef) return 'unknown';
  const state = trackingByDocument.get(documentRef);
  if (!state || state.compositionTarget !== focusedElement(documentRef)) return 'unknown';
  return state.composition;
}
