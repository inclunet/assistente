import { expect, it, vi } from 'vitest';
import wire from './__fixtures__/commandKeyboardMap.json';
import profileWire from './__fixtures__/commandKeyboardWorkspaceProfile.json';
import profileOnlyWire from './__fixtures__/commandKeyboardWorkspaceProfileOnly.json';
import { createCommandLocalKeyboardWailsPort } from './commandLocalKeyboardWails';
import { createLocalCommandKeyboard, type LocalCommandKeyboardMap } from './commandLocalKeyboard';
import { parseLocalPaletteConditions } from './commandLocalPaletteConditions';

// Esta fixture é comparada ao endpoint Go real em TestCommandFrontendKeyboardWireContract.
async function controller(map = structuredClone(wire) as unknown as LocalCommandKeyboardMap) {
  const api = { GetLocalCommandKeyboardMap: vi.fn(async () => map), DispatchLocalCommandKey: vi.fn(), BeginLocalCommandUIKey: vi.fn(), ResetLocalCommandKeyboard: vi.fn() };
  const target = { go: { app: { App: api } }, runtime: {} } as unknown as Window;
  const port = createCommandLocalKeyboardWailsPort({ target });
  const accepted = vi.fn();
  const onDown = vi.fn(async () => {});
  const context = { surfaceId: 'contract-toolbar', surfaceType: 'toolbar', profile: undefined as string | undefined, appPage: 'settings' as 'settings' | 'workspace', isCurrent: () => true };
  const keyboard = createLocalCommandKeyboard({ target: window, loadMap: () => port.loadMap(), onDown, onUp: vi.fn(async () => {}), reset: vi.fn(async () => {}), blocked: () => false, onMapAccepted: accepted, readContext: () => context });
  await keyboard.refresh();
  return { keyboard, accepted, onDown, context };
}

function press(code: string, modifiers: KeyboardEventInit) {
  window.dispatchEvent(new KeyboardEvent('keydown', { code, ...modifiers, cancelable: true }));
  window.dispatchEvent(new KeyboardEvent('keyup', { code, ...modifiers, cancelable: true }));
}

it('instala o mapa Go completo, preserva paleta e despacha Ctrl+Tab, Alt+M e Ctrl+N', async () => {
  const { keyboard, accepted, onDown, context } = await controller();
  try {
    expect(accepted).toHaveBeenCalledTimes(1);
    const installed = accepted.mock.calls[0][0] as LocalCommandKeyboardMap;
    expect(installed.localPaletteCommands).toContain('navigation.menu.open');
    expect(parseLocalPaletteConditions(installed.localPaletteConditions ?? [])).not.toBeNull();
    expect(parseLocalPaletteConditions(installed.contextualPaletteConditions ?? [])).not.toBeNull();
    press('Tab', { ctrlKey: true });
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'workspace.tab.next' }));
    press('KeyM', { altKey: true });
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'navigation.menu.open' }));
    press('KeyN', { ctrlKey: true });
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'command_settings.create.open' }));
    context.surfaceType = 'providers';
    press('KeyN', { ctrlKey: true });
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'providers.create.open' }));
    const before = onDown.mock.calls.length;
    context.appPage = 'workspace';
    press('KeyN', { ctrlKey: true });
    expect(onDown).toHaveBeenCalledTimes(before); // Não executa a ação de configurações fora da página.
  } finally { keyboard.dispose(); }
});

it('aceita folha vazia de perfil dentro da página sem habilitar ação inexistente', async () => {
  const map = structuredClone(wire) as unknown as LocalCommandKeyboardMap;
  const branch = map.contextualBindings!.find(entry => entry.byPage)!.byPage!.workspace;
  branch.byProfile = { custom: { shortcut: branch.shortcut, bySurface: {}, fallback: null } };
  const { keyboard, accepted } = await controller(map);
  try { expect(accepted).toHaveBeenCalledTimes(1); } finally { keyboard.dispose(); }
});

it.each(['missing-fallback', 'unknown-page', 'empty-root'])(
  'continua recusando mapa inválido: %s', async (invalid) => {
    const map = structuredClone(wire) as unknown as LocalCommandKeyboardMap;
    const entry = map.contextualBindings!.find(item => item.byPage)!;
    if (invalid === 'missing-fallback') delete (entry.byPage!.workspace as Partial<typeof entry>).fallback;
    if (invalid === 'unknown-page') entry.byPage!.invalid = entry.byPage!.workspace;
    if (invalid === 'empty-root') { delete entry.byPage; }
    const { keyboard, accepted, onDown } = await controller(map);
    try {
      expect(accepted).not.toHaveBeenCalled();
      press('Tab', { ctrlKey: true });
      expect(onDown).not.toHaveBeenCalled();
    } finally { keyboard.dispose(); }
  },
);

it('aceita o DTO Go de personalização por página e perfil sem desabilitar os demais atalhos', async () => {
  const map = structuredClone(wire) as unknown as LocalCommandKeyboardMap;
  map.contextualBindings = [profileWire as unknown as NonNullable<LocalCommandKeyboardMap['contextualBindings']>[number]];
  const { keyboard, accepted, onDown, context } = await controller(map);
  try {
    expect(accepted).toHaveBeenCalledTimes(1);
    context.appPage = 'workspace'; context.surfaceType = 'workspaces'; context.profile = 'dev';
    press('F2', {});
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'workspaces.rename.open' }));
    press('Tab', { ctrlKey: true });
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'workspace.tab.next' }));
  } finally { keyboard.dispose(); }
});

it('perfil sem página preserva o padrão nos demais perfis e aplica a personalização somente no escolhido', async () => {
  const map = structuredClone(wire) as unknown as LocalCommandKeyboardMap;
  map.contextualBindings = [profileOnlyWire as unknown as NonNullable<LocalCommandKeyboardMap['contextualBindings']>[number]];
  const { keyboard, accepted, onDown, context } = await controller(map);
  try {
    expect(accepted).toHaveBeenCalledTimes(1);
    context.appPage = 'workspace'; context.surfaceType = 'workspaces'; context.profile = 'other';
    press('F2', {});
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'workspaces.rename.open' }));
    context.profile = 'dev'; press('F2', {});
    expect(onDown).toHaveBeenLastCalledWith(expect.objectContaining({ commandId: 'workspaces.delete.open' }));
    const before = onDown.mock.calls.length;
    context.surfaceType = 'toolbar'; press('F2', {});
    expect(onDown).toHaveBeenCalledTimes(before);
  } finally { keyboard.dispose(); }
});
