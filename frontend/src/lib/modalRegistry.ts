// Registro NEUTRO (sem dependência de React/UI) do conjunto de modais abertos.
//
// Mantém a stack global de modais e os efeitos globais associados (inert/aria-hidden
// no `#root` e overflow do body). É consumido tanto pelo componente `Modal` quanto por
// stores/hooks que só precisam saber se há um modal aberto. Centralizar essa lógica
// aqui evita a inversão de camadas em que a camada de estado (stores Zustand) dependia
// de um componente React (`components/ui/Modal.tsx`), e a porta aberta para imports
// circulares que isso representava.

// Stack global simples para garantir que apenas o modal do topo
// trate Escape/Tab/click-outside quando há múltiplos modais abertos.
const OPEN_MODAL_STACK: string[] = [];

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
  // Safety net: se a stack diz que há modais abertos, mas nenhum overlay
  // está no DOM, a stack ficou dessincronizada (ex: erro de render ou
  // unmount inesperado). Limpa a stack para restaurar a interatividade.
  if (OPEN_MODAL_STACK.length > 0) {
    const actualOverlays = document.querySelectorAll('.modal-overlay').length;
    if (actualOverlays === 0) {
      OPEN_MODAL_STACK.length = 0;
    }
  }
  setGlobalModalEffects(OPEN_MODAL_STACK.length > 0);
}

export function isModalOpen(): boolean {
  return OPEN_MODAL_STACK.length > 0;
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
export function registerOpenModal(id: string) {
  for (let i = OPEN_MODAL_STACK.length - 1; i >= 0; i--) {
    if (OPEN_MODAL_STACK[i] === id) OPEN_MODAL_STACK.splice(i, 1);
  }
  OPEN_MODAL_STACK.push(id);
  syncGlobalModalEffects();
}

/**
 * Remove um modal (por id de instância) da stack global e reavalia os efeitos
 * globais. Seguro de chamar mesmo que o id não esteja presente.
 */
export function unregisterOpenModal(id: string) {
  for (let i = OPEN_MODAL_STACK.length - 1; i >= 0; i--) {
    if (OPEN_MODAL_STACK[i] === id) OPEN_MODAL_STACK.splice(i, 1);
  }
  syncGlobalModalEffects();
}

/**
 * Indica se o modal informado é o que está no topo da stack — único autorizado a
 * tratar Escape/Tab/click-outside quando há múltiplos modais abertos.
 */
export function isTopmostModal(id: string): boolean {
  return OPEN_MODAL_STACK.length > 0 && OPEN_MODAL_STACK[OPEN_MODAL_STACK.length - 1] === id;
}
