import { useAuthStore } from '../store/authStore';
import { isCommandNavigation, isCommandNavigationTextField } from './commandNavigation';
import { isEditorModeCommand } from './commandEditorMode';
import { isEditorFileCommand } from './commandEditorFile';
import { isEditorFormatCommand } from './commandEditorFormatting';
import { isEditorMermaidCommand } from './commandEditorMermaid';
import { useWorkspaceStore } from '../store/workspaceStore';
import {
  createTrustedCommandContextSession,
  type OwnedCommandContextFrame,
  type TrustedCommandContextSession,
  type TrustedOwner,
} from './commandContextSession';

/** Token local de uma única utilização; não é credencial nem prova de backend. */
export type CommandUIEffectToken = object;
export type CommandUIEffect = () => undefined;
export type CommandUIEffectPurpose = 'default' | 'palette';
const WORKSPACE_CHAT_OPEN_COMMAND_ID = 'workspace.chat.open';

export interface CommandUIEffectGuard {
  capture(surfaceID?: string, commandID?: string): CommandUIEffectToken | undefined;
  capturePalette(surfaceID?: string): CommandUIEffectToken | undefined;
  commit(token: CommandUIEffectToken, effect: CommandUIEffect): boolean;
  dispose(): void;
}

interface CapturedEffect {
  readonly commandID?: string;
  readonly purpose: CommandUIEffectPurpose;
  readonly owner: TrustedOwner;
  readonly frame: OwnedCommandContextFrame['frame'];
  readonly surfaceLease: object | null;
  readonly documentRef: Document;
  readonly activeElement: Element;
  readonly activeTabID: string | null;
  readonly epoch: number;
  readonly surfaceID?: string;
}

function sameValue(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  if (left === null || right === null || typeof left !== 'object' || typeof right !== 'object') {
    return false;
  }
  if (Array.isArray(left) !== Array.isArray(right)) return false;
  const leftObject = left as Record<string, unknown>;
  const rightObject = right as Record<string, unknown>;
  const leftKeys = Object.keys(leftObject).sort();
  const rightKeys = Object.keys(rightObject).sort();
  return leftKeys.length === rightKeys.length && leftKeys.every((key, index) =>
    key === rightKeys[index] && sameValue(leftObject[key], rightObject[key]),
  );
}

function sameOwner(left: TrustedOwner, right: TrustedOwner): boolean {
  return left.userId === right.userId && left.sessionId === right.sessionId && left.workspaceId === right.workspaceId;
}

function usableFocus(
  frame: OwnedCommandContextFrame['frame'],
  commandID?: string,
  purpose: CommandUIEffectPurpose = 'default',
): boolean {
  const focus = frame.focus;
  // Unknown não vira inactive: apenas navegação explícita independe dessa
  // prova. Composição observada ativa continua bloqueada nas duas origens.
  const editorMode = commandID !== undefined && (isEditorModeCommand(commandID) || isEditorFileCommand(commandID) || isEditorFormatCommand(commandID) || isEditorMermaidCommand(commandID));
  const chatFromRichEditable = (commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID || editorMode) &&
    isRichEditableTextField(currentActiveElement());
  const textSurfaceException = (commandID !== undefined && isCommandNavigation(commandID)) ||
    purpose === 'palette' || commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID || editorMode;
  const navigationFromText = textSurfaceException &&
    (isCommandNavigationTextField(currentActiveElement()) || chatFromRichEditable) &&
    focus.composition === 'unknown';
  return focus.hasFocus && !focus.detached && focus.control !== null &&
    !focus.control.capabilities.disabled && (focus.composition === 'inactive' || navigationFromText);
}

function isRichEditableTextField(target: Element | undefined): boolean {
  return Boolean(target && target.closest('.monaco-editor, [contenteditable="true"]'));
}

function usableFrame(
  frame: OwnedCommandContextFrame['frame'],
  surfaceID?: string,
  commandID?: string,
  purpose: CommandUIEffectPurpose = 'default',
): boolean {
  if (!usableFocus(frame, commandID, purpose)) return false;
  // Uma superfície pedida precisa existir agora; nunca inferimos a aba pelo
  // workspace quando o provider não entregou sua leitura.
  return surfaceID === undefined || frame.surface !== null;
}

function currentActiveElement(): Element | undefined {
  if (typeof document === 'undefined') return undefined;
  const active = document.activeElement;
  return active && typeof Element !== 'undefined' && active instanceof Element ? active : undefined;
}

function currentActiveTabID(): string | null {
  return useWorkspaceStore.getState().workspace?.activeTabId ?? null;
}

function sameFrame(left: CapturedEffect, right: OwnedCommandContextFrame): boolean {
  return sameOwner(left.owner, right.owner) &&
    left.surfaceLease === right.surfaceLease &&
    left.frame.version === right.frame.version &&
    sameValue(left.frame.modal, right.frame.modal) &&
    sameValue(left.frame.focus, right.frame.focus) &&
    sameValue(left.frame.surface, right.frame.surface) &&
    sameValue(left.frame.profile, right.frame.profile);
}

function isAsyncFunction(effect: CommandUIEffect): boolean {
  // Isto só protege callers acidentais. Um JS malicioso pode retornar
  // thenable/agendar trabalho sem ser uma AsyncFunction; o guard não é uma
  // fronteira de segurança e o contrato depende dos callers internos.
  return effect.constructor?.name === 'AsyncFunction';
}

/**
 * Guarda efeitos puramente visuais contra a mudança síncrona das fontes da UI.
 * A revalidação é uma prova local de contexto; autorização, ledger e versões
 * autoritativas continuam exclusivamente no backend.
 *
 * Quando `trustedSession` é fornecida, ela é uma sessão emprestada: o guard
 * compartilha seus registrations e nunca a encerra. O dono deve chamar
 * `trustedSession.dispose()` depois de destruir o guard. Sem argumento, o
 * guard cria e encerra sua própria sessão.
 */
export function createCommandUIEffectGuard(
  trustedSession?: TrustedCommandContextSession,
): CommandUIEffectGuard {
  const ownsSession = trustedSession === undefined;
  const session = trustedSession ?? createTrustedCommandContextSession();
  const tokens = new WeakMap<object, CapturedEffect>();
  let epoch = 0;
  let disposed = false;

  const unsubscribeAuth = useAuthStore.subscribe(() => { epoch += 1; });
  const unsubscribeWorkspace = useWorkspaceStore.subscribe(() => { epoch += 1; });
  const invalidateFocusEpoch = () => { epoch += 1; };
  if (typeof document !== 'undefined') {
    document.addEventListener('focusin', invalidateFocusEpoch, true);
    document.addEventListener('focusout', invalidateFocusEpoch, true);
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('blur', invalidateFocusEpoch, true);
  }

  function captureWithPurpose(
    surfaceID: string | undefined,
    commandID: string | undefined,
    purpose: CommandUIEffectPurpose,
  ): CommandUIEffectToken | undefined {
    if (disposed) return undefined;
    const captureEpoch = epoch;
    const owned = session.readOwnedCommandContextFrame(surfaceID);
    const activeElement = currentActiveElement();
    const activeTabID = currentActiveTabID();
    if (captureEpoch !== epoch || !owned || !usableFrame(owned.frame, surfaceID, commandID, purpose) ||
      !activeElement || owned.frame.focus.control === null) return undefined;
    const token = Object.freeze({});
    tokens.set(token, {
      commandID,
      purpose,
      owner: Object.freeze({ ...owned.owner }),
      frame: owned.frame,
      surfaceLease: owned.surfaceLease,
      documentRef: document,
      activeElement,
      activeTabID,
      epoch,
      surfaceID,
    });
    return token;
  }

  function capture(surfaceID?: string, commandID?: string): CommandUIEffectToken | undefined {
    return captureWithPurpose(surfaceID, commandID, 'default');
  }

  function capturePalette(surfaceID?: string): CommandUIEffectToken | undefined {
    return captureWithPurpose(surfaceID, undefined, 'palette');
  }

  function commit(token: CommandUIEffectToken, effect: CommandUIEffect): boolean {
    if (disposed || typeof token !== 'object' || token === null || typeof effect !== 'function') {
      return false;
    }
    const captured = tokens.get(token);
    tokens.delete(token);
    if (!captured || captured.epoch !== epoch || isAsyncFunction(effect)) return false;

    const readEpoch = epoch;
    const current = session.readOwnedCommandContextFrame(captured.surfaceID);
    const activeElement = currentActiveElement();
    const activeTabID = currentActiveTabID();
    if (readEpoch !== epoch || !current || !usableFrame(current.frame, captured.surfaceID, captured.commandID, captured.purpose) ||
      activeElement !== captured.activeElement || document !== captured.documentRef ||
      activeTabID !== captured.activeTabID ||
      !sameFrame(captured, current)) {
      return false;
    }
    effect();
    return true;
  }

  function dispose(): void {
    if (disposed) return;
    disposed = true;
    epoch += 1;
    unsubscribeAuth();
    unsubscribeWorkspace();
    if (typeof document !== 'undefined') {
      document.removeEventListener('focusin', invalidateFocusEpoch, true);
      document.removeEventListener('focusout', invalidateFocusEpoch, true);
    }
    if (typeof window !== 'undefined') {
      window.removeEventListener('blur', invalidateFocusEpoch, true);
    }
    if (ownsSession) session.dispose();
  }

  return Object.freeze({ capture, capturePalette, commit, dispose });
}
