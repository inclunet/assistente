import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  CHAT_PICKER_COMMAND_IDS,
  CHAT_PRESENTATION_COMMAND_EVENT,
  requestChatPresentationCommand,
  captureChatPickerTarget,
  isChatPickerCommand,
  registerChatPickerSurface,
  type ChatPickerSurfaceRegistration,
} from './commandChatPickers';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';

const roots: HTMLElement[] = [];
const cleanups: Array<() => void> = [];

function surface(overrides: Partial<ChatPickerSurfaceRegistration> = {}) {
  const root = document.createElement('div');
  root.style.display = 'block';
  document.body.appendChild(root);
  roots.push(root);
  const registration: ChatPickerSurfaceRegistration = {
    root,
    workspaceId: 'workspace-1',
    ownerId: 'owner-1',
    sessionId: 'session-1',
    tabId: 'tab-1',
    conversationId: 'conversation-1',
    instanceId: `instance-${cleanups.length}`,
    generation: 'generation-1',
    allowedCommandIds: CHAT_PICKER_COMMAND_IDS,
    isActive: () => true,
    isCurrent: () => true,
    canOpen: () => true,
    open: vi.fn(() => true),
    ...overrides,
  };
  cleanups.push(registerChatPickerSurface(registration));
  return registration;
}

afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup());
  roots.splice(0).forEach((root) => root.remove());
  document.querySelectorAll('.modal-overlay').forEach((node) => node.remove());
  unregisterOpenModal('chat-modal-a');
  unregisterOpenModal('chat-modal-b');
});

describe('commandChatPickers', () => {
  it.each(CHAT_PICKER_COMMAND_IDS)('solicita %s sem abrir antes da admissão', (commandID) => {
    const registration = surface();
    expect(requestChatPresentationCommand(commandID, registration.instanceId)).toBe(false);
    const listener = vi.fn((event: Event) => {
      expect((event as CustomEvent).detail).toEqual({ commandID, instanceId: registration.instanceId });
      expect(event.cancelable).toBe(true);
      event.preventDefault();
    });
    window.addEventListener(CHAT_PRESENTATION_COMMAND_EVENT, listener);
    try {
      expect(requestChatPresentationCommand(commandID, registration.instanceId)).toBe(true);
      expect(requestChatPresentationCommand(commandID, '')).toBe(false);
      expect(listener).toHaveBeenCalledTimes(1);
      expect(registration.open).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener(CHAT_PRESENTATION_COMMAND_EVENT, listener);
    }
  });

  it('não redireciona request de instância antiga nem relaxa ambiguidade', () => {
    surface({ instanceId: 'new' });
    expect(captureChatPickerTarget(() => '/', 'old')).toBeUndefined();
    expect(captureChatPickerTarget(() => '/', '')).toBeUndefined();
    const lease = captureChatPickerTarget(() => '/', 'new');
    expect(lease).toBeDefined();
    lease?.dispose();
    surface({ instanceId: 'other' });
    expect(captureChatPickerTarget(() => '/', 'new')).toBeUndefined();
  });

  it.each(['chat.pinned.open', 'chat.tokens.open'] as const)('preserva guard, ABA e consumo único para %s', (commandID) => {
    let notify: (() => void) | undefined;
    let allowed = false;
    const unsubscribe = vi.fn();
    const registration = surface({
      canOpen: () => allowed,
      subscribe: (callback) => { notify = callback; return unsubscribe; },
    });
    const lease = captureChatPickerTarget(() => '/', registration.instanceId);
    expect(lease?.open(commandID)).toBe(false);
    expect(registration.open).not.toHaveBeenCalled();
    allowed = true;
    notify?.();
    expect(lease?.canOpen(commandID)).toBe(false);
    expect(lease?.open(commandID)).toBe(false);
    lease?.dispose();
    expect(unsubscribe).toHaveBeenCalledTimes(1);
    const fresh = captureChatPickerTarget(() => '/', registration.instanceId);
    expect(fresh?.open(commandID)).toBe(true);
    expect(fresh?.open(commandID)).toBe(false);
    expect(registration.open).toHaveBeenCalledExactlyOnceWith(commandID);
  });

  it('reconhece somente os cinco comandos de apresentação', () => {
    expect(CHAT_PICKER_COMMAND_IDS).toHaveLength(5);
    expect(isChatPickerCommand('chat.pinned.open')).toBe(true);
    expect(isChatPickerCommand('chat.tokens.open')).toBe(true);
    expect(isChatPickerCommand('chat.conversation.clear')).toBe(false);
    expect(isChatPickerCommand('chat.model.open')).toBe(true);
    expect(isChatPickerCommand('chat.history.open')).toBe(true);
    expect(isChatPickerCommand('chat.profile.open')).toBe(true);
    expect(isChatPickerCommand('decision.respond')).toBe(false);
    expect(isChatPickerCommand('workspace.tab.close')).toBe(false);
  });

  it('captura alvo, aplica guards e abre sem retarget', () => {
    const open = vi.fn(() => true);
    const registration = surface({ open });
    const target = document.createElement('button');
    registration.root.appendChild(target);
    target.focus();

    const lease = captureChatPickerTarget(() => '/');
    expect(lease).toBeDefined();
    expect(lease?.canOpen('chat.model.open', target)).toBe(true);
    expect(lease?.canOpen('decision.respond', target)).toBe(false);
    expect(lease?.open('chat.history.open')).toBe(true);
    expect(open).toHaveBeenCalledWith('chat.history.open');

    registration.root.remove();
    expect(lease?.isCurrent()).toBe(false);
    expect(lease?.open('chat.profile.open')).toBe(false);
  });

  it('falha fechado quando há duas superfícies ativas', () => {
    surface({ instanceId: 'instance-a' });
    surface({ instanceId: 'instance-b' });
    expect(captureChatPickerTarget(() => '/')).toBeUndefined();
  });

  it('não mantém superfície keep-alive fora da rota workspace', () => {
    surface({ isRouteCurrent: (pathname) => pathname === '/' });
    expect(captureChatPickerTarget(() => '/settings')).toBeUndefined();
  });

  it('invalida lease quando ancestor fica oculto', () => {
    const registration = surface();
    const lease = captureChatPickerTarget(() => '/');
    expect(lease?.isCurrent()).toBe(true);
    const wrapper = document.createElement('section');
    registration.root.replaceWith(wrapper);
    wrapper.appendChild(registration.root);
    wrapper.setAttribute('aria-hidden', 'true');
    expect(lease?.isCurrent()).toBe(false);
  });

  it('aceita somente chat picker explicitamente permitido no modal topmost', () => {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.setAttribute('data-modal-id', 'chat-modal-a');
    document.body.appendChild(overlay);
    registerOpenModal('chat-modal-a');
    const registration = surface({
      modalId: 'chat-modal-a',
      allowedCommandIds: ['chat.history.open'],
    });
    overlay.appendChild(registration.root);

    const lease = captureChatPickerTarget(() => '/settings');
    expect(lease?.canOpen('chat.history.open')).toBe(true);
    expect(lease?.canOpen('chat.model.open')).toBe(false);
    expect(lease?.open('chat.model.open')).toBe(false);
    expect(lease?.open('chat.history.open')).toBe(true);
    expect(registration.open).toHaveBeenCalledWith('chat.history.open');
  });

  it('invalida lease quando o stack modal muda, mesmo retornando ao mesmo id', () => {
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.setAttribute('data-modal-id', 'chat-modal-a');
    document.body.appendChild(overlay);
    registerOpenModal('chat-modal-a');
    const registration = surface({ modalId: 'chat-modal-a' });
    overlay.appendChild(registration.root);
    const lease = captureChatPickerTarget(() => '/settings');
    unregisterOpenModal('chat-modal-a');
    registerOpenModal('chat-modal-a');
    expect(lease?.isCurrent()).toBe(false);
  });

  it('falha fechado quando callbacks de identidade, guard ou abertura lançam', () => {
    surface({
      isCurrent: () => { throw new Error('stale'); },
    });
    const lease = captureChatPickerTarget(() => '/');
    expect(lease).toBeUndefined();

    const guardLease = (() => {
      cleanups.splice(-1, 1)[0]?.();
    surface({ canOpen: () => { throw new Error('guard'); } });
    return captureChatPickerTarget(() => '/');
  })();
    expect(guardLease?.canOpen('chat.model.open')).toBe(false);
    expect(guardLease?.open('chat.model.open')).toBe(false);

    cleanups.splice(-1, 1)[0]?.();
    surface({ open: () => { throw new Error('open'); } });
    const openLease = captureChatPickerTarget(() => '/');
    expect(openLease?.open('chat.model.open')).toBe(false);
    expect(openLease?.isCurrent()).toBe(false);
  });

  it('consome a lease somente após open, uma única vez', () => {
    const open = vi.fn(() => true);
    surface({ open });
    const lease = captureChatPickerTarget(() => '/');
    expect(lease?.canOpen('chat.model.open')).toBe(true);
    expect(lease?.canOpen('chat.model.open')).toBe(true);
    expect(lease?.open('chat.model.open')).toBe(true);
    expect(lease?.open('chat.model.open')).toBe(false);
    expect(lease?.isCurrent()).toBe(false);
    expect(open).toHaveBeenCalledTimes(1);
  });
});
