/** @vitest-environment jsdom */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useState } from 'react';
import type { TFunction } from 'i18next';

import type { SignalForm } from '../components/channels';
import { useSignalChannelController } from './useSignalChannelController';

const mockSignalCheckAPI = vi.fn();
const mockSignalListAccounts = vi.fn();
const mockSignalRegister = vi.fn();
const mockSignalVerify = vi.fn();
const mockSignalLink = vi.fn();
const mockSignalUnregister = vi.fn();

vi.mock('@wailsjs/go/wailsapi/Signal', () => ({
  SignalCheckAPI: (...args: unknown[]) => mockSignalCheckAPI(...args),
  SignalListAccounts: (...args: unknown[]) => mockSignalListAccounts(...args),
  SignalRegister: (...args: unknown[]) => mockSignalRegister(...args),
  SignalVerify: (...args: unknown[]) => mockSignalVerify(...args),
  SignalLink: (...args: unknown[]) => mockSignalLink(...args),
  SignalUnregister: (...args: unknown[]) => mockSignalUnregister(...args),
}));

const addToastMock = vi.fn(() => 'toast-id');
const announceMock = vi.fn();
const requestConfirmMock = vi.fn();

const t = ((key: string, options?: Record<string, unknown>) => {
  if (key === 'channels.signal.apiInfo') {
    return `API ${options?.version} ${options?.build}`;
  }
  if (key === 'channels.signal.apiAccounts') {
    return `accounts ${options?.accounts}`;
  }
  if (key === 'channels.announce.deviceLinked') {
    return `linked ${options?.account}`;
  }
  if (key === 'channels.announce.codeSent') {
    return `sent ${options?.mode} ${options?.account}`;
  }
  if (key === 'channels.announce.accountRemoved') {
    return `removed ${options?.account}`;
  }
  return key;
}) as TFunction;

const getErrorMessage = (error: unknown) => error instanceof Error ? error.message : String(error ?? '');

const baseForm: SignalForm = {
  enabled: false,
  apiURL: 'http://signal-api.local',
  account: '+5511999999999',
  apiToken: ' token ',
  profile: 'canais-comunicacao',
  maxHistory: 50,
  maxContacts: 1,
};

function renderController(initialForm: SignalForm = baseForm) {
  return renderHook(() => {
    const [signalForm, setSignalForm] = useState(initialForm);
    const controller = useSignalChannelController({
      signalForm,
      setSignalForm,
      addToast: addToastMock,
      announce: announceMock,
      requestConfirm: requestConfirmMock,
      t,
      getErrorMessage,
    });
    return { ...controller, signalForm };
  });
}

describe('useSignalChannelController', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-02T12:00:00Z'));
    mockSignalCheckAPI.mockResolvedValue({ version: '1.2.3', build: 2, mode: 'native', versions: ['v1', 'v2'], capabilities: {} });
    mockSignalListAccounts.mockResolvedValue([]);
    mockSignalRegister.mockResolvedValue(undefined);
    mockSignalVerify.mockResolvedValue(undefined);
    mockSignalLink.mockResolvedValue('data:image/png;base64,qr');
    mockSignalUnregister.mockResolvedValue(undefined);
    requestConfirmMock.mockResolvedValue(true);
    addToastMock.mockClear();
    announceMock.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('checa a API, carrega contas e seleciona a primeira conta quando o formulário está vazio', async () => {
    mockSignalListAccounts.mockResolvedValue(['+5511888888888']);
    const { result } = renderController({ ...baseForm, account: '' });

    await act(async () => {
      await result.current.handleSignalCheckAPI();
    });

    expect(mockSignalCheckAPI).toHaveBeenCalledWith('http://signal-api.local', 'token');
    expect(mockSignalListAccounts).toHaveBeenCalledWith('http://signal-api.local', 'token');
    expect(result.current.signalAPIReady).toBe(true);
    expect(result.current.signalAccounts).toEqual(['+5511888888888']);
    expect(result.current.signalForm.account).toBe('+5511888888888');
    expect(result.current.signalAPIInfo).toContain('accounts +5511888888888');
    expect(announceMock).toHaveBeenCalledWith(expect.stringContaining('accounts +5511888888888'));
  });

  it('registra por SMS e verifica o código informado', async () => {
    const { result } = renderController();

    await act(async () => {
      await result.current.handleSignalRegister('sms');
    });

    expect(mockSignalRegister).toHaveBeenCalledWith(
      'http://signal-api.local',
      '+5511999999999',
      'sms',
      '',
      'token'
    );
    expect(result.current.signalRegStep).toBe('awaiting_code');
    expect(result.current.signalSmsSent).toBe(true);

    act(() => {
      result.current.setSignalRegCode('123456');
    });

    await act(async () => {
      await result.current.handleSignalVerify();
    });

    expect(mockSignalVerify).toHaveBeenCalledWith(
      'http://signal-api.local',
      '+5511999999999',
      '123456',
      'token'
    );
    expect(result.current.signalRegStep).toBe('done');
    expect(result.current.signalSmsSent).toBe(false);
  });

  it('gera QR code e encerra o polling quando uma conta aparece', async () => {
    mockSignalListAccounts
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(['+5511777777777']);
    const { result } = renderController({ ...baseForm, account: '' });

    await act(async () => {
      await result.current.handleSignalLink();
    });

    expect(mockSignalLink).toHaveBeenCalledWith('http://signal-api.local', 'chat.assistant', 'token');
    expect(result.current.signalLinkQR).toBe('data:image/png;base64,qr');
    expect(result.current.signalLinking).toBe(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(result.current.signalLinking).toBe(false);
    expect(result.current.signalAccounts).toEqual(['+5511777777777']);
    expect(result.current.signalForm.account).toBe('+5511777777777');
    expect(result.current.signalLinkQR).toBe('');
    expect(announceMock).toHaveBeenCalledWith('linked +5511777777777');
  });

  it('expõe erro assertivo quando o polling de link expira', async () => {
    mockSignalListAccounts.mockResolvedValue([]);
    const { result } = renderController();

    await act(async () => {
      await result.current.handleSignalLink();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(130000);
    });

    expect(result.current.signalLinking).toBe(false);
    expect(result.current.signalRegError).toBe('channels.error.signalLinkTimeoutDetails');
    expect(announceMock).toHaveBeenCalledWith('channels.announce.linkTimeout', 'assertive');
  });

  it('confirma unregister, atualiza contas e limpa a conta removida', async () => {
    mockSignalListAccounts.mockResolvedValue(['+5511666666666']);
    const { result } = renderController();

    await act(async () => {
      await result.current.handleSignalUnregister('+5511999999999');
    });

    expect(requestConfirmMock).toHaveBeenCalledWith(expect.objectContaining({
      title: 'channels.confirm.removeSignalAccountTitle',
      variant: 'danger',
    }));
    expect(mockSignalUnregister).toHaveBeenCalledWith(
      'http://signal-api.local',
      '+5511999999999',
      true,
      'token'
    );
    expect(result.current.signalAccounts).toEqual(['+5511666666666']);
    expect(result.current.signalForm.account).toBe('+5511666666666');
    expect(announceMock).toHaveBeenCalledWith('removed +5511999999999');
  });
});
