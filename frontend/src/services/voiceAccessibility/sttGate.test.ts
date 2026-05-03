import { beforeEach, describe, expect, it, vi } from 'vitest';
import { registerVoiceAccessibilityActiveResolver } from './announcerBroker';
import {
  cancelInactiveSTTSession,
  canStartSTT,
  finishSTTSession,
  requestSTTStart,
  resetSTTGateForTests,
} from './sttGate';

describe('sttGate', () => {
  let unregisterResolver: (() => void) | undefined;

  beforeEach(() => {
    unregisterResolver?.();
    unregisterResolver = undefined;
    resetSTTGateForTests();
  });

  it('bloqueia início de STT em origem inativa', () => {
    unregisterResolver = registerVoiceAccessibilityActiveResolver((origin) => origin?.tabId === 'active');

    expect(canStartSTT({ tabId: 'inactive' })).toBe(false);
    expect(requestSTTStart({ origin: { tabId: 'inactive' }, cancel: vi.fn() })).toBe(false);
  });

  it('cancela sessão anterior quando outra origem ativa inicia', () => {
    const cancelFirst = vi.fn();

    expect(requestSTTStart({ origin: { tabId: 'tab-1' }, cancel: cancelFirst })).toBe(true);
    expect(requestSTTStart({ origin: { tabId: 'tab-2' }, cancel: vi.fn() })).toBe(true);

    expect(cancelFirst).toHaveBeenCalled();
  });

  it('cancela sessão que ficou inativa', () => {
    let activeTabId = 'tab-1';
    const cancel = vi.fn();
    unregisterResolver = registerVoiceAccessibilityActiveResolver((origin) => origin?.tabId === activeTabId);

    requestSTTStart({ origin: { tabId: 'tab-1' }, cancel });
    activeTabId = 'tab-2';
    cancelInactiveSTTSession();

    expect(cancel).toHaveBeenCalled();
  });

  it('finaliza somente a sessão correspondente', () => {
    const cancel = vi.fn();

    requestSTTStart({ origin: { tabId: 'tab-1' }, cancel });
    finishSTTSession({ tabId: 'tab-2' });
    cancelInactiveSTTSession();

    expect(cancel).not.toHaveBeenCalled();
  });
});
