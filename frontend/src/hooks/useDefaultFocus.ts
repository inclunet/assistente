/**
 * Global registry for the current page's "default focus area".
 *
 * Each page registers a focus function when it mounts (via useLandmarkNavigation
 * with `defaultLandmarkId`) and unregisters on unmount.  Any component — Modal,
 * Menu, toast, etc. — can call `restoreDefaultFocus()` to send focus back to the
 * page's primary interaction zone (e.g. the editor, the chat input, the data grid).
 *
 * Implementado como uma PILHA (stack), no mesmo espírito do `OPEN_MODAL_STACK`
 * do `Modal`: o registrante mais recente é o "default" corrente, e ao
 * desregistrar o slot volta para o registrante anterior em vez de virar `null`.
 *
 * Motivação (issue #205): havia um único slot last-writer-wins. Quando um painel
 * com foco próprio (ex.: `TaskListView`) ficava ativo, ele sobrescrevia o
 * registro do `WorkspaceLayout`; ao sair desse painel, o cleanup zerava o slot e
 * a Content Area inteligente (que foca o input do chat) nunca era reinvocada. Com
 * a pilha, o `WorkspaceLayout` permanece no fundo e reassume o default assim que
 * o painel especializado é desativado.
 */

const defaultFocusStack: Array<() => boolean> = [];

export function registerDefaultFocus(fn: () => boolean) {
  // Remove qualquer entrada antiga (best-effort) e empilha no topo, para que
  // re-registros da mesma função não dupliquem nem fiquem soterrados.
  for (let i = defaultFocusStack.length - 1; i >= 0; i--) {
    if (defaultFocusStack[i] === fn) defaultFocusStack.splice(i, 1);
  }
  defaultFocusStack.push(fn);
}

export function unregisterDefaultFocus(fn: () => boolean) {
  for (let i = defaultFocusStack.length - 1; i >= 0; i--) {
    if (defaultFocusStack[i] === fn) defaultFocusStack.splice(i, 1);
  }
}

/**
 * Attempts to restore focus to the current page's default area.
 * Returns true if focus was successfully restored.
 */
export function restoreDefaultFocus(): boolean {
  const fn = defaultFocusStack[defaultFocusStack.length - 1];
  if (!fn) return false;
  return fn();
}
