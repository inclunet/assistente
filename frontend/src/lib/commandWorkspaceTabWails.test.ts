import { describe, expect, it, vi } from 'vitest';
import { createCommandWorkspaceTabWailsPort } from './commandWorkspaceTabWails';
import { createCommandLocalKeyboardWailsPort } from './commandLocalKeyboardWails';

describe('fachada da escrita contextual', () => {
  it('isola a projeção durável clonada da resposta da bridge', async () => {
    const condition = { commandId: 'workspace.tab.chat.create', bySurface: { chat: false },
      bySurfaceId: { chat: { a: true } }, fallback: false,
      byProfile: { focused: { commandId: 'workspace.tab.chat.create', bySurface: { chat: true }, bySurfaceId: { chat: { a: false } }, fallback: false } },
    };
    const source = { generation: 'g', bindings: [], contextualPaletteConditions: [condition] };
    const app = { GetLocalCommandKeyboardMap: vi.fn(async () => source), DispatchLocalCommandKey: vi.fn(), BeginLocalCommandUIKey: vi.fn(), ResetLocalCommandKeyboard: vi.fn() };
    const port = createCommandLocalKeyboardWailsPort({ target: { go: { app: { App: app } } } as unknown as Window });
    const map = await port.loadMap();
    const clone = map.contextualPaletteConditions![0];
    clone.bySurface.chat = true;
    clone.bySurfaceId!.chat.a = false;
    clone.byProfile!.focused.bySurface.chat = false;
    clone.byProfile!.focused.bySurfaceId!.chat.a = true;
    expect(condition.bySurface.chat).toBe(false);
    expect(condition.bySurfaceId.chat.a).toBe(true);
    expect(condition.byProfile.focused.bySurface.chat).toBe(true);
    expect(condition.byProfile.focused.bySurfaceId.chat.a).toBe(false);
    expect(map.contextualPaletteConditions).not.toBe(source.contextualPaletteConditions);
  });
  it('usa Begin contextual com observação copiada, sem Begin genérico', async () => {
    const begin = vi.fn(async () => ({ ticket: 't', invocationId: 'i', commandId: 'workspace.tab.chat.create' }));
    const generic = vi.fn();
    const observed = { surfaceType: 'chat', surfaceId: 'tab-a', profile: 'focused' };
    const target = { go: { app: { App: { BeginUICommand: generic, BeginContextualPaletteUICommand: begin } } } } as unknown as Window;
    const port = createCommandWorkspaceTabWailsPort({ target, contextualPalette: {
      commandId: 'workspace.tab.chat.create', generation: 'g', observed, isCurrent: () => true,
    } });
    observed.surfaceId = 'forged';
    await port.beginUICommand('workspace.tab.chat.create');
    expect(begin).toHaveBeenCalledExactlyOnceWith('g', 'workspace.tab.chat.create', { surfaceType: 'chat', surfaceId: 'tab-a', profile: 'focused' });
    expect(generic).not.toHaveBeenCalled();
  });
  it('recusa lease invalidada durante bridge e no Commit, sem fallback', async () => {
    let current = true;
    const begin = vi.fn(); const generic = vi.fn(); const commit = vi.fn();
    const target = { go: { app: { App: { BeginUICommand: generic, BeginContextualPaletteUICommand: begin, CommitWorkspaceTabCommand: commit } } } } as unknown as Window;
    const port = createCommandWorkspaceTabWailsPort({ target, contextualPalette: {
      commandId: 'workspace.tab.chat.create', generation: 'g', observed: { surfaceType: 'chat', surfaceId: 'a' }, isCurrent: () => current,
    } });
    const pending = port.beginUICommand('workspace.tab.chat.create');
    current = false;
    await expect(pending).rejects.toThrow('contextual-palette-stale');
    expect(() => port.commitBackendCommand('t', 'h')).toThrow('contextual-palette-stale');
    expect(begin).not.toHaveBeenCalled(); expect(generic).not.toHaveBeenCalled(); expect(commit).not.toHaveBeenCalled();
  });
  it('não usa Begin genérico quando API contextual falta ou ID diverge', async () => {
    const generic = vi.fn();
    const target = { go: { app: { App: { BeginUICommand: generic } } } } as unknown as Window;
    const port = createCommandWorkspaceTabWailsPort({ target, contextualPalette: {
      commandId: 'workspace.tab.chat.create', generation: 'g', observed: { surfaceType: 'chat', surfaceId: 'a' }, isCurrent: () => true,
    } });
    await expect(port.beginUICommand('workspace.tab.chat.create')).rejects.toThrow('unavailable');
    await expect(port.beginUICommand('workspace.tab.close')).rejects.toThrow('stale');
    expect(generic).not.toHaveBeenCalled();
  });
  it('submete criação/fechamento pela API contextual genérica', async () => {
    const submit = vi.fn(async () => undefined);
    const target = { go: { app: { App: { CommitWorkspaceTabCommand: submit } } } } as unknown as Window;
    const port = createCommandWorkspaceTabWailsPort({ target });
    const pending = port.commitBackendCommand('ticket', 'handoff');
    expect(submit).toHaveBeenCalledWith('ticket', 'handoff');
    await pending;
  });
  it('submete a criação de terminal pelo mesmo commit contextual', async () => {
    const submit = vi.fn(async () => undefined);
    const target = { go: { app: { App: { CommitWorkspaceTabCommand: submit } } } } as unknown as Window;
    const port = createCommandWorkspaceTabWailsPort({ target });

    await port.commitBackendCommand('terminal-ticket', 'terminal-handoff');

    expect(submit).toHaveBeenCalledExactlyOnceWith('terminal-ticket', 'terminal-handoff');
  });
  it('não oferece API de commit de navegação', () => {
    const generic = vi.fn(async () => undefined);
    const target = { go: { app: { App: { CommitWorkspaceTabNavigationCommand: generic } } } } as unknown as Window;
    const port = createCommandWorkspaceTabWailsPort({ target });
    expect(() => port.commitBackendCommand('ticket', 'handoff')).toThrow();
    expect(generic).not.toHaveBeenCalled();
  });
  it('submete sincronamente à bridge já pronta, sem await de disponibilidade', async () => {
    const submit = vi.fn(async () => undefined);
    const target = { go: { app: { App: { CommitWorkspaceTabCommand: submit } } } } as unknown as Window;
    const port = createCommandWorkspaceTabWailsPort({ target });
    const pending = port.commitBackendCommand('ticket', 'handoff');
    expect(submit).toHaveBeenCalledWith('ticket', 'handoff');
    await pending;
  });
  it('recusa API ausente sem adiar a submissão para outro contexto', () => {
    const port = createCommandWorkspaceTabWailsPort({ target: {} as Window });
    expect(() => port.commitBackendCommand('ticket', 'handoff')).toThrow();
  });
});
