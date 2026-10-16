import { afterEach, describe, expect, it, vi } from 'vitest';
import { ReadFocusContext, registerSurfaceContext } from './commandContextProviders';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';

const stores = vi.hoisted(() => {
  const authListeners = new Set<() => void>();
  const workspaceListeners = new Set<() => void>();
  return {
    auth: {
      isAuthenticated: true,
      user: { userId: 'user-a', sessionId: 'session-a', role: 'user' },
    },
    workspace: {
      workspace: {
        id: 'workspace-a',
        name: 'A',
        profile: 'profile-a',
        tabs: [
          { id: 'tab-a', type: 'editor', title: 'A', position: 0 },
          { id: 'tab-b', type: 'editor', title: 'B', position: 1 },
        ],
        activeTabId: 'tab-a',
      },
    },
    authListeners,
    workspaceListeners,
    notify(listeners: Set<() => void>) {
      listeners.forEach((listener) => listener());
    },
  };
});

vi.mock('../store/authStore', () => ({
  useAuthStore: {
    getState: () => stores.auth,
    subscribe: (listener: () => void) => {
      stores.authListeners.add(listener);
      return () => stores.authListeners.delete(listener);
    },
  },
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => stores.workspace,
    subscribe: (listener: () => void) => {
      stores.workspaceListeners.add(listener);
      return () => stores.workspaceListeners.delete(listener);
    },
  },
}));

import { createTrustedCommandContextSession } from './commandContextSession';
import {
  createCommandUIEffectGuard,
  type CommandUIEffect,
  type CommandUIEffectToken,
} from './commandUIEffect';

let disposeGuard: (() => void) | undefined;
let disposeSurface: (() => void) | undefined;

afterEach(() => {
  disposeGuard?.();
  disposeGuard = undefined;
  disposeSurface?.();
  disposeSurface = undefined;
  stores.auth.isAuthenticated = true;
  stores.auth.user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
  stores.workspace.workspace = {
    id: 'workspace-a',
    name: 'A',
    profile: 'profile-a',
    tabs: [
      { id: 'tab-a', type: 'editor', title: 'A', position: 0 },
      { id: 'tab-b', type: 'editor', title: 'B', position: 1 },
    ],
    activeTabId: 'tab-a',
  };
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

function focusButton(disabled = false): HTMLButtonElement {
  const button = document.createElement('button');
  button.disabled = disabled;
  document.body.appendChild(button);
  button.focus();
  return button;
}

function registerSurface(): void {
  disposeSurface = registerSurfaceContext('surface-a', () => ({
    surfaceType: 'editor',
    surfaceId: 'surface-a',
    snapshotVersion: 'surface-a-v1',
  }));
}

describe('command UI effect guard', () => {
  it.each(['workspace.panel.focus', 'workspace.tab.chat.create', 'workspace.tab.editor.create', 'workspace.tab.tasklist.create', 'workspace.tab.close', 'navigation.future.open', undefined])('mantém unknown bloqueado para %s', (commandID) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    const input = document.createElement('textarea');
    document.body.appendChild(input);
    input.focus();
    expect(guard.capture(undefined, commandID)).toBeUndefined();
  });

  it('permite unknown em campo nativo somente pela finalidade explícita de paleta', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    const token = guard.capturePalette();
    const effect = vi.fn();
    expect(token).toBeDefined();
    expect(guard.commit(token!, effect)).toBe(true);
    expect(effect).toHaveBeenCalledTimes(1);
  });

  it.each(['monaco', 'contenteditable'].flatMap(surface =>
    ['workspace.chat.open', 'editor.mode.markdown', 'editor.mode.rich', 'editor.mode.view', 'editor.mermaid.open', 'editor.mermaid.apply', 'editor.mermaid.remove'].map(commandID => ({ surface, commandID }))))(
    'permite unknown em $surface para comando contextual preparado $commandID',
    ({ surface, commandID }) => {
      vi.spyOn(document, 'hasFocus').mockReturnValue(true);
      const guard = createCommandUIEffectGuard();
      disposeGuard = guard.dispose;
      const editor = document.createElement('div');
      if (surface === 'monaco') editor.className = 'monaco-editor';
      else editor.contentEditable = 'true';
      editor.tabIndex = -1;
      document.body.appendChild(editor);
      editor.focus();

      const token = guard.capture(undefined, commandID);
      const effect = vi.fn(() => undefined);
      expect(token).toBeDefined();
      expect(guard.commit(token!, effect)).toBe(true);
      expect(effect).toHaveBeenCalledOnce();
    },
  );

  it.each(['workspace.chat.open', 'editor.mermaid.open', 'editor.mermaid.apply', 'editor.mermaid.remove'])('bloqueia unknown de %s quando há modal aberto', (commandID) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    const editor = document.createElement('div');
    editor.className = 'monaco-editor';
    editor.tabIndex = -1;
    document.body.appendChild(editor);
    editor.focus();
    const token = guard.capture(undefined, commandID);
    expect(token).toBeDefined();
    registerOpenModal('chat-unknown-modal');
    try {
      expect(guard.commit(token!, vi.fn(() => undefined))).toBe(false);
    } finally {
      unregisterOpenModal('chat-unknown-modal');
    }
  });

  it('início de composição entre captura e commit invalida navegação', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    const input = document.createElement('textarea');
    document.body.appendChild(input);
    input.focus();
    const token = guard.capture(undefined, 'navigation.settings.open');
    expect(token).toBeDefined();
    input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    const effect = vi.fn();
    expect(guard.commit(token!, effect)).toBe(false);
    expect(effect).not.toHaveBeenCalled();
  });

  it('commits uma vez após revalidar owner, foco, modal, surface e profile', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    focusButton();
    registerSurface();
    const session = createTrustedCommandContextSession();
    const unregister = session.registerSurfaceContext('surface-a', () => ({
      surfaceType: 'editor', surfaceId: 'surface-a', snapshotVersion: 'surface-a-v1',
    }));
    const guard = createCommandUIEffectGuard(session);
    disposeGuard = guard.dispose;
    const token = guard.capture('surface-a');
    const effect = vi.fn();

    expect(token).toBeDefined();
    expect(guard.commit(token!, effect)).toBe(true);
    expect(effect).toHaveBeenCalledTimes(1);
    expect(guard.commit(token!, effect)).toBe(false);
    unregister();
    session.dispose();
  });

  it('não reutiliza token após replacement da superfície com mesmo snapshot', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    focusButton();
    const session = createTrustedCommandContextSession();
    const oldCleanup = session.registerSurfaceContext('surface-a', () => ({
      surfaceType: 'editor', surfaceId: 'surface-a', snapshotVersion: 'surface-a-v1',
    }));
    const guard = createCommandUIEffectGuard(session);
    disposeGuard = guard.dispose;
    const token = guard.capture('surface-a');
    expect(token).toBeDefined();

    const newCleanup = session.registerSurfaceContext('surface-a', () => ({
      surfaceType: 'editor', surfaceId: 'surface-a', snapshotVersion: 'surface-a-v1',
    }));
    oldCleanup();
    expect(guard.commit(token!, vi.fn())).toBe(false);

    const stableToken = guard.capture('surface-a');
    const effect = vi.fn();
    expect(stableToken).toBeDefined();
    expect(guard.commit(stableToken!, effect)).toBe(true);
    expect(effect).toHaveBeenCalledTimes(1);
    newCleanup();
    session.dispose();
  });

  it('não aceita token forjado, superfície ausente ou efeito assíncrono', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    focusButton();
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    const effect = vi.fn();

    expect(guard.capture('missing-surface')).toBeUndefined();
    const token = guard.capture();
    expect(token).toBeDefined();
    expect(guard.commit({} as CommandUIEffectToken, effect)).toBe(false);

    const asyncToken = guard.capture();
    let asyncCalls = 0;
    // @ts-expect-error efeitos do guard são síncronos por contrato.
    const asyncEffect: CommandUIEffect = async () => { asyncCalls += 1; };
    expect(guard.commit(asyncToken!, asyncEffect)).toBe(false);
    expect(asyncCalls).toBe(0);
  });

  it.each([
    ['sem foco', () => vi.spyOn(document, 'hasFocus').mockReturnValue(false)],
    ['controle disabled', () => {
      vi.spyOn(document, 'hasFocus').mockReturnValue(true);
      focusButton(true);
    }],
    ['IME desconhecido', () => {
      vi.spyOn(document, 'hasFocus').mockReturnValue(true);
      const input = document.createElement('input');
      document.body.appendChild(input);
      input.focus();
    }],
    ['IME ativo', () => {
      vi.spyOn(document, 'hasFocus').mockReturnValue(true);
      const input = document.createElement('input');
      document.body.appendChild(input);
      input.focus();
    }],
  ])('recusa captura %s', (reason, setup) => {
    setup();
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    if (reason === 'IME ativo') {
      document.activeElement?.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
      expect(ReadFocusContext().composition).toBe('active');
    }
    expect(guard.capture()).toBeUndefined();
  });

  it('recusa rebind/logout e troca de aba mesmo quando a notificação é perdida', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    focusButton();
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    const effect = vi.fn();

    let token = guard.capture();
    stores.workspace.workspace = { ...stores.workspace.workspace, activeTabId: 'tab-b' };
    expect(guard.commit(token!, effect)).toBe(false);

    token = guard.capture();
    stores.workspace.workspace = { ...stores.workspace.workspace, profile: 'profile-b' };
    expect(guard.commit(token!, effect)).toBe(false);

    token = guard.capture();
    stores.auth.user = { userId: 'user-b', sessionId: 'session-b', role: 'user' };
    // Sem notify: a comparação síncrona da fonte ainda precisa recusar.
    expect(guard.commit(token!, effect)).toBe(false);

    token = guard.capture();
    stores.auth.isAuthenticated = false;
    stores.notify(stores.authListeners);
    expect(guard.commit(token!, effect)).toBe(false);
    expect(effect).not.toHaveBeenCalled();
  });

  it('recusa mutação de store notificada durante a leitura síncrona do provider', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    focusButton();
    const session = createTrustedCommandContextSession();
    let getterCalls = 0;
    disposeSurface = session.registerSurfaceContext('surface-a', () => {
      getterCalls += 1;
      stores.workspace.workspace = { ...stores.workspace.workspace, profile: 'profile-reentrant' };
      stores.notify(stores.workspaceListeners);
      return { surfaceType: 'editor', surfaceId: 'surface-a', snapshotVersion: 'surface-a-v1' };
    });
    const guard = createCommandUIEffectGuard(session);
    disposeGuard = guard.dispose;

    expect(guard.capture('surface-a')).toBeUndefined();
    expect(getterCalls).toBe(1);
    session.dispose();
  });

  it('não aceita token de outra instância e invalida logout/login ABA observado', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    focusButton();
    const guard = createCommandUIEffectGuard();
    const other = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    try {
      const token = guard.capture();
      const effect = vi.fn();
      expect(token).toBeDefined();
      expect(other.commit(token!, effect)).toBe(false);
      // A instância estrangeira não pode consumir o token do dono.
      expect(guard.commit(token!, effect)).toBe(true);
      expect(effect).toHaveBeenCalledTimes(1);

      const beforeLogout = guard.capture();
      expect(beforeLogout).toBeDefined();
      stores.auth.isAuthenticated = false;
      stores.notify(stores.authListeners);
      stores.auth.isAuthenticated = true;
      stores.notify(stores.authListeners);
      expect(guard.commit(beforeLogout!, effect)).toBe(false);
      expect(effect).toHaveBeenCalledTimes(1);
    } finally {
      other.dispose();
      guard.dispose();
    }
    expect(stores.authListeners.size).toBe(0);
    expect(stores.workspaceListeners.size).toBe(0);
  });

  it('recusa troca de aba sem notify mesmo quando não há profile efetivo', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    focusButton();
    stores.workspace.workspace = {
      ...stores.workspace.workspace,
      profile: '',
      activeTabId: 'tab-a',
      tabs: [
        { id: 'tab-a', type: 'editor', title: 'A', position: 0 },
        { id: 'tab-b', type: 'editor', title: 'B', position: 1 },
      ],
    };
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    const token = guard.capture();
    stores.workspace.workspace = { ...stores.workspace.workspace, activeTabId: 'tab-b' };

    // Profile continua null nas duas abas; activeTabId é uma fonte separada.
    expect(token).toBeDefined();
    expect(guard.commit(token!, vi.fn())).toBe(false);
  });

  it('não captura efeito em novo editable após troca de activeElement sem eventos', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const first = document.createElement('input');
    const second = document.createElement('input');
    document.body.append(first, second);
    first.focus();
    const session = createTrustedCommandContextSession();
    const guard = createCommandUIEffectGuard(session);
    disposeGuard = guard.dispose;

    first.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    first.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    expect(ReadFocusContext().composition).toBe('inactive');

    const originalActiveElement = Object.getOwnPropertyDescriptor(document, 'activeElement');
    Object.defineProperty(document, 'activeElement', {
      configurable: true,
      get: () => second,
    });
    try {
      expect(ReadFocusContext().composition).toBe('unknown');
      expect(guard.capture()).toBeUndefined();
    } finally {
      if (originalActiveElement) {
        Object.defineProperty(document, 'activeElement', originalActiveElement);
      } else {
        delete (document as unknown as Record<string, unknown>).activeElement;
      }
      session.dispose();
    }
  });

  it('recusa mutação do getter na segunda leitura, durante commit', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    focusButton();
    const session = createTrustedCommandContextSession();
    let getterCalls = 0;
    const unregister = session.registerSurfaceContext('surface-a', () => {
      getterCalls += 1;
      if (getterCalls === 2) {
        stores.workspace.workspace = { ...stores.workspace.workspace, profile: 'profile-during-commit' };
        stores.notify(stores.workspaceListeners);
      }
      return { surfaceType: 'editor', surfaceId: 'surface-a', snapshotVersion: 'surface-a-v1' };
    });
    const guard = createCommandUIEffectGuard(session);
    disposeGuard = guard.dispose;
    const token = guard.capture('surface-a');
    expect(token).toBeDefined();
    expect(guard.commit(token!, vi.fn())).toBe(false);
    expect(getterCalls).toBe(2);
    unregister();
    session.dispose();
  });

  it('invalidates tokens on modal/focus change and dispose, sem executar o efeito', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const first = focusButton();
    const second = document.createElement('button');
    document.body.appendChild(second);
    const guard = createCommandUIEffectGuard();
    disposeGuard = guard.dispose;
    const effect = vi.fn();

    const focusToken = guard.capture();
    expect(focusToken).toBeDefined();
    second.focus();
    first.focus();
    // A descrição e o activeElement voltaram ao mesmo valor: a época local
    // precisa preservar a prova de que houve a passagem por outra aba/foco.
    expect(guard.commit(focusToken!, effect)).toBe(false);

    const disposeToken = guard.capture();
    expect(disposeToken).toBeDefined();
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.appendChild(overlay);
    registerOpenModal('guard-modal');
    const modalToken = guard.capture();
    expect(modalToken).toBeDefined();
    unregisterOpenModal('guard-modal');
    overlay.remove();
    expect(guard.commit(modalToken!, effect)).toBe(false);

    guard.dispose();
    disposeGuard = undefined;
    expect(guard.commit(disposeToken!, effect)).toBe(false);
    expect(effect).not.toHaveBeenCalled();
  });
});
