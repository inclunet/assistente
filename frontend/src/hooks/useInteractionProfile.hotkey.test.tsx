/** @vitest-environment jsdom */
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  getProfile: vi.fn(), getActiveProfileAndSlug: vi.fn(),
  init: vi.fn<() => Promise<boolean>>(), start: vi.fn(), stop: vi.fn(), cancel: vi.fn(), noop: vi.fn(),
}));
vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({ GetProfile: mocks.getProfile, GetActiveProfileAndSlug: mocks.getActiveProfileAndSlug }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: () => () => undefined }));
vi.mock('./useSTT', () => ({ useSTT: () => ({
  isListening: false, isRecording: false, isProcessing: false, isInitialized: false,
  volume: 0, interimText: '', startRecording: mocks.start, stopRecording: mocks.stop,
  cancelRecording: mocks.cancel, setMode: mocks.noop, setProvider: mocks.noop,
  setLanguage: mocks.noop, updateConfig: mocks.noop, init: mocks.init,
}) }));
vi.mock('./useWakewordDetection', () => ({ useWakewordDetection: () => ({
  isListening: false, startListening: mocks.noop, stopListening: mocks.noop, lastRecognizedText: '',
}) }));
vi.mock('../services/tts', () => ({ ttsService: {
  stop: mocks.noop, clearAllRoleConfigs: mocks.noop, setEnabled: mocks.noop,
  setAutoRead: mocks.noop, setEnabledForUser: mocks.noop,
} }));
vi.mock('../services/audioFeedback', () => ({ playSound: mocks.noop, SOUND_TYPES: {} }));

import { useInteractionProfile } from './useInteractionProfile';
import { captureVoiceHotkeyTarget } from '../services/voiceHotkeyRouting';
import { registerOpenModal, unregisterOpenModal } from '../lib/modalRegistry';

const profile = (name = 'Voz', enabled = true) => ({ name, input: { enabled, triggers: [], stt_provider: 'webspeech' }, voice: {} });
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
const event = { triggerType: 'hotkey', bringToFront: false };
const capture = () => captureVoiceHotkeyTarget('padrao', event);

beforeEach(() => {
  vi.clearAllMocks();
  mocks.getProfile.mockReset().mockResolvedValue(profile());
  mocks.getActiveProfileAndSlug.mockReset().mockResolvedValue({ profile: profile(), slug: 'padrao' });
  mocks.init.mockReset().mockResolvedValue(true);
});
afterEach(() => {
  cleanup();
  document.querySelectorAll('.modal-overlay').forEach((node) => node.remove());
  unregisterOpenModal('pending-voice-modal');
});

describe('useInteractionProfile — lifecycle da hotkey', () => {
  it('aciona somente a instância ativa, não a última montada', async () => {
    const active = renderHook(() => useInteractionProfile({ isHotkeyEligible: () => true }));
    const inactive = renderHook(() => useInteractionProfile({ isHotkeyEligible: () => false }));
    await waitFor(() => expect(active.result.current.isLoading).toBe(false));
    await waitFor(() => expect(inactive.result.current.isLoading).toBe(false));
    await act(async () => { expect(capture()?.execute()).toBe(true); });
    expect(mocks.start).toHaveBeenCalledTimes(1);
    active.unmount();
    expect(capture()).toBeUndefined();
  });

  it.each(['inativo', 'reativado', 'desmontado'] as const)('não inicia gravação pendente depois de ficar %s', async (state) => {
    const pending = deferred<boolean>();
    mocks.init.mockReturnValue(pending.promise);
    const hook = renderHook(({ eligible }) => useInteractionProfile({ isHotkeyEligible: () => eligible }), { initialProps: { eligible: true } });
    await waitFor(() => expect(hook.result.current.isLoading).toBe(false));
    const target = capture();
    act(() => { expect(target?.execute()).toBe(true); });
    expect(mocks.init).toHaveBeenCalledOnce();
    if (state === 'desmontado') hook.unmount();
    else {
      hook.rerender({ eligible: false });
      if (state === 'reativado') hook.rerender({ eligible: true });
    }
    await act(async () => { pending.resolve(true); await pending.promise; });
    expect(mocks.start).not.toHaveBeenCalled();
  });

  it('recusa entrada de voz desabilitada no perfil efetivo', async () => {
    mocks.getActiveProfileAndSlug.mockResolvedValue({ profile: profile('Sem voz', false), slug: 'padrao' });
    const hook = renderHook(() => useInteractionProfile({ isHotkeyEligible: () => true }));
    await waitFor(() => expect(hook.result.current.isLoading).toBe(false));
    expect(capture()).toBeUndefined();
    expect(mocks.init).not.toHaveBeenCalled();
  });

  it('resposta atrasada de outro perfil não substitui o perfil atual', async () => {
    const old = deferred<ReturnType<typeof profile>>();
    mocks.getProfile.mockImplementation((slug: string) => slug === 'antigo' ? old.promise : Promise.resolve(profile('Atual')));
    const hook = renderHook(({ slug }) => useInteractionProfile({ effectiveProfileSlug: slug, isHotkeyEligible: () => true }), { initialProps: { slug: 'antigo' } });
    hook.rerender({ slug: 'atual' });
    await waitFor(() => expect(hook.result.current.activeProfile?.name).toBe('Atual'));
    await act(async () => { old.resolve(profile('Antigo')); await old.promise; });
    expect(hook.result.current.activeProfile?.name).toBe('Atual');
    expect(hook.result.current.isLoading).toBe(false);
  });

  it('cancelar interação não deixa carregamento de perfil preso', async () => {
    const pending = deferred<ReturnType<typeof profile>>();
    mocks.getActiveProfileAndSlug.mockReturnValue(pending.promise.then((loadedProfile) => ({ profile: loadedProfile, slug: 'padrao' })));
    const hook = renderHook(() => useInteractionProfile({ isHotkeyEligible: () => true }));
    act(() => { hook.result.current.cancelInteraction(); });
    await act(async () => { pending.resolve(profile()); await pending.promise; });
    expect(hook.result.current.isLoading).toBe(false);
    expect(hook.result.current.activeProfile?.name).toBe('Voz');
  });

  it('não começa a gravar se um modal abre durante a inicialização', async () => {
    const pending = deferred<boolean>();
    mocks.init.mockReturnValue(pending.promise);
    const hook = renderHook(() => useInteractionProfile({ isHotkeyEligible: () => true }));
    await waitFor(() => expect(hook.result.current.isLoading).toBe(false));
    const target = capture();
    act(() => { expect(target?.execute()).toBe(true); });
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.appendChild(overlay);
    registerOpenModal('pending-voice-modal');
    await act(async () => { pending.resolve(true); await pending.promise; });
    expect(mocks.start).not.toHaveBeenCalled();
  });
});
