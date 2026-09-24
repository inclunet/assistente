import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { CHAT_CLEAR_COMMAND } from '../lib/commandChatClear';
import { TERMINAL_INTERRUPT_COMMAND } from '../lib/commandTerminalOperation';

const getSettings = vi.fn();
const mutateSettings = vi.fn();
const prepareManual = vi.fn();
const setActive = vi.fn();
const applyLayerAction = vi.fn();
const beginCapture = vi.fn();
const cancelCapture = vi.fn();
const getProfiles = vi.fn();
const announce = vi.fn();
let user = { userId: 'user-1', sessionId: 'session-1' };
let workspaceTabs: { id: string; type: string; title: string }[] = [];
const runtime = vi.hoisted(() => ({
  eventsOn: vi.fn(),
}));

vi.mock('../services/commandSettings', () => ({
  getCommandSettingsForScope: (...args: unknown[]) => getSettings(...args),
  mutateCommandSettings: (...args: unknown[]) => mutateSettings(...args),
  prepareManualCommandLayerForScope: (...args: unknown[]) => prepareManual(...args),
  setCommandLayerActiveForScope: (...args: unknown[]) => setActive(...args),
  applyCommandLayerAction: (...args: unknown[]) => applyLayerAction(...args),
}));
vi.mock('../services/commandDeckCapture', () => ({
  beginCommandDeckCapture: (...args: unknown[]) => beginCapture(...args),
  cancelCommandDeckCapture: (...args: unknown[]) => cancelCapture(...args),
}));
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));
vi.mock('../hooks/useGridFocus', () => ({ useGridFocus: () => ({ handleGridReady: vi.fn() }) }));
vi.mock('../store/authStore', () => ({ useAuthStore: (selector: (state: unknown) => unknown) => selector({ user }) }));
vi.mock('../store/workspaceStore', () => ({ useWorkspaceStore: (selector: (state: unknown) => unknown) => selector({ workspace: { id: 'workspace-1', tabs: workspaceTabs } }) }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: runtime.eventsOn }));
vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({ GetProfiles: (...args: unknown[]) => getProfiles(...args) }));
import CommandSettingsPage from './CommandSettingsPage';

async function bindingAction(name: string) {
  const grid = await screen.findByRole('grid', { name: 'commandSettings.commands' });
  fireEvent.click(within(grid).getByRole('button', { name: 'common.actions' }));
  return screen.findByRole('menuitem', { name });
}

async function selectLayer(name: string) {
  const grid = await screen.findByRole('grid', { name: 'commandSettings.layers' });
  fireEvent.click(within(grid).getByText(name));
  await screen.findByRole('heading', { name });
}

const snapshot = {
  layers: [{ id: 'builtin', name: 'Base', description: 'Base', builtin: true, enabled: true, active: true, manualReady: false, manualActive: false }, { id: 'user', name: 'Minha camada', description: 'Descrição', builtin: false, enabled: true, active: false, manualReady: false, manualActive: false }],
  bindings: [{ id: 'default-1', layerId: 'builtin', commandId: 'cmd.new', triggerType: 'keyboard.local', triggerSpec: '{"version":1,"code":"KeyN","modifiers":["Control"]}', enabled: true, customized: false, readOnly: true, defaultId: 'default-1', reviewStatus: '' }],
  commands: [{ id: 'cmd.new', name: 'Novo', description: 'Novo', allowedSources: ['keyboard.local', 'palette'] }],
  keyboardOperational: false,
};

async function openBindingAdvancedOptions() {
  fireEvent.click(await bindingAction('commandSettings.actions.editBinding'));
  const toggle = await screen.findByRole('button', { name: 'commandSettings.advancedOptions' });
  toggle.focus();
  await userEvent.keyboard('{Enter}');
  await waitFor(() => expect(toggle).toHaveAttribute('aria-expanded', 'true'));
}

async function revealAdvancedOptionsIfNeeded() {
  const toggle = await screen.findByRole('button', { name: 'commandSettings.advancedOptions' });
  if (toggle.getAttribute('aria-expanded') === 'false') fireEvent.click(toggle);
}

describe('CommandSettingsPage', () => {
  let keyboardMapChanged: (() => void) | undefined;
  let deckStatusChanged: ((payload: unknown) => void) | undefined;
  let deckCaptureChanged: ((payload: unknown) => void) | undefined;
  let unsubscribeKeyboardMapChanged: ReturnType<typeof vi.fn>;

  it('recupera snapshot ausente quando o mapa é publicado depois do bootstrap', async () => {
    getSettings.mockRejectedValueOnce(new Error('bootstrap ainda não pronto'))
      .mockResolvedValueOnce({ ...snapshot, keyboardOperational: true });
    render(<CommandSettingsPage />);
    await screen.findByText('commandSettings.errors.load');

    await act(async () => keyboardMapChanged?.());

    expect(await screen.findByText('commandSettings.keyboardAvailable')).toBeInTheDocument();
    expect(getSettings).toHaveBeenCalledTimes(2);
  });

  it('drena evento recebido durante a carga inicial depois que ela termina em erro', async () => {
    let rejectInitial: (reason: Error) => void = () => undefined;
    getSettings
      .mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectInitial = reject; }))
      .mockResolvedValueOnce({ ...snapshot, keyboardOperational: true });
    render(<CommandSettingsPage />);

    await act(async () => keyboardMapChanged?.());
    await act(async () => rejectInitial(new Error('bootstrap ainda não pronto')));

    expect(await screen.findByText('commandSettings.keyboardAvailable')).toBeInTheDocument();
    expect(getSettings).toHaveBeenCalledTimes(2);
  });

  it('não recarrega o snapshot em evento de fundo durante editor ou mutação', async () => {
    let resolveMutation: (value: unknown) => void = () => undefined;
    mutateSettings.mockImplementationOnce(() => new Promise((resolve) => { resolveMutation = resolve; }));
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');

    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.actions.newLayer' }));
    await act(async () => keyboardMapChanged?.());
    expect(getSettings).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }));
    fireEvent.click(await bindingAction('commandSettings.actions.suppress'));
    await act(async () => keyboardMapChanged?.());
    expect(getSettings).toHaveBeenCalledTimes(1);

    await act(async () => resolveMutation({ committed: true, published: true, id: 'default-1' }));
  });

  it('remove o listener de bootstrap ao desmontar', () => {
    const { unmount } = render(<CommandSettingsPage />);
    unmount();
    expect(unsubscribeKeyboardMapChanged).toHaveBeenCalledTimes(6);
  });

  it('informa o recorte operacional quando o backend oferece teclado local', async () => {
    getSettings.mockResolvedValue({ ...snapshot, keyboardOperational: true });
    render(<CommandSettingsPage />);
    expect(await screen.findByText('commandSettings.keyboardAvailable')).toBeInTheDocument();
    expect(screen.queryByText('commandSettings.keyboardUnavailable')).not.toBeInTheDocument();
  });

  it('aplica toggle temporário à regra explícita e volta sem ruleID no escopo atual', async () => {
    applyLayerAction.mockResolvedValue({ committed: true, published: true, id: 'activation' });
    getSettings.mockResolvedValue({
      ...snapshot,
      rules: [{
        id: 'rule-temporary', layerId: 'user', mode: 'manual', condition: { version: 1, clauses: [] },
        lifecycle: 'temporary', enabled: true, reviewStatus: 'active',
      }],
    });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    const rulesGrid = await screen.findByRole('grid', { name: 'commandSettings.rules.title' });
    fireEvent.click(within(rulesGrid).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.manualToggle' }));
    fireEvent.change(screen.getByRole('spinbutton'), { target: { value: '42' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.apply' }));
    await waitFor(() => expect(applyLayerAction).toHaveBeenCalledWith('global', 'rule-temporary', 'toggle', 42));

  });

  it('envia back sem ruleID e aguarda o commit observável', async () => {
    applyLayerAction.mockResolvedValue({ committed: true, published: true, id: 'back' });
    getSettings.mockResolvedValue(snapshot);
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.manualBack' }));
    await waitFor(() => expect(applyLayerAction).toHaveBeenCalledWith('global', '', 'back', 0));
    expect(await screen.findByText('commandSettings.messages.saved')).toBeInTheDocument();
  });

  it('desativa regra manual ativa sem solicitar duração temporária', async () => {
    applyLayerAction.mockResolvedValue({ committed: true, published: true, id: 'activation' });
    getSettings.mockResolvedValue({
      ...snapshot,
      rules: [{ id: 'rule-active', layerId: 'user', mode: 'manual', condition: { version: 1, clauses: [] }, lifecycle: 'temporary', enabled: true, manualActive: true, manualExpiresAt: Date.now() + 60000, reviewStatus: 'active' }],
    });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    const rulesGrid = await screen.findByRole('grid', { name: 'commandSettings.rules.title' });
    fireEvent.click(within(rulesGrid).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.manualDeactivate' }));
    await waitFor(() => expect(applyLayerAction).toHaveBeenCalledWith('global', 'rule-active', 'deactivate', 0));
  });

  it('traduz status do Stream Deck e usa fallback para valores desconhecidos', async () => {
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    await act(async () => deckStatusChanged?.({ status: 'connected', devices: [] }));
    expect(screen.getByText(/commandSettings\.deck\.status/)).toBeInTheDocument();
    const deckStatus = screen.getByRole('region', { name: /commandSettings\.deck\.status/ });
    expect(within(deckStatus).queryByText('connected', { exact: true })).not.toBeInTheDocument();
    await act(async () => deckStatusChanged?.({ status: 'future-state', devices: [] }));
    expect(screen.queryByText(/future-state/)).not.toBeInTheDocument();
  });

  it('mostra estado/capacidade por dispositivo sem expor identificador ou reason bruto', async () => {
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    await act(async () => deckStatusChanged?.({ status: 'degraded', devices: [
      { id: 'PRIVATE-SERIAL-01', model: 'Stream Deck XL', keyCount: 32, status: 'reconnecting', reason: 'reconnect_backoff' },
      { id: 'PRIVATE-SERIAL-02', model: 'Stream Deck Mini', keyCount: 6, status: 'unavailable', reason: 'open_failed' },
    ] }));

    expect(screen.getByRole('heading', { name: /commandSettings\.deck\.status/ })).toBeInTheDocument();
    expect(screen.getByText('Stream Deck XL')).toBeInTheDocument();
    expect(screen.getByText('Stream Deck Mini')).toBeInTheDocument();
    expect(screen.getByText('commandSettings.deck.statuses.reconnecting')).toBeInTheDocument();
    expect(screen.getByText('commandSettings.deck.statuses.unavailable')).toBeInTheDocument();
    expect(screen.getByText('commandSettings.deck.reasons.reconnectBackoff')).toBeInTheDocument();
    expect(screen.getByText('commandSettings.deck.reasons.openFailed')).toBeInTheDocument();
    expect(screen.queryByText(/PRIVATE-SERIAL/)).not.toBeInTheDocument();
    expect(screen.queryByText(/reconnect_backoff|open_failed/)).not.toBeInTheDocument();
  });

  it('revela prioridade/condições por teclado e deixa erros de perfil visíveis antes da expansão', async () => {
    getProfiles.mockRejectedValue(new Error('catálogo indisponível'));
    getSettings.mockResolvedValue({ ...snapshot,
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined }],
    });
    render(<CommandSettingsPage />);
    await selectLayer('Minha camada');
    fireEvent.click(await bindingAction('commandSettings.actions.editBinding'));

    expect(screen.getByText('profiles.loadError')).toBeInTheDocument();
    const toggle = screen.getByRole('button', { name: 'commandSettings.advancedOptions' });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByLabelText('commandSettings.form.priority')).not.toBeInTheDocument();

    toggle.focus();
    expect(toggle).toHaveFocus();
    await userEvent.keyboard('{Enter}');
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByLabelText('commandSettings.form.priority')).toBeVisible();
    expect(screen.getByText('profiles.loadError')).toBeInTheDocument();
  });

  it('grava acionador por captura, exibe modelo e tecla e não expõe serial', async () => {
    const userEventSetup = userEvent.setup();
    getSettings.mockResolvedValue({
      ...snapshot,
      commands: [{ ...snapshot.commands[0], allowedSources: ['streamdeck.key'] }],
    });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    await act(async () => deckStatusChanged?.({ status: 'connected', devices: [{ id: 'RUNTIME', model: 'Runtime', keyCount: 15, status: 'connected' }] }));
    fireEvent.click(screen.getByText('Minha camada'));
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.actions.newBinding' }));
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'streamdeck.key' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.form.captureButton' }));
    expect(beginCapture).toHaveBeenCalledWith(expect.any(String));
    const requestId = beginCapture.mock.calls[0][0];
    await act(async () => deckCaptureChanged?.({ requestId, status: 'captured', model: 'Stream Deck XL', key: 255, triggerSpec: '{"version":1,"device":"SERIAL-MAX","key":255}' }));
    expect(screen.getByText('commandSettings.form.captureResult')).toBeInTheDocument();
    expect(screen.queryByText('SERIAL-MAX')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'common.save' })).toBeEnabled();
    await userEventSetup.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_create',
      scope: 'global',
      binding: expect.objectContaining({ triggerSpec: '{"version":1,"device":"SERIAL-MAX","key":255}' }),
    })));
  });

  it('cancela captura sem apagar binding anterior e ignora evento obsoleto', async () => {
    mutateSettings.mockResolvedValue({ committed: true, published: true, id: 'binding' });
    getSettings.mockResolvedValue({
      ...snapshot,
      commands: [{ ...snapshot.commands[0], allowedSources: ['streamdeck.key'] }],
    });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.actions.newBinding' }));
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'streamdeck.key' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.form.captureButton' }));
    const firstRequestId = beginCapture.mock.calls[0][0];
    await act(async () => deckCaptureChanged?.({ requestId: firstRequestId, status: 'captured', model: 'Stream Deck', key: 4, triggerSpec: '{"version":1,"device":"FIRST","key":4}' }));
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.form.captureButton' }));
    const requestId = beginCapture.mock.calls[1][0];
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.form.captureCancel' }));
    expect(cancelCapture).toHaveBeenCalledWith(requestId);
    await act(async () => deckCaptureChanged?.({ requestId, status: 'captured', model: 'Late', key: 1, triggerSpec: '{"version":1,"device":"LATE","key":1}' }));
    await userEvent.setup().click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_create',
      binding: expect.objectContaining({ triggerSpec: '{"version":1,"device":"FIRST","key":4}' }),
    })));
  });

  it('mostra estados de erro da captura sem sair do editor', async () => {
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.actions.newBinding' }));
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'streamdeck.key' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.form.captureButton' }));
    const requestId = beginCapture.mock.calls[0][0];
    await act(async () => deckCaptureChanged?.({ requestId, status: 'timeout' }));
    expect(await screen.findByText('commandSettings.form.captureStatus.timeout')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'common.save' })).toBeDisabled();
  });

  it('aguarda o HID abrir antes de pedir a tecla e mantém no_device cancelável', async () => {
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.actions.newBinding' }));
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'streamdeck.key' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.form.captureButton' }));
    const requestId = beginCapture.mock.calls[0][0];
    await act(async () => deckCaptureChanged?.({ requestId, status: 'starting' }));
    expect(screen.getByText('commandSettings.form.captureStarting')).toBeInTheDocument();
    await act(async () => deckCaptureChanged?.({ requestId, status: 'no_device' }));
    expect(screen.getByRole('button', { name: 'commandSettings.form.captureCancel' })).toBeEnabled();
    expect(screen.getByText('commandSettings.form.captureStatus.no_device')).toBeInTheDocument();
    await act(async () => window.dispatchEvent(new Event('blur')));
    expect(cancelCapture).toHaveBeenCalledWith(requestId);
  });

  it('cancela a captura tardia após desmontagem', async () => {
    const userEvents = userEvent.setup();
    let resolveBegin: (() => void) | undefined;
    beginCapture.mockImplementationOnce(() => new Promise<void>((resolve) => { resolveBegin = resolve; }));
    const { unmount } = render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    await userEvents.click(screen.getByText('Minha camada'));
    await userEvents.click(screen.getByRole('button', { name: 'commandSettings.actions.newBinding' }));
    await userEvents.selectOptions(await screen.findByLabelText('commandSettings.form.source'), 'streamdeck.key');
    await userEvents.click(screen.getByRole('button', { name: 'commandSettings.form.captureButton' }));
    const requestId = beginCapture.mock.calls[0][0];
    unmount();
    await act(async () => resolveBegin?.());
    expect(cancelCapture).toHaveBeenCalledTimes(2);
    expect(cancelCapture).toHaveBeenLastCalledWith(requestId);
  });

  it('não exibe status de teclado enquanto o snapshot está carregando', () => {
    getSettings.mockReturnValueOnce(new Promise(() => undefined));
    render(<CommandSettingsPage />);
    expect(screen.queryByText('commandSettings.keyboardAvailable')).not.toBeInTheDocument();
    expect(screen.queryByText('commandSettings.keyboardUnavailable')).not.toBeInTheDocument();
  });

  it('não exibe status de teclado quando o carregamento falha', async () => {
    getSettings.mockRejectedValueOnce(new Error('load failed'));
    render(<CommandSettingsPage />);
    expect(await screen.findByText('commandSettings.errors.load')).toBeInTheDocument();
    expect(screen.queryByText('commandSettings.keyboardAvailable')).not.toBeInTheDocument();
    expect(screen.queryByText('commandSettings.keyboardUnavailable')).not.toBeInTheDocument();
  });

  it('mostra o status real depois de recuperar o carregamento', async () => {
    getSettings
      .mockRejectedValueOnce(new Error('load failed'))
      .mockResolvedValueOnce({ ...snapshot, keyboardOperational: true });
    render(<CommandSettingsPage />);
    await screen.findByText('commandSettings.errors.load');
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.actions.reload' }));
    expect(await screen.findByText('commandSettings.keyboardAvailable')).toBeInTheDocument();
    expect(screen.queryByText('commandSettings.keyboardUnavailable')).not.toBeInTheDocument();
  });

  it('mantém o erro próprio de mutação após rejeição da operação', async () => {
    mutateSettings.mockRejectedValueOnce(new Error('mutation failed'));
    render(<CommandSettingsPage />);
    fireEvent.click(await bindingAction('commandSettings.actions.suppress'));
    expect(await screen.findByText('commandSettings.errors.generic')).toBeInTheDocument();
    expect(screen.queryByText('commandSettings.errors.load')).not.toBeInTheDocument();
  });

  it('mantém defaults globais de job somente leitura e suprime sem os argumentos nativos', async () => {
    const globalBinding = {
      id: 'builtin.global.job.run',
      layerId: 'application.global_hotkeys',
      commandId: 'job.run',
      triggerType: 'keyboard.global',
      triggerSpec: '{"version":1,"code":"KeyJ","modifiers":["Control","Shift"]}',
      arguments: { job_id: 'weekly-report' },
      enabled: true,
      customized: false,
      readOnly: true,
      defaultId: 'builtin.global.job.run',
      currentDefaultVersion: '1',
      currentDefaultFingerprint: 'fingerprint-global-job',
      reviewStatus: 'active',
    };
    const initialSnapshot = {
      ...snapshot,
      layers: [...snapshot.layers, {
        id: 'application.global_hotkeys', name: 'Atalhos globais', description: '',
        builtin: true, enabled: true, active: true, manualReady: false, manualActive: false,
      }],
      bindings: [globalBinding],
      commands: [{ id: 'job.run', name: 'Executar job', description: '', allowedSources: ['keyboard.global'] }],
    };
    const persistedOverrideSnapshot = {
      ...initialSnapshot,
      bindings: [{
        ...globalBinding,
        id: 'persisted-global-job-override',
        customized: true,
        suppressed: true,
        enabled: false,
        effect: 'suppress',
      }],
    };
    getSettings.mockResolvedValueOnce(initialSnapshot).mockResolvedValueOnce(persistedOverrideSnapshot);
    render(<CommandSettingsPage />);

    fireEvent.click(await screen.findByText('Atalhos globais'));
    const grid = await screen.findByRole('grid', { name: 'commandSettings.commands' });
    expect(within(grid).getByText('Control+Shift+KeyJ')).toBeInTheDocument();
    expect(within(grid).getByText('commandSettings.sources.keyboard')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'commandSettings.actions.newBinding' })).toBeDisabled();

    const edit = await bindingAction('commandSettings.actions.editBinding');
    expect(edit).toBeDisabled();
    fireEvent.click(edit);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    fireEvent.click(await bindingAction('commandSettings.actions.suppress'));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_create',
      binding: expect.objectContaining({
        layerId: 'application.global_hotkeys',
        commandId: '',
        triggerType: 'keyboard.global',
        triggerSpec: globalBinding.triggerSpec,
        arguments: {},
        effect: 'suppress',
        replacesDefaultId: 'builtin.global.job.run',
        replacesDefaultVersion: '1',
        replacesDefaultFingerprint: 'fingerprint-global-job',
      }),
    })));

    await waitFor(() => expect(getSettings).toHaveBeenCalledTimes(2));
    fireEvent.click(await screen.findByText('Atalhos globais'));
    fireEvent.click(await bindingAction('commandSettings.actions.restore'));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledTimes(2));
    expect(mutateSettings).toHaveBeenLastCalledWith(expect.objectContaining({
      operation: 'binding_restore',
      id: 'persisted-global-job-override',
    }));
    expect(mutateSettings).not.toHaveBeenLastCalledWith(expect.objectContaining({
      operation: 'binding_restore',
      id: 'builtin.global.job.run',
    }));
  });

  beforeEach(() => {
    vi.resetAllMocks();
    user = { userId: 'user-1', sessionId: 'session-1' };
    workspaceTabs = [];
    getSettings.mockResolvedValue(snapshot);
    getProfiles.mockResolvedValue([{ name: 'Padrão', slug: 'padrao', description: '', icon: '', source: 'local', builtin: true }]);
    prepareManual.mockResolvedValue({ committed: true, published: true, id: 'user' });
    setActive.mockResolvedValue({ committed: true, published: true, id: 'user' });
    mutateSettings.mockResolvedValue({ committed: true, published: true, id: 'mutation' });
    keyboardMapChanged = undefined;
    unsubscribeKeyboardMapChanged = vi.fn();
    runtime.eventsOn.mockImplementation((event: string, handler: () => void) => {
      if (event === 'command:keyboard-map-changed') keyboardMapChanged = handler;
      if (event === 'command:deck-status') deckStatusChanged = handler as (payload: unknown) => void;
      if (event === 'command:deck-capture') deckCaptureChanged = handler as (payload: unknown) => void;
      return unsubscribeKeyboardMapChanged;
    });
  });

  it('usa o deep link da paleta para abrir o binding do comando na configuração', async () => {
    const originalURL = window.location.href;
    const targetURL = new URL(originalURL);
    targetURL.search = '?commandId=cmd.new';
    window.history.replaceState(window.history.state, '', targetURL);
    getSettings.mockResolvedValue({
      ...snapshot,
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined }],
    });
    const view = render(<CommandSettingsPage />);
    try {
      expect(await screen.findByRole('dialog', { name: 'commandSettings.dialog.bindingTitle' })).toBeInTheDocument();
      expect(window.location.search).toBe('');
    } finally {
      view.unmount();
      window.history.replaceState(window.history.state, '', originalURL);
    }
  });

  it('mantém a seção avançada do binding alcançável por teclado', async () => {
    getSettings.mockResolvedValue({
      ...snapshot,
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined }],
    });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    await selectLayer('Minha camada');
    await openBindingAdvancedOptions();
    expect(screen.getByRole('button', { name: 'commandSettings.advancedOptions' })).toHaveAttribute('aria-expanded', 'true');
  });

  it('não permite persistir argumentos potencialmente sensíveis de uma execução de ferramenta', async () => {
    const commandId = 'tool.execute.t_018f123456787abc8def012345678901';
    getSettings.mockResolvedValue({
      ...snapshot,
      commands: [...snapshot.commands, { id: commandId, name: 'Executar: Pesquisa', description: '', allowedSources: ['keyboard.local'] }],
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined,
        commandId, arguments: { arguments_json: '{"token":"nao-exibir"}' } }],
    });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(await bindingAction('commandSettings.actions.editBinding'));

    expect(screen.getByText('commandSettings.toolExecution.adHocOnly')).toBeInTheDocument();
    await waitFor(() => expect(announce).toHaveBeenCalledWith('commandSettings.toolExecution.adHocOnly'));
    expect(screen.queryByDisplayValue(/nao-exibir/)).not.toBeInTheDocument();
    expect(screen.queryByText(/018f1234|tool\.execute\.t_/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'common.save' })).toBeDisabled();
  });

  async function layerAction(name: string) {
    const grid = await screen.findByRole('grid', { name: 'commandSettings.layers' });
    const row = within(grid).getByText('Minha camada').closest('[role="row"]');
    expect(row).not.toBeNull();
    fireEvent.click(within(row as HTMLElement).getByRole('button', { name: 'common.actions' }));
    return screen.findByRole('menuitem', { name });
  }

  it('prepara uma camada sem ativá-la automaticamente', async () => {
    render(<CommandSettingsPage />);
    const prepare = await layerAction('commandSettings.prepareManual');
    fireEvent.click(prepare);
    await waitFor(() => expect(prepareManual).toHaveBeenCalledWith('global', 'user'));
    expect(setActive).not.toHaveBeenCalled();
  });

  it('não trata enabled como manualActive e envia true na ativação', async () => {
    getSettings.mockResolvedValue({ ...snapshot, layers: snapshot.layers.map((layer) => layer.id === 'user' ? { ...layer, manualReady: true, enabled: true, active: false, manualActive: false } : layer) });
    render(<CommandSettingsPage />);
    const activate = await layerAction('commandSettings.activateManual');
    fireEvent.click(activate);
    await waitFor(() => expect(setActive).toHaveBeenCalledWith('global', 'user', true));
    expect(setActive).toHaveBeenCalledTimes(1);
  });

  it('desativa a claim manual desta tela quando ela existe', async () => {
    getSettings.mockResolvedValue({ ...snapshot, layers: snapshot.layers.map((layer) => layer.id === 'user' ? { ...layer, manualReady: true, manualActive: true, active: true } : layer) });
    render(<CommandSettingsPage />);
    const deactivate = await layerAction('commandSettings.deactivateManual');
    fireEvent.click(deactivate);
    await waitFor(() => expect(setActive).toHaveBeenCalledWith('global', 'user', false));
  });

  it('não oferece retirar a ativação que pertence somente a outra origem', async () => {
    getSettings.mockResolvedValue({ ...snapshot, layers: snapshot.layers.map((layer) => layer.id === 'user' ? { ...layer, manualReady: true, manualActive: false, active: true } : layer) });
    render(<CommandSettingsPage />);
    const activate = await layerAction('commandSettings.activateManual');
    expect(screen.queryByRole('menuitem', { name: 'commandSettings.deactivateManual' })).not.toBeInTheDocument();
    fireEvent.click(activate);
    await waitFor(() => expect(setActive).toHaveBeenCalledWith('global', 'user', true));
  });

  it('permite retirar a claim manual mesmo com a camada desabilitada', async () => {
    getSettings.mockResolvedValue({ ...snapshot, layers: snapshot.layers.map((layer) => layer.id === 'user' ? { ...layer, manualReady: true, manualActive: true, enabled: false, active: false } : layer) });
    render(<CommandSettingsPage />);
    const deactivate = await layerAction('commandSettings.deactivateManual');
    expect(deactivate).toBeEnabled();
    fireEvent.click(deactivate);
    await waitFor(() => expect(setActive).toHaveBeenCalledWith('global', 'user', false));
  });

  it('desabilita a ativação manual quando a camada está disabled', async () => {
    getSettings.mockResolvedValue({ ...snapshot, layers: snapshot.layers.map((layer) => layer.id === 'user' ? { ...layer, manualReady: true, manualActive: false, enabled: false } : layer) });
    render(<CommandSettingsPage />);
    expect(await layerAction('commandSettings.activateManual')).toBeDisabled();
    expect(setActive).not.toHaveBeenCalled();
  });

  it('mantém committedNotPublished sem repetir a ativação', async () => {
    setActive.mockResolvedValue({ committed: true, published: false, id: 'user' });
    getSettings.mockResolvedValue({ ...snapshot, layers: snapshot.layers.map((layer) => layer.id === 'user' ? { ...layer, manualReady: true } : layer) });
    render(<CommandSettingsPage />);
    fireEvent.click(await layerAction('commandSettings.activateManual'));
    expect(await screen.findByText('commandSettings.messages.committedNotPublished')).toBeInTheDocument();
    expect(setActive).toHaveBeenCalledTimes(1);
  });

  it('carrega camadas e oferece supressão própria para default readonly', async () => {
    render(<CommandSettingsPage />);
    await waitFor(() => expect(screen.getByText('Minha camada')).toBeInTheDocument());
    expect(screen.getByText('Control+KeyN')).toBeInTheDocument();
    const suppress = await bindingAction('commandSettings.actions.suppress');
    fireEvent.click(suppress);
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_create',
      binding: expect.objectContaining({ effect: 'suppress', replacesDefaultId: 'default-1' }),
    })));
  });

  it('mostra a sequência padrão e permite suprimi-la sem converter para atalho simples', async () => {
    getSettings.mockResolvedValue({
      ...snapshot,
      bindings: snapshot.bindings.map((binding) => ({ ...binding, triggerSpec: JSON.stringify({
        version: 2,
        steps: [{ code: 'KeyN', modifiers: ['Control'] }, { code: 'KeyC', modifiers: [] }],
      }) })),
    });
    render(<CommandSettingsPage />);
    expect(await screen.findByText('Control+KeyN KeyC')).toBeInTheDocument();
    fireEvent.click(await bindingAction('commandSettings.actions.suppress'));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_create',
      binding: expect.objectContaining({ effect: 'suppress', replacesDefaultId: 'default-1' }),
    })));
  });

  it.each([false, true])('mostra F1 sem modificadores e altera supressão (customized=%s)', async (customized) => {
    const defaultId = 'builtin.keyboard.f1.navigation.help.open';
    getSettings.mockResolvedValue({
      ...snapshot,
      commands: [{ ...snapshot.commands[0], id: 'navigation.help.open', name: 'Ajuda' }],
      bindings: [{ ...snapshot.bindings[0], id: defaultId, defaultId,
        commandId: 'navigation.help.open', customized, enabled: !customized,
        triggerSpec: JSON.stringify({ version: 1, code: 'F1', modifiers: [] }),
      }],
    });
    render(<CommandSettingsPage />);
    expect(await screen.findByText('F1')).toBeInTheDocument();
    fireEvent.click(await bindingAction(customized ? 'commandSettings.actions.restore' : 'commandSettings.actions.suppress'));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining(
      customized
        ? { operation: 'binding_restore', id: defaultId }
        : { operation: 'binding_create', binding: expect.objectContaining({ effect: 'suppress', replacesDefaultId: defaultId }) }
    )));
  });

  it('bloqueia duplo disparo durante mutação', async () => {
    let resolve: (value: unknown) => void = () => undefined;
    mutateSettings.mockImplementation(() => new Promise((done) => { resolve = done; }));
    render(<CommandSettingsPage />);
    await waitFor(() => expect(screen.getByText('Minha camada')).toBeInTheDocument());
    const suppress = await bindingAction('commandSettings.actions.suppress');
    fireEvent.click(suppress); fireEvent.click(suppress);
    expect(mutateSettings).toHaveBeenCalledTimes(1);
    await act(async () => resolve({ committed: true, published: true, id: 'default-1' }));
  });

  it('não anuncia sucesso quando o backend não confirma commit', async () => {
    mutateSettings.mockResolvedValue({ committed: false, published: false, id: 'default-1' });
    render(<CommandSettingsPage />);
    await waitFor(() => expect(screen.getByText('Minha camada')).toBeInTheDocument());
    fireEvent.click(await bindingAction('commandSettings.actions.suppress'));
    await screen.findByText('commandSettings.errors.generic');
    expect(announce).toHaveBeenCalledWith('commandSettings.errors.generic', 'assertive');
    expect(announce).not.toHaveBeenCalledWith('commandSettings.messages.saved');
    expect(getSettings).toHaveBeenCalledTimes(1);
  });

  it('restaura uma personalização sem executar exclusão genérica', async () => {
    getSettings.mockResolvedValue({ ...snapshot, bindings: [{ ...snapshot.bindings[0], customized: true, enabled: false }] });
    render(<CommandSettingsPage />);
    fireEvent.click(await bindingAction('commandSettings.actions.restore'));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_restore', id: 'default-1',
    })));
  });

  it('usa o ID persistido do delta para restaurar e rebasear um default', async () => {
    getSettings.mockResolvedValue({
      ...snapshot,
      bindings: [{
        ...snapshot.bindings[0],
        id: 'delta-1',
        customized: true,
        currentDefaultVersion: '2',
        currentDefaultFingerprint: 'current-fp',
        replacesDefaultVersion: '1',
        replacesDefaultFingerprint: 'old-fp',
      }],
    });
    render(<CommandSettingsPage />);
    const restore = await bindingAction('commandSettings.actions.restore');
    fireEvent.click(restore);
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_restore', id: 'delta-1',
    })));

    getSettings.mockResolvedValue({
      ...snapshot,
      bindings: [{
        ...snapshot.bindings[0],
        id: 'delta-1',
        customized: true,
        currentDefaultVersion: '2',
        currentDefaultFingerprint: 'current-fp',
      }],
    });
    render(<CommandSettingsPage />);
    const rebase = await bindingAction('commandSettings.actions.rebaseDefault');
    expect(rebase).toBeEnabled();
    fireEvent.click(rebase);
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'default_rebase',
      default: expect.objectContaining({
        bindingId: 'delta-1',
        default: { id: 'default-1', version: '2', fingerprint: 'current-fp' },
      }),
    })));
  });

  it('mantém aviso de commit sem publicação e não repete a operação', async () => {
    mutateSettings.mockResolvedValue({ committed: true, published: false, id: 'default-1' });
    render(<CommandSettingsPage />);
    fireEvent.click(await bindingAction('commandSettings.actions.suppress'));
    expect(await screen.findByText('commandSettings.messages.committedNotPublished')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledWith('commandSettings.messages.committedNotPublished');
    expect(mutateSettings).toHaveBeenCalledTimes(1);
  });

  it('descarta resultado e erro de mutação após troca de sessão', async () => {
    let reject: (reason: Error) => void = () => undefined;
    mutateSettings.mockReturnValue(new Promise((_resolve, fail) => { reject = fail; }));
    const { rerender } = render(<CommandSettingsPage />);
    fireEvent.click(await bindingAction('commandSettings.actions.suppress'));
    user = { ...user, sessionId: 'session-2' };
    rerender(<CommandSettingsPage />);
    await act(async () => reject(new Error('old session')));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(announce).not.toHaveBeenCalledWith('commandSettings.messages.saved');
  });

  it('não mostra snapshot que chegou após troca de usuário', async () => {
    let resolve: (value: typeof snapshot) => void = () => undefined;
    getSettings.mockReturnValueOnce(new Promise((done) => { resolve = done; }));
    const { rerender } = render(<CommandSettingsPage />);
    user = { userId: 'user-2', sessionId: 'session-2' };
    getSettings.mockResolvedValue({ ...snapshot, layers: [{ ...snapshot.layers[0], name: 'Nova conta' }] });
    rerender(<CommandSettingsPage />);
    await screen.findAllByText('Nova conta');
    await act(async () => resolve(snapshot));
    expect(screen.queryByText('Minha camada')).not.toBeInTheDocument();
  });

  it('restringe builtin, deriva seleção palette e exige combinação no teclado', async () => {
    mutateSettings.mockResolvedValue({ committed: true, published: true, id: 'binding' });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    expect(screen.getByRole('button', { name: 'commandSettings.actions.newBinding' })).toBeDisabled();
    fireEvent.click(screen.getByText('Minha camada'));
    const create = screen.getByRole('button', { name: 'commandSettings.actions.newBinding' });
    await waitFor(() => expect(create).toBeEnabled());
    fireEvent.click(create);
    expect(screen.getByRole('button', { name: 'common.save' })).toBeDisabled();
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'palette' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_create',
      binding: expect.objectContaining({ layerId: 'user', commandId: 'cmd.new', triggerType: 'palette', triggerSpec: '{"version":1,"selection":"cmd.new"}' }),
    })));
  });

  it.each(['global', 'workspace'] as const)('cria, grava e salva uma sequência v2 no escopo %s', async (scope) => {
    getSettings.mockImplementation((_locale: string, requestedScope: string) => Promise.resolve(
      requestedScope === 'workspace'
        ? { ...snapshot, layers: snapshot.layers.map((layer) => layer.id === 'user' ? { ...layer, workspaceId: 'workspace-1' } : layer) }
        : snapshot
    ));
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    if (scope === 'workspace') {
      fireEvent.change(screen.getByLabelText('commandSettings.scope.label'), { target: { value: scope } });
      await waitFor(() => expect(getSettings).toHaveBeenLastCalledWith(expect.any(String), scope));
    }
    fireEvent.click(screen.getByText('Minha camada'));
    const create = await screen.findByRole('button', { name: 'commandSettings.actions.newBinding' });
    await waitFor(() => expect(create).toBeEnabled());
    fireEvent.click(create);

    fireEvent.change(screen.getByLabelText('commandShortcutCapture.mode'), { target: { value: 'sequence' } });
    const capture = screen.getByRole('button', { name: 'commandShortcutCapture.buttonLabel' });
    fireEvent.click(capture);
    fireEvent.keyDown(capture, { key: 'k', code: 'KeyK', ctrlKey: true, getModifierState: () => false });
    fireEvent.keyUp(capture, { key: 'k', code: 'KeyK', ctrlKey: false, getModifierState: () => false });
    fireEvent.keyDown(capture, { key: 'c', code: 'KeyC', getModifierState: () => false });

    await waitFor(() => expect(screen.getByRole('button', { name: 'common.save' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_create',
      scope,
      binding: expect.objectContaining({
        triggerSpec: '{"version":2,"steps":[{"code":"KeyK","modifiers":["Control"]},{"code":"KeyC","modifiers":[]}]}',
      }),
    })));
  });

  it('bloqueia salvar durante o prefixo e cancela sem alterar o atalho anterior', async () => {
    const previous = '{"version":1,"code":"KeyN","modifiers":["Control"]}';
    getSettings.mockResolvedValue({
      ...snapshot,
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined, triggerSpec: previous }],
    });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    fireEvent.click(within(await screen.findByRole('grid', { name: 'commandSettings.commands' })).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.actions.editBinding' }));

    fireEvent.change(screen.getByLabelText('commandShortcutCapture.mode'), { target: { value: 'sequence' } });
    const capture = screen.getByRole('button', { name: 'commandShortcutCapture.buttonLabel' });
    fireEvent.click(capture);
    fireEvent.keyDown(capture, { key: 'k', code: 'KeyK', ctrlKey: true, getModifierState: () => false });
    expect(screen.getByRole('button', { name: 'common.save' })).toBeDisabled();
    expect(mutateSettings).not.toHaveBeenCalled();

    fireEvent.keyDown(capture, { key: 'Escape', code: 'Escape', getModifierState: () => false });
    expect(capture).toHaveTextContent('Control+KeyN');
    expect(screen.getByRole('button', { name: 'common.save' })).toBeEnabled();

    fireEvent.click(capture);
    fireEvent.keyDown(capture, { key: 'k', code: 'KeyK', ctrlKey: true, getModifierState: () => false });
    fireEvent.change(screen.getByLabelText('commandSettings.form.source'), { target: { value: 'palette' } });
    expect(screen.getByRole('button', { name: 'common.save' })).toBeEnabled();
  });

  it('edita uma sequência v2 preservando-a ao alterar prioridade e condições', async () => {
    const sequence = '{"version":2,"steps":[{"code":"KeyK","modifiers":["Control"]},{"code":"KeyC","modifiers":[]}]}';
    getSettings.mockResolvedValue({
      ...snapshot,
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined, triggerSpec: sequence }],
    });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    fireEvent.click(within(await screen.findByRole('grid', { name: 'commandSettings.commands' })).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.actions.editBinding' }));
    await revealAdvancedOptionsIfNeeded();
    expect(screen.getByLabelText('commandShortcutCapture.mode')).toHaveValue('sequence');
    fireEvent.change(screen.getByLabelText('commandSettings.form.priority'), { target: { value: '7' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    fireEvent.click(screen.getByLabelText('commandSettings.conditions.value'));
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_update',
      scope: 'global',
      binding: expect.objectContaining({
        resolutionPriority: 7,
        triggerSpec: sequence,
        condition: { version: 1, clauses: [{ field: 'app.focused', value: true }] },
      }),
    })));
  });

  it('configura aba pelo nome e envia tipo e identidade sem entrada de ID opaco', async () => {
    workspaceTabs = [{ id: 'chat-one', type: 'chat', title: 'Conversa de trabalho' }];
    getSettings.mockResolvedValue({ ...snapshot, bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined }] });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(within(await screen.findByRole('grid', { name: 'commandSettings.commands' })).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.actions.editBinding' }));
    await revealAdvancedOptionsIfNeeded();
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.field'), { target: { value: 'surface.id' } });
    expect(screen.getByRole('option', { name: 'Conversa de trabalho' })).toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'commandSettings.conditions.value' })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({ condition: { version: 1, clauses: [{ field: 'surface.id', value: 'chat-one' }, { field: 'surface.type', value: 'chat' }] } }) })));
  });

  it.each(['chat.model.open', 'workspace.create', 'workspace.chat.open', 'workspace.tab.chat.create', 'workspace.tab.close', CHAT_CLEAR_COMMAND, TERMINAL_INTERRUPT_COMMAND, 'chat.message.copy', 'chat.message.delete', 'editor.format.bold', 'editor.file.save'])('configura condição de aba pelo nome na paleta de %s', async (commandId) => {
    workspaceTabs = [{ id: 'chat-one', type: 'chat', title: 'Conversa de trabalho' }];
    getSettings.mockResolvedValue({ ...snapshot,
      commands: [{ ...snapshot.commands[0], id: commandId, allowedSources: ['palette'] }],
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined,
        commandId, triggerType: 'palette', triggerSpec: JSON.stringify({ version: 1, selection: commandId }) }],
    });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(within(await screen.findByRole('grid', { name: 'commandSettings.commands' })).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.actions.editBinding' }));
    await revealAdvancedOptionsIfNeeded();
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.field'), { target: { value: 'surface.id' } });
    expect(screen.getByRole('option', { name: 'Conversa de trabalho' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      binding: expect.objectContaining({ commandId, triggerType: 'palette',
        condition: { version: 1, clauses: [{ field: 'surface.id', value: 'chat-one' }, { field: 'surface.type', value: 'chat' }] } }),
    })));
  });

  it.each(['chat.model.open', 'workspace.tab.chat.create', 'chat.message.copy', 'editor.format.bold'])('preserva seleção da paleta ao editar condição e prioridade de supressão de %s', async (commandId) => {
    workspaceTabs = [{ id: 'chat-one', type: 'chat', title: 'Conversa de trabalho' }];
    const triggerSpec = JSON.stringify({ version: 1, selection: commandId });
    getSettings.mockResolvedValue({ ...snapshot,
      commands: [{ ...snapshot.commands[0], id: commandId, allowedSources: ['palette'] }],
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: 'palette-default',
        commandId: '', effect: 'suppress', triggerType: 'palette', triggerSpec }],
    });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(within(await screen.findByRole('grid', { name: 'commandSettings.commands' })).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.actions.editBinding' }));
    await revealAdvancedOptionsIfNeeded();
    fireEvent.change(screen.getByLabelText('commandSettings.form.priority'), { target: { value: '7' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.field'), { target: { value: 'surface.id' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      binding: expect.objectContaining({ effect: 'suppress', triggerSpec, resolutionPriority: 7,
        replacesDefaultId: 'palette-default',
        condition: { version: 1, clauses: [{ field: 'surface.id', value: 'chat-one' }, { field: 'surface.type', value: 'chat' }] } }),
    })));
  });

  it('não oferece condições visuais na paleta para ações backend', async () => {
    getSettings.mockResolvedValue({ ...snapshot,
      commands: [{ ...snapshot.commands[0], id: 'workspace.list', allowedSources: ['palette'] }],
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined,
        commandId: 'workspace.list', triggerType: 'palette', triggerSpec: '{"version":1,"selection":"workspace.list"}' }],
    });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(within(await screen.findByRole('grid', { name: 'commandSettings.commands' })).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.actions.editBinding' }));
    await revealAdvancedOptionsIfNeeded();
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    const fields = within(screen.getByLabelText('commandSettings.conditions.field')).getAllByRole('option');
    expect(fields.map(field => (field as HTMLOptionElement).value)).toEqual(['profile']);
  });

  it.each(['tasklists.duplicate', 'tasklists.delete', 'tasklists.clear', 'profiles.duplicate', 'profiles.delete', 'profiles.activate'])('salva tipo de página sem oferecer ID opaco em %s', async (commandId) => {
    getSettings.mockResolvedValue({ ...snapshot,
      commands: [{ ...snapshot.commands[0], id: commandId, allowedSources: ['palette'] }],
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined,
        commandId, triggerType: 'palette', triggerSpec: JSON.stringify({ version: 1, selection: commandId }) }],
    });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(await bindingAction('commandSettings.actions.editBinding'));
    await revealAdvancedOptionsIfNeeded();
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    const field = screen.getByLabelText('commandSettings.conditions.field');
    expect(within(field).getAllByRole('option').map(option => (option as HTMLOptionElement).value)).toEqual(['app.focused', 'surface.type', 'profile']);
    fireEvent.change(field, { target: { value: 'surface.type' } });
    const surface = commandId.startsWith('profiles.') ? 'profiles' : 'tasklists';
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.value'), { target: { value: surface } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({ commandId, condition: { version: 1, clauses: [{ field: 'surface.type', value: surface }] } }) })));
  });

  it('oferece condições visuais no Deck para ações local_ui e seleciona uma aba amigável', async () => {
    workspaceTabs = [{ id: 'friendly-tab', type: 'chat', title: 'Aba amigável' }];
    getSettings.mockResolvedValue({ ...snapshot,
      commands: [{ ...snapshot.commands[0], id: 'navigation.settings.open', allowedSources: ['streamdeck.key'] }],
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined,
        commandId: 'navigation.settings.open', triggerType: 'streamdeck.key', triggerSpec: '{"version":1,"device":"SERIAL","key":0}' }],
    });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(await bindingAction('commandSettings.actions.editBinding'));
    await revealAdvancedOptionsIfNeeded();
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.field'), { target: { value: 'surface.id' } });
    expect(screen.getByRole('option', { name: 'Aba amigável' })).toBeInTheDocument();
    const conditionValues = screen.getAllByLabelText('commandSettings.conditions.value');
    fireEvent.change(conditionValues[0], { target: { value: 'friendly-tab' } });
    expect((conditionValues[0] as HTMLSelectElement).value).toBe('friendly-tab');
  });

  it('configura programa e dispositivo no Deck sem expor serial como texto', async () => {
    getSettings.mockResolvedValue({ ...snapshot,
      commands: [{ ...snapshot.commands[0], allowedSources: ['streamdeck.key'] }],
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined,
        triggerType: 'streamdeck.key', triggerSpec: '{"version":1,"device":"PRIVATE-SERIAL","key":0}' }],
    });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    await act(async () => deckStatusChanged?.({ status: 'connected', devices: [{ id: 'PRIVATE-SERIAL', model: 'Stream Deck', keyCount: 15, status: 'connected' }] }));
    fireEvent.click(await bindingAction('commandSettings.actions.editBinding'));
    await revealAdvancedOptionsIfNeeded();
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.field'), { target: { value: 'foreground.process' } });
    expect(screen.getByText('commandSettings.foregroundProcessHint')).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.value'), { target: { value: 'obs64.exe' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    fireEvent.change(screen.getAllByLabelText('commandSettings.conditions.field')[1], { target: { value: 'device' } });
    expect(screen.getByRole('option', { name: 'Stream Deck' })).toHaveProperty('selected', true);
    expect(screen.queryByText('PRIVATE-SERIAL')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({ condition: {
      version: 1, clauses: [{ field: 'foreground.process', value: 'obs64.exe' }, { field: 'device', value: 'PRIVATE-SERIAL' }],
    } }) })));
  });

  it('preserva dispositivo desconectado sem substituí-lo pelo disponível', async () => {
    const condition = { version: 1, clauses: [{ field: 'device', value: 'DISCONNECTED' }] };
    getSettings.mockResolvedValue({ ...snapshot,
      commands: [{ ...snapshot.commands[0], allowedSources: ['streamdeck.key'] }],
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined,
        triggerType: 'streamdeck.key', triggerSpec: '{"version":1,"device":"DISCONNECTED","key":0}', condition }],
    });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    await act(async () => deckStatusChanged?.({ status: 'connected', devices: [{ id: 'OTHER', model: 'Stream Deck', keyCount: 15, status: 'connected' }] }));
    fireEvent.click(await bindingAction('commandSettings.actions.editBinding'));
    await revealAdvancedOptionsIfNeeded();
    expect(screen.getByRole('option', { name: 'commandSettings.unavailableConditionValue' })).toHaveProperty('selected', true);
    expect(screen.queryByText('DISCONNECTED')).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('commandSettings.form.priority'), { target: { value: '4' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({ condition }) })));
  });

  it('salva perfil por nome no binding de teclado local', async () => {
    getProfiles.mockResolvedValue([{ name: 'Programação', slug: 'programacao', description: '', icon: '', source: 'local' }]);
    mutateSettings.mockResolvedValue({ committed: true, published: true, id: 'binding-new' });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(await screen.findByRole('button', { name: 'commandSettings.actions.newBinding' }));
    await revealAdvancedOptionsIfNeeded();
    fireEvent.change(screen.getByLabelText('commandSettings.form.command'), { target: { value: 'cmd.new' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.field'), { target: { value: 'profile' } });
    expect(screen.getByRole('option', { name: 'Programação' })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('commandSettings.conditions.value'), { target: { value: 'programacao' } });
    const capture = screen.getByRole('button', { name: 'commandShortcutCapture.buttonLabel' });
    fireEvent.click(capture);
    fireEvent.keyDown(capture, { key: 'n', code: 'KeyN', ctrlKey: true, getModifierState: () => false });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'binding_create',
      binding: expect.objectContaining({ triggerType: 'keyboard.local', condition: { version: 1, clauses: [{ field: 'profile', value: 'programacao' }] } }),
    })));
  });

  it('preserva perfil removido e informa erro de catálogo com retry', async () => {
    const condition = { version: 1, clauses: [{ field: 'profile', value: 'removido' }] };
    getProfiles.mockRejectedValue(new Error('falha de catálogo'));
    getSettings.mockResolvedValue({ ...snapshot, bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined, condition }] });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(await bindingAction('commandSettings.actions.editBinding'));
    expect(screen.getByRole('option', { name: 'commandSettings.unavailableConditionValue' })).toBeDisabled();
    expect(screen.getByText('profiles.loadError')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'commandSettings.advancedOptions' })).toHaveAttribute('aria-expanded', 'true');
    expect(announce).toHaveBeenCalledWith('profiles.loadError', 'assertive');
    expect(screen.getByRole('button', { name: 'commandSettings.actions.reload' })).toBeEnabled();
    fireEvent.change(screen.getByLabelText('commandSettings.form.priority'), { target: { value: '8' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({ condition }) })));
  });

  it('preserva condição de aba ausente ao editar prioridade', async () => {
    const condition = { version: 1, clauses: [{ field: 'surface.type', value: 'chat' }, { field: 'surface.id', value: 'missing-chat' }] };
    getSettings.mockResolvedValue({ ...snapshot, bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined, condition }] });
    render(<CommandSettingsPage />);
    fireEvent.click(await screen.findByText('Minha camada'));
    fireEvent.click(within(await screen.findByRole('grid', { name: 'commandSettings.commands' })).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.actions.editBinding' }));
    expect(screen.getByRole('option', { name: 'commandSettings.unavailableConditionValue' })).toHaveProperty('selected', true);
    fireEvent.change(screen.getByLabelText('commandSettings.form.priority'), { target: { value: '9' } });
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(mutateSettings).toHaveBeenCalledWith(expect.objectContaining({ binding: expect.objectContaining({ condition, resolutionPriority: 9 }) })));
  });

  it('aceita sequência v2 existente em binding do escopo global', async () => {
    const sequence = '{"version":2,"steps":[{"code":"KeyK","modifiers":["Control"]},{"code":"KeyC","modifiers":[]}]}';
    getSettings.mockResolvedValue({
      ...snapshot,
      bindings: [{ ...snapshot.bindings[0], layerId: 'user', readOnly: false, defaultId: undefined, triggerSpec: sequence }],
    });
    render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    fireEvent.click(screen.getByText('Minha camada'));
    fireEvent.click(within(await screen.findByRole('grid', { name: 'commandSettings.commands' })).getByRole('button', { name: 'common.actions' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'commandSettings.actions.editBinding' }));
    expect(screen.getByLabelText('commandShortcutCapture.mode')).toHaveValue('sequence');
    expect(screen.getByRole('button', { name: 'common.save' })).toBeEnabled();
  });

  it('usa os componentes reais sem violações automáticas de acessibilidade', async () => {
    const { container } = render(<CommandSettingsPage />);
    await screen.findByText('Minha camada');
    // jsdom não mede cores/layout; contraste permanece no aceite visual dos temas.
    expect(await axe(container, { rules: { 'color-contrast': { enabled: false } } })).toHaveNoViolations();
  });
});
