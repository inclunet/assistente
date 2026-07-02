import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react';
import type { TFunction } from 'i18next';
import {
  SignalCheckAPI,
  SignalListAccounts,
  SignalRegister,
  SignalVerify,
  SignalLink,
  SignalUnregister,
} from '@wailsjs/go/app/App';
import type { AddToastOptions, ToastAction } from '../store/uiStore';
import type { ConfirmOptions } from '../store/confirmStore';
import type {
  SignalConnectionMode,
  SignalForm,
  SignalRegisterStep,
} from '../components/channels';

type AddToast = (
  message: string,
  type: 'success' | 'error' | 'warning' | 'info',
  duration?: number,
  action?: ToastAction,
  options?: AddToastOptions,
) => string;

type Announce = (message: string, priority?: 'polite' | 'assertive') => void;

interface UseSignalChannelControllerOptions {
  signalForm: SignalForm;
  setSignalForm: Dispatch<SetStateAction<SignalForm>>;
  addToast: AddToast;
  announce: Announce;
  requestConfirm: (options: ConfirmOptions) => Promise<boolean>;
  t: TFunction;
  getErrorMessage: (error: unknown) => string;
}

export function useSignalChannelController({
  signalForm,
  setSignalForm,
  addToast,
  announce,
  requestConfirm,
  t,
  getErrorMessage,
}: UseSignalChannelControllerOptions) {
  const [signalRegStep, setSignalRegStep] = useState<SignalRegisterStep>('idle');
  const [signalRegCode, setSignalRegCode] = useState('');
  const [signalRegCaptcha, setSignalRegCaptcha] = useState('');
  const [signalRegError, setSignalRegError] = useState('');
  const [signalSmsSent, setSignalSmsSent] = useState(false);
  const [signalCheckingAPI, setSignalCheckingAPI] = useState(false);
  const [signalAPIInfo, setSignalAPIInfo] = useState('');
  const [signalAPIReady, setSignalAPIReady] = useState(false);
  const [signalAccounts, setSignalAccounts] = useState<string[]>([]);
  const [signalConnectionMode, setSignalConnectionMode] = useState<SignalConnectionMode>('register');
  const [signalLinkQR, setSignalLinkQR] = useState('');
  const [signalLinking, setSignalLinking] = useState(false);
  const [signalUnregistering, setSignalUnregistering] = useState<string | null>(null);
  const linkPollRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const stopLinkPolling = useCallback(() => {
    if (linkPollRef.current) {
      clearTimeout(linkPollRef.current);
      linkPollRef.current = null;
    }
  }, []);

  const startLinkPolling = useCallback(function poll(startTime: number) {
    const POLL_TIMEOUT_MS = 2 * 60 * 1000;
    linkPollRef.current = setTimeout(async () => {
      if (Date.now() - startTime > POLL_TIMEOUT_MS) {
        setSignalLinking(false);
        const linkTimeoutMessage = t('channels.announce.linkTimeout');
        setSignalRegError(t('channels.error.signalLinkTimeoutDetails'));
        addToast(linkTimeoutMessage, 'error', undefined, undefined, { suppressAnnounce: true });
        announce(linkTimeoutMessage, 'assertive');
        return;
      }
      try {
        const apiToken = signalForm.apiToken.trim();
        const accounts = await SignalListAccounts(signalForm.apiURL, apiToken);
        if (accounts && accounts.length > 0) {
          setSignalAccounts(accounts);
          if (!signalForm.account) {
            setSignalForm((prev) => ({ ...prev, account: accounts[0] }));
          }
          setSignalLinking(false);
          setSignalLinkQR('');
          const deviceLinkedMessage = t('channels.announce.deviceLinked', { account: accounts[0] });
          addToast(deviceLinkedMessage, 'success', undefined, undefined, { suppressAnnounce: true });
          announce(deviceLinkedMessage);
          return;
        }
      } catch { /* polling */ }
      poll(startTime);
    }, 5000);
  }, [addToast, announce, setSignalForm, signalForm.account, signalForm.apiToken, signalForm.apiURL, t]);

  useEffect(() => {
    return () => stopLinkPolling();
  }, [stopLinkPolling]);

  const handleSignalCheckAPI = useCallback(async () => {
    if (!signalForm.apiURL) {
      addToast(t('channels.error.signalApiUrlRequired'), 'error');
      return;
    }
    const apiToken = signalForm.apiToken.trim();
    setSignalCheckingAPI(true);
    setSignalAPIInfo('');
    setSignalAPIReady(false);
    setSignalRegError('');
    setSignalAccounts([]);
    try {
      const [info, accounts] = await Promise.all([
        SignalCheckAPI(signalForm.apiURL, apiToken),
        SignalListAccounts(signalForm.apiURL, apiToken).catch(() => [] as string[]),
      ]);
      setSignalAccounts(accounts || []);
      let infoText = t('channels.signal.apiInfo', { version: info['version'] || '?', build: info['build'] || '?' });
      if (accounts && accounts.length > 0) {
        infoText += ` ${t('channels.signal.apiAccounts', { accounts: accounts.join(', ') })}`;
        if (!signalForm.account) {
          setSignalForm((prev) => ({ ...prev, account: accounts[0] }));
        }
      } else {
        infoText += ` ${t('channels.signal.apiNoAccounts')}`;
      }
      setSignalAPIInfo(infoText);
      setSignalAPIReady(true);
      // suppressAnnounce: anunciamos infoText (mais rico) logo abaixo.
      addToast(t('channels.toast.signalApiAccessible'), 'success', undefined, undefined, { suppressAnnounce: true });
      announce(infoText);
    } catch (error: unknown) {
      setSignalAPIInfo('');
      const msg = getErrorMessage(error) || t('channels.error.signalApiConnectFailed');
      setSignalRegError(msg);
      setSignalAPIReady(false);
      addToast(msg, 'error', undefined, undefined, { suppressAnnounce: true });
      announce(msg, 'assertive');
    } finally {
      setSignalCheckingAPI(false);
    }
  }, [addToast, announce, getErrorMessage, setSignalForm, signalForm.account, signalForm.apiToken, signalForm.apiURL, t]);

  const handleSignalRegister = useCallback(async (mode: 'sms' | 'voice' = 'sms') => {
    if (!signalForm.account || !signalForm.apiURL) {
      const msg = t('channels.error.signalAccountAndUrlRequired');
      setSignalRegError(msg);
      addToast(msg, 'error', undefined, undefined, { suppressAnnounce: true });
      announce(msg, 'assertive');
      return;
    }
    const apiToken = signalForm.apiToken.trim();
    setSignalRegStep('registering');
    setSignalRegError('');
    try {
      await SignalRegister(signalForm.apiURL, signalForm.account, mode, signalRegCaptcha, apiToken);
      setSignalRegStep('awaiting_code');
      setSignalRegCaptcha('');
      if (mode === 'sms') setSignalSmsSent(true);
      const modeLabel = mode === 'voice' ? t('channels.signal.modeVoice') : t('channels.signal.modeSms');
      const codeSentMessage = t('channels.announce.codeSent', { mode: modeLabel, account: signalForm.account });
      addToast(codeSentMessage, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(codeSentMessage);
    } catch (error: unknown) {
      setSignalRegStep(signalSmsSent ? 'awaiting_code' : 'idle');
      const msg = getErrorMessage(error) || t('channels.error.signalRegisterFailed');
      setSignalRegError(msg);
      addToast(msg, 'error', undefined, undefined, { suppressAnnounce: true });
      announce(msg, 'assertive');
    }
  }, [
    addToast,
    announce,
    getErrorMessage,
    signalForm.account,
    signalForm.apiToken,
    signalForm.apiURL,
    signalRegCaptcha,
    signalSmsSent,
    t,
  ]);

  const handleSignalVerify = useCallback(async () => {
    if (!signalRegCode) {
      const msg = t('channels.error.verificationCodeRequired');
      setSignalRegError(msg);
      announce(msg, 'assertive');
      return;
    }
    const apiToken = signalForm.apiToken.trim();
    setSignalRegStep('verifying');
    setSignalRegError('');
    try {
      await SignalVerify(signalForm.apiURL, signalForm.account, signalRegCode, apiToken);
      setSignalRegStep('done');
      setSignalSmsSent(false);
      const numberVerifiedMessage = t('channels.announce.numberVerified');
      addToast(numberVerifiedMessage, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(numberVerifiedMessage);
    } catch (error: unknown) {
      setSignalRegStep('awaiting_code');
      const msg = getErrorMessage(error) || t('channels.error.signalVerifyFailed');
      setSignalRegError(msg);
      addToast(msg, 'error', undefined, undefined, { suppressAnnounce: true });
      announce(msg, 'assertive');
    }
  }, [addToast, announce, getErrorMessage, signalForm.account, signalForm.apiToken, signalForm.apiURL, signalRegCode, t]);

  const handleSignalLink = useCallback(async () => {
    if (!signalForm.apiURL) {
      const msg = t('channels.error.signalApiUrlRequired');
      setSignalRegError(msg);
      announce(msg, 'assertive');
      return;
    }
    const apiToken = signalForm.apiToken.trim();
    setSignalLinkQR('');
    setSignalRegError('');
    setSignalLinking(true);
    stopLinkPolling();
    try {
      const qr = await SignalLink(signalForm.apiURL, t('common.chat.assistant'), apiToken);
      setSignalLinkQR(qr);
      announce(t('channels.announce.qrGenerated'));
      startLinkPolling(Date.now());
    } catch (error: unknown) {
      const errorMessage = getErrorMessage(error) || t('channels.error.signalLinkQrFailedDetailed');
      setSignalRegError(errorMessage);
      addToast(errorMessage, 'error', undefined, undefined, { suppressAnnounce: true });
      announce(errorMessage, 'assertive');
      setSignalLinking(false);
    }
  }, [addToast, announce, getErrorMessage, signalForm.apiToken, signalForm.apiURL, startLinkPolling, stopLinkPolling, t]);

  const handleSignalUnregister = useCallback(async (account: string) => {
    const shouldRemove = await requestConfirm({
      title: t('channels.confirm.removeSignalAccountTitle'),
      message: t('channels.confirm.removeSignalAccountMessage', { account }),
      confirmText: t('channels.confirm.removeSignalAccountConfirm'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!shouldRemove) return;
    setSignalUnregistering(account);
    try {
      const apiToken = signalForm.apiToken.trim();
      await SignalUnregister(signalForm.apiURL, account, true, apiToken);
      const accounts = await SignalListAccounts(signalForm.apiURL, apiToken).catch(() => [] as string[]);
      setSignalAccounts(accounts || []);
      if (signalForm.account === account) {
        setSignalForm((prev) => ({ ...prev, account: accounts?.[0] || '' }));
      }
      const accountRemovedMessage = t('channels.announce.accountRemoved', { account });
      addToast(accountRemovedMessage, 'success', undefined, undefined, { suppressAnnounce: true });
      announce(accountRemovedMessage);
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('channels.error.removeAccountFailed'), 'error');
    } finally {
      setSignalUnregistering(null);
    }
  }, [
    addToast,
    announce,
    getErrorMessage,
    requestConfirm,
    setSignalForm,
    signalForm.account,
    signalForm.apiToken,
    signalForm.apiURL,
    t,
  ]);

  return {
    signalRegStep,
    signalRegCode,
    signalRegCaptcha,
    signalRegError,
    signalSmsSent,
    signalCheckingAPI,
    signalAPIInfo,
    signalAPIReady,
    signalAccounts,
    signalConnectionMode,
    signalLinkQR,
    signalLinking,
    signalUnregistering,
    setSignalRegStep,
    setSignalRegCode,
    setSignalRegCaptcha,
    setSignalRegError,
    setSignalSmsSent,
    setSignalCheckingAPI,
    setSignalAPIInfo,
    setSignalAPIReady,
    setSignalAccounts,
    setSignalConnectionMode,
    setSignalLinkQR,
    setSignalLinking,
    stopLinkPolling,
    handleSignalCheckAPI,
    handleSignalRegister,
    handleSignalVerify,
    handleSignalLink,
    handleSignalUnregister,
  };
}
