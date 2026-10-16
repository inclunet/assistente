import { describe, expect, it, vi } from 'vitest';
import {
  COMMAND_NAVIGATION_EVENT,
  COMMAND_NAVIGATION_ROUTES,
  type CommandNavigationID,
  createCommandNavigationHandlers,
  isCommandNavigation,
  isCommandNavigationTextField,
  isWorkspaceTabNavigationTextField,
  requestCommandNavigation,
} from './commandNavigation';

describe('commandNavigation', () => {
  it('mantém somente a allowlist de rotas públicas', () => {
    expect(COMMAND_NAVIGATION_ROUTES).toEqual({
      'navigation.workspace.open': '/',
      'navigation.history.open': '/history',
      'navigation.memories.open': '/memories',
      'navigation.tasklists.open': '/tasklists',
      'navigation.jobs.open': '/jobs',
      'navigation.profiles.open': '/profiles',
      'navigation.settings.open': '/settings',
      'navigation.data.export.open': '/settings/data?action=export',
      'navigation.data.import.open': '/settings/data?action=import',
      'navigation.help.open': '/help',
      'navigation.about.open': '/about',
    });
  });

  it('cria handlers síncronos que navegam para o destino exato', () => {
    const navigate = vi.fn();
    const handlers = createCommandNavigationHandlers(navigate);

    for (const [commandID, path] of Object.entries(COMMAND_NAVIGATION_ROUTES)) {
      expect(handlers.has(commandID)).toBe(true);
      const result = handlers.get(commandID)?.();
      expect(result).toBeUndefined();
      expect(navigate).toHaveBeenLastCalledWith(path);
    }
  });

  it('recusa IDs fora da allowlist', () => {
    expect(createCommandNavigationHandlers(vi.fn()).has('navigation.unknown.open')).toBe(false);
    expect(createCommandNavigationHandlers(vi.fn()).has('__proto__')).toBe(false);
  });

  it('emite a solicitação fechada e só confirma quando o dispatcher consome', () => {
    const listener = vi.fn((event: Event) => {
      expect((event as CustomEvent).detail).toEqual({ commandId: 'navigation.workspace.open' });
      expect(event.cancelable).toBe(true);
      event.preventDefault();
    });
    window.addEventListener(COMMAND_NAVIGATION_EVENT, listener);
    try {
      expect(requestCommandNavigation('navigation.workspace.open')).toBe(true);
      expect(listener).toHaveBeenCalledOnce();
    } finally {
      window.removeEventListener(COMMAND_NAVIGATION_EVENT, listener);
    }
  });

  it('falha fechado sem dispatcher ou para ID que não é rota', () => {
    expect(requestCommandNavigation('navigation.workspace.open')).toBe(false);
    expect(requestCommandNavigation('navigation.palette.open' as CommandNavigationID)).toBe(false);
  });

  it('mantém a paleta como navegação local sem handler de rota estática', () => {
    expect(isCommandNavigation('navigation.palette.open')).toBe(true);
    expect(COMMAND_NAVIGATION_ROUTES).not.toHaveProperty('navigation.palette.open');
    expect(createCommandNavigationHandlers(vi.fn()).has('navigation.palette.open')).toBe(false);
  });

  it('permite navegação em input/textarea, mas protege Monaco e contenteditable', () => {
    const input = document.body.appendChild(document.createElement('input'));
    const textarea = document.body.appendChild(document.createElement('textarea'));
    const monaco = document.body.appendChild(document.createElement('div'));
    monaco.className = 'monaco-editor';
    const contenteditable = document.body.appendChild(document.createElement('div'));
    contenteditable.setAttribute('contenteditable', 'true');

    expect(isCommandNavigationTextField(input)).toBe(true);
    expect(isCommandNavigationTextField(textarea)).toBe(true);
    expect(isCommandNavigationTextField(monaco)).toBe(false);
    expect(isCommandNavigationTextField(contenteditable)).toBe(false);

    input.remove();
    textarea.remove();
    monaco.remove();
    contenteditable.remove();
  });

  it('permite Monaco apenas para IDs conhecidos de navegação entre abas locais', () => {
    const monaco = document.body.appendChild(document.createElement('div'));
    monaco.className = 'monaco-editor';
    const inputarea = monaco.appendChild(document.createElement('textarea'));
    const nativeEditContext = monaco.appendChild(document.createElement('div'));
    nativeEditContext.className = 'native-edit-context';
    nativeEditContext.setAttribute('contenteditable', 'true');
    const rich = document.body.appendChild(document.createElement('div'));
    rich.className = 'rich-text-editor';
    rich.setAttribute('contenteditable', 'true');
    const richInput = rich.appendChild(document.createElement('textarea'));

    expect(isWorkspaceTabNavigationTextField('workspace.tab.next', inputarea)).toBe(true);
    expect(isWorkspaceTabNavigationTextField('workspace.tab.previous', inputarea)).toBe(true);
    expect(isWorkspaceTabNavigationTextField('workspace.tab.previous', nativeEditContext)).toBe(true);
    expect(isWorkspaceTabNavigationTextField('workspace.tab.close', inputarea)).toBe(false);
    expect(isWorkspaceTabNavigationTextField('navigation.workspace.open', inputarea)).toBe(false);
    expect(isWorkspaceTabNavigationTextField('workspace.tab.next', richInput)).toBe(false);
    expect(isWorkspaceTabNavigationTextField('workspace.tab.next', monaco)).toBe(false);

    monaco.remove();
    rich.remove();
  });
});
