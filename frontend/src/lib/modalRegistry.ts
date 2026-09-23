// Registro NEUTRO (sem dependência de React/UI) do conjunto de modais abertos.
//
// Mantém a stack global de modais e os efeitos globais associados (inert/aria-hidden
// no `#root` e overflow do body). É consumido tanto pelo componente `Modal` quanto por
// stores/hooks que só precisam saber se há um modal aberto. Centralizar essa lógica
// aqui evita a inversão de camadas em que a camada de estado (stores Zustand) dependia
// de um componente React (`components/ui/Modal.tsx`), e a porta aberta para imports
// circulares que isso representava.

import {
  DECISION_REPEAT_TRIGGER,
  DECISION_RESPOND_COMMAND_ID,
  type DialogCommandScope,
} from './commandBridge';

// Stack global simples para garantir que apenas o modal do topo
// trate Escape/Tab/click-outside quando há múltiplos modais abertos.
const OPEN_MODAL_STACK: string[] = [];
const OPEN_MODAL_SCOPES = new Map<string, DialogCommandScope>();
interface ChatPresentationModalScopeRecord {
  readonly commandIds: readonly string[];
  readonly token: object;
}

const OPEN_MODAL_CHAT_SCOPES = new Map<string, ChatPresentationModalScopeRecord>();

let fallbackNonceCounter = 0;
const STARTUP_NONCE = (() => {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
  } catch {
    // A non-cryptographic opaque nonce is sufficient to distinguish this
    // frontend process when randomUUID is unavailable (for example in SSR).
  }
  fallbackNonceCounter += 1;
  return `startup-${Date.now().toString(36)}-${fallbackNonceCounter.toString(36)}-${Math.random().toString(36).slice(2)}`;
})();

let modalStackGeneration = 0;
const modalStackListeners = new Set<() => void>();

export interface ModalRegistrySnapshot {
  readonly generation: string;
  /** Alias kept explicit for consumers that call this a snapshot generation. */
  readonly snapshotGeneration: string;
  readonly generationNumber: number;
  readonly topID: string | null;
  readonly ids: readonly string[];
  /** Scope somente do modal topmost; null também quando ele bloqueia fallback. */
  readonly dialogCommandScope: DialogCommandScope | null;
  readonly chatPresentationCommandIds: readonly string[] | null;
}

function stacksEqual(left: readonly string[], right: readonly string[]): boolean {
  if (left.length !== right.length) return false;
  return left.every((id, index) => id === right[index]);
}

function currentGeneration(): string {
  return `${STARTUP_NONCE}:${modalStackGeneration}`;
}

function getStackSnapshotBeforeSync(): string[] {
  return [...OPEN_MODAL_STACK];
}

function scopeEqual(
  left: DialogCommandScope | undefined,
  right: DialogCommandScope | undefined
): boolean {
  if (left === right) return true;
  if (!left || !right) return false;
  return (
    left.dialogId === right.dialogId &&
    left.kind === right.kind &&
    left.generation === right.generation &&
    left.allowedCommandIds.length === right.allowedCommandIds.length &&
    left.allowedCommandIds.every((value, index) => value === right.allowedCommandIds[index]) &&
    left.allowedTriggerSpecs.length === right.allowedTriggerSpecs.length &&
    left.allowedTriggerSpecs.every((value, index) => value === right.allowedTriggerSpecs[index])
  );
}

function scopesEqual(
  left: ReadonlyMap<string, DialogCommandScope>,
  right: ReadonlyMap<string, DialogCommandScope>
): boolean {
  if (left.size !== right.size) return false;
  for (const [id, scope] of left) {
    if (!scopeEqual(scope, right.get(id))) return false;
  }
  return true;
}

function recordStackChange(
  previous: readonly string[],
  previousScopes: ReadonlyMap<string, DialogCommandScope>
) {
  if (!stacksEqual(previous, OPEN_MODAL_STACK) || !scopesEqual(previousScopes, OPEN_MODAL_SCOPES)) {
    modalStackGeneration += 1;
    for (const listener of modalStackListeners) {
      try {
        listener();
      } catch {
        // A consumer must not prevent the registry from updating.
      }
    }
  }
}

/** Observa mudanças reais de stack/scope sem criar dependência com React. */
export function subscribeModalRegistry(listener: () => void): () => void {
  modalStackListeners.add(listener);
  return () => modalStackListeners.delete(listener);
}

function notifyModalStackGenerationChange(): void {
  modalStackGeneration += 1;
  for (const listener of modalStackListeners) {
    try {
      listener();
    } catch {
      // A notificação é best-effort; o estado da stack continua autoritativo.
    }
  }
}

function cloneDialogCommandScope(scope: DialogCommandScope | undefined): DialogCommandScope | null {
  if (
    !scope ||
    typeof scope !== 'object' ||
    typeof scope.dialogId !== 'string' ||
    scope.dialogId.length === 0 ||
    scope.dialogId.trim() !== scope.dialogId ||
    scope.kind !== 'decision' ||
    typeof scope.generation !== 'string' ||
    !/^[1-9][0-9]*$/.test(scope.generation) ||
    !Array.isArray(scope.allowedCommandIds) ||
    scope.allowedCommandIds.length !== 1 ||
    scope.allowedCommandIds[0] !== DECISION_RESPOND_COMMAND_ID ||
    !Array.isArray(scope.allowedTriggerSpecs) ||
    scope.allowedTriggerSpecs.length !== 1 ||
    scope.allowedTriggerSpecs[0] !== DECISION_REPEAT_TRIGGER
  ) {
    return null;
  }

  return Object.freeze({
    dialogId: scope.dialogId,
    kind: 'decision' as const,
    generation: scope.generation,
    allowedCommandIds: Object.freeze([DECISION_RESPOND_COMMAND_ID] as const),
    allowedTriggerSpecs: Object.freeze([DECISION_REPEAT_TRIGGER] as const),
  });
}

let previousBodyOverflow: string | null = null;

function setGlobalModalEffects(enabled: boolean) {
  const appRoot = document.getElementById('root');

  if (enabled) {
    if (appRoot) {
      appRoot.setAttribute('aria-hidden', 'true');
      appRoot.setAttribute('inert', '');
    }
    if (previousBodyOverflow === null) {
      previousBodyOverflow = document.body.style.overflow;
    }
    document.body.style.overflow = 'hidden';
    return;
  }

  if (appRoot) {
    appRoot.removeAttribute('aria-hidden');
    appRoot.removeAttribute('inert');
  }

  if (previousBodyOverflow !== null) {
    document.body.style.overflow = previousBodyOverflow;
    previousBodyOverflow = null;
  } else {
    document.body.style.overflow = '';
  }
}

function syncGlobalModalEffects() {
  if (typeof document === 'undefined') return;
  const previousStack = getStackSnapshotBeforeSync();
  const previousScopes = new Map(OPEN_MODAL_SCOPES);
  // Safety net: se a stack diz que há modais abertos, mas nenhum overlay
  // está no DOM, a stack ficou dessincronizada (ex: erro de render ou
  // unmount inesperado). Limpa a stack para restaurar a interatividade.
  if (OPEN_MODAL_STACK.length > 0) {
    const actualOverlays = document.querySelectorAll('.modal-overlay').length;
    if (actualOverlays === 0) {
      OPEN_MODAL_STACK.length = 0;
      OPEN_MODAL_SCOPES.clear();
    }
  }
  recordStackChange(previousStack, previousScopes);
  setGlobalModalEffects(OPEN_MODAL_STACK.length > 0);
}

export function isModalOpen(): boolean {
  return OPEN_MODAL_STACK.length > 0;
}

/**
 * Reads the authoritative modal stack synchronously. The returned array is a
 * detached snapshot; callers cannot mutate the registry through it.
 */
export function getModalRegistrySnapshot(): ModalRegistrySnapshot {
  // A read is authoritative even when a prior DOM notification was missed.
  // syncGlobalModalEffects only mutates the stack when the overlay is truly gone.
  syncGlobalModalEffects();
  const generation = currentGeneration();
  return Object.freeze({
    generation,
    snapshotGeneration: generation,
    generationNumber: modalStackGeneration,
    topID: OPEN_MODAL_STACK.length > 0 ? OPEN_MODAL_STACK[OPEN_MODAL_STACK.length - 1] : null,
    ids: Object.freeze([...OPEN_MODAL_STACK]),
    dialogCommandScope:
      OPEN_MODAL_STACK.length > 0
        ? (OPEN_MODAL_SCOPES.get(OPEN_MODAL_STACK[OPEN_MODAL_STACK.length - 1]) ?? null)
        : null,
    chatPresentationCommandIds:
      OPEN_MODAL_STACK.length > 0
        ? (OPEN_MODAL_CHAT_SCOPES.get(OPEN_MODAL_STACK[OPEN_MODAL_STACK.length - 1])?.commandIds ?? null)
        : null,
  });
}

export const readModalRegistrySnapshot = getModalRegistrySnapshot;

export function getModalSnapshotGeneration(): string {
  return getModalRegistrySnapshot().generation;
}

export function getTopmostModalID(): string | null {
  return getModalRegistrySnapshot().topID;
}

/** Retorna somente o scope do modal topmost; modal superior sem scope bloqueia fallback. */
export function getTopmostDialogCommandScope(): DialogCommandScope | null {
  return getModalRegistrySnapshot().dialogCommandScope;
}

export function getTopmostChatPresentationCommandIds(): readonly string[] | null {
  return getModalRegistrySnapshot().chatPresentationCommandIds;
}

export function registerChatPresentationModalScope(
  id: string,
  commandIds: readonly string[],
): (() => void) | undefined {
  if (!OPEN_MODAL_STACK.includes(id) || OPEN_MODAL_SCOPES.has(id) || commandIds.length === 0) return undefined;
  const allowed = [...commandIds];
  if (new Set(allowed).size !== allowed.length || allowed.some((commandID) =>
    commandID !== 'chat.model.open' && commandID !== 'chat.history.open' && commandID !== 'chat.profile.open' &&
    commandID !== 'chat.pinned.open' && commandID !== 'chat.tokens.open'
  )) return undefined;
  const token = {};
  const previous = OPEN_MODAL_CHAT_SCOPES.get(id);
  const next = { commandIds: Object.freeze(allowed), token };
  OPEN_MODAL_CHAT_SCOPES.set(id, next);
  if (!previous || previous.commandIds.length !== next.commandIds.length || previous.commandIds.some((value, index) => value !== next.commandIds[index])) {
    notifyModalStackGenerationChange();
  }
  return () => {
    if (OPEN_MODAL_CHAT_SCOPES.get(id)?.token !== token) return;
    OPEN_MODAL_CHAT_SCOPES.delete(id);
    notifyModalStackGenerationChange();
  };
}

export function unregisterChatPresentationModalScope(id: string): void {
  const existing = OPEN_MODAL_CHAT_SCOPES.delete(id);
  if (existing) notifyModalStackGenerationChange();
}

/**
 * Força a limpeza do estado de modal (inert/aria-hidden) quando a stack
 * ficou dessincronizada. Chamado ao navegar entre páginas como safety net.
 */
export function ensureModalCleanup() {
  // Stack já vazia: nada a sincronizar e, deliberadamente, não tocamos em
  // `body.style.overflow` (não havia modal que o tivesse alterado).
  if (OPEN_MODAL_STACK.length === 0) return;
  // Reutiliza o único caminho de sincronização: se não houver overlay no DOM,
  // a stack está dessincronizada e será zerada, removendo os efeitos globais.
  syncGlobalModalEffects();
}

/**
 * Registra um modal (por id de instância) no topo da stack global e aplica os
 * efeitos globais. Idempotente: remove qualquer entrada antiga do mesmo id antes
 * de empilhar (best-effort), garantindo que o id apareça uma única vez no topo.
 */
export function registerOpenModal(id: string, dialogCommandScope?: DialogCommandScope) {
  const previousStack = getStackSnapshotBeforeSync();
  const previousScopes = new Map(OPEN_MODAL_SCOPES);
  for (let i = OPEN_MODAL_STACK.length - 1; i >= 0; i--) {
    if (OPEN_MODAL_STACK[i] === id) OPEN_MODAL_STACK.splice(i, 1);
  }
  OPEN_MODAL_SCOPES.delete(id);
  if (OPEN_MODAL_CHAT_SCOPES.delete(id)) notifyModalStackGenerationChange();
  OPEN_MODAL_STACK.push(id);
  const stableScope = cloneDialogCommandScope(dialogCommandScope);
  if (stableScope) OPEN_MODAL_SCOPES.set(id, stableScope);
  recordStackChange(previousStack, previousScopes);
  syncGlobalModalEffects();
}

/**
 * Atualiza o scope de uma instância já aberta sem alterar a ordem da stack ou
 * reaplicar os efeitos globais. IDs ausentes são ignorados para não reabrir um
 * modal desmontado por uma atualização tardia.
 */
export function updateOpenModalScope(
  id: string,
  dialogCommandScope?: DialogCommandScope,
): boolean {
  if (!OPEN_MODAL_STACK.includes(id)) return false;

  const previousScopes = new Map(OPEN_MODAL_SCOPES);
  OPEN_MODAL_SCOPES.delete(id);
  const stableScope = cloneDialogCommandScope(dialogCommandScope);
  if (stableScope) OPEN_MODAL_SCOPES.set(id, stableScope);
  recordStackChange(OPEN_MODAL_STACK, previousScopes);
  return true;
}

/**
 * Remove um modal (por id de instância) da stack global e reavalia os efeitos
 * globais. Seguro de chamar mesmo que o id não esteja presente.
 */
export function unregisterOpenModal(id: string) {
  const previousStack = getStackSnapshotBeforeSync();
  const previousScopes = new Map(OPEN_MODAL_SCOPES);
  for (let i = OPEN_MODAL_STACK.length - 1; i >= 0; i--) {
    if (OPEN_MODAL_STACK[i] === id) OPEN_MODAL_STACK.splice(i, 1);
  }
  OPEN_MODAL_SCOPES.delete(id);
  if (OPEN_MODAL_CHAT_SCOPES.delete(id)) notifyModalStackGenerationChange();
  recordStackChange(previousStack, previousScopes);
  syncGlobalModalEffects();
}

/**
 * Indica se o modal informado é o que está no topo da stack — único autorizado a
 * tratar Escape/Tab/click-outside quando há múltiplos modais abertos.
 */
export function isTopmostModal(id: string): boolean {
  return OPEN_MODAL_STACK.length > 0 && OPEN_MODAL_STACK[OPEN_MODAL_STACK.length - 1] === id;
}
