import { logger } from '../utils/logger';
import { useState, useEffect, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DownOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import {
  GetChannelConfig,
  SaveChannelConfig,
  GetMessagingStatus,
  SignalCheckAPI,
  SignalListAccounts,
  SignalRegister,
  SignalVerify,
  SignalLink,
  SignalUnregister,
  RestartChannel,
  GetChannelTemplates,
  ListCredentials,
  UpsertCredential,
  DeleteCredential,
} from '@wailsjs/go/app/App';
import { channels } from '../../wailsjs/go/models';
import { useUIStore } from '../store/uiStore';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { Button } from '../components';
import { ChannelsTelegramSection, ChannelsSignalSection, ChannelsSlackSection } from '../components/channels';
import { Toolbar, ToolbarButton } from '../components/ui/Toolbar';
import { DataGrid, type DataGridColumn } from '../components/ui/DataGrid';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { EditorPanelFields, EditorPanelFooter } from '../components/ui/EditorPanel';
import { ContextMenu, MenuItem } from '../components/menu';
import { MenuButton } from '../components/layout/MenuButton';
import CreateChannelModal from '../components/modals/CreateChannelModal';
import { useConfirm } from '../hooks/useConfirm';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import './ChannelsPage.css';

// ─── Types ───────────────────────────────────────────────────────────

interface ChannelRow {
  id: string;
  name: string;
  label: string;
  enabled: boolean;
  status: string;
}

interface CredentialSummary {
  pattern: string;
  type: string;
  masked: string;
}

interface TelegramForm {
  enabled: boolean;
  botToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

interface SignalForm {
  enabled: boolean;
  apiURL: string;
  account: string;
  apiToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

interface SlackForm {
  enabled: boolean;
  botToken: string;
  appToken: string;
  profile: string;
  maxHistory: number;
  maxContacts: number;
}

type SignalRegisterStep = 'idle' | 'registering' | 'awaiting_code' | 'verifying' | 'done';

// ─── Component ───────────────────────────────────────────────────────

export default function ChannelsPage() {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const { announce } = useAnnouncer();
  const { handleGridReady: channelsHandleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'channels-page' });
  const defaultChannelProfile = 'canais-comunicacao';
  const requestConfirm = useConfirm();
  const getErrorMessage = (error: unknown) =>
    error instanceof Error ? error.message : String(error ?? '');

  const channelCredentialPattern = useCallback((channel: string, key: string) => `channel:${channel}:${key}`, []);

  // ── Loading ──────────────────────────────────────────────────────
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);

  // ── Channels grid ────────────────────────────────────────────────
  const [channelRows, setChannelRows] = useState<ChannelRow[]>([]);
  const [editingChannel, setEditingChannel] = useState<string | null>(null);
  const [focusedChannel, setFocusedChannel] = useState<ChannelRow | null>(null);

  // ── Channel forms ────────────────────────────────────────────────
  const [telegramForm, setTelegramForm] = useState<TelegramForm>({
    enabled: false, botToken: '', profile: '', maxHistory: 50, maxContacts: 1,
  });
  const [signalForm, setSignalForm] = useState<SignalForm>({
    enabled: false, apiURL: '', account: '', apiToken: '', profile: '', maxHistory: 50, maxContacts: 1,
  });
  const [slackForm, setSlackForm] = useState<SlackForm>({
    enabled: false, botToken: '', appToken: '', profile: '', maxHistory: 50, maxContacts: 1,
  });

  const [credentialSummaries, setCredentialSummaries] = useState<Record<string, CredentialSummary>>({});
  const [telegramUseVault, setTelegramUseVault] = useState(true);
  const [signalUseVault, setSignalUseVault] = useState(true);
  const [slackUseVault, setSlackUseVault] = useState(true);

  // ── Signal registration ──────────────────────────────────────────
  const [signalRegStep, setSignalRegStep] = useState<SignalRegisterStep>('idle');
  const [signalRegCode, setSignalRegCode] = useState('');
  const [signalRegCaptcha, setSignalRegCaptcha] = useState('');
  const [signalRegError, setSignalRegError] = useState('');
  const [signalSmsSent, setSignalSmsSent] = useState(false);
  const [signalCheckingAPI, setSignalCheckingAPI] = useState(false);
  const [signalAPIInfo, setSignalAPIInfo] = useState('');
  const [signalAPIReady, setSignalAPIReady] = useState(false);
  const [signalAccounts, setSignalAccounts] = useState<string[]>([]);
  const [signalConnectionMode, setSignalConnectionMode] = useState<'register' | 'link'>('register');
  const [signalLinkQR, setSignalLinkQR] = useState('');
  const [signalLinking, setSignalLinking] = useState(false);
  const [signalUnregistering, setSignalUnregistering] = useState<string | null>(null);
  const linkPollRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // ── Create Channel Modal ─────────────────────────────────────────
  const [showCreateChannelModal, setShowCreateChannelModal] = useState(false);
  const [createModalTemplateType, setCreateModalTemplateType] = useState<string | null>(null);
  const [channelTemplates, setChannelTemplates] = useState<channels.ChannelTemplate[]>([]);
  const [createMenuVisible, setCreateMenuVisible] = useState(false);
  const [createMenuPosition, setCreateMenuPosition] = useState({ x: 0, y: 0 });
  const createMenuButtonRef = useRef<HTMLButtonElement | null>(null);

  // ── Load ─────────────────────────────────────────────────────────

  const loadTemplates = useCallback(async () => {
    try {
      const result = await GetChannelTemplates();
      setChannelTemplates(result || []);
    } catch (error) {
      logger.error('Erro ao carregar templates de canal:', error);
    }
  }, []);

  const loadAll = useCallback(async () => {
    setLoading(true);
    try {
      const [telegramCfg, signalCfg, slackCfg, status, credentialsList] = await Promise.all([
        GetChannelConfig('telegram'),
        GetChannelConfig('signal'),
        GetChannelConfig('slack'),
        GetMessagingStatus(),
        ListCredentials().catch(() => [] as CredentialSummary[]),
      ]);

      const tgEnabled = telegramCfg?.enabled || false;
      const sigEnabled = signalCfg?.enabled || false;
      const slackEnabled = slackCfg?.enabled || false;

      if (telegramCfg) {
        setTelegramForm({
          enabled: tgEnabled,
          botToken: telegramCfg.bot_token || '',
          profile: telegramCfg.profile || defaultChannelProfile,
          maxHistory: telegramCfg.max_history || 50,
          maxContacts: telegramCfg.max_contacts || 1,
        });
      }
      if (signalCfg) {
        setSignalForm({
          enabled: sigEnabled,
          apiURL: signalCfg.api_url || '',
          account: signalCfg.account || '',
          apiToken: signalCfg.api_token || '',
          profile: signalCfg.profile || defaultChannelProfile,
          maxHistory: signalCfg.max_history || 50,
          maxContacts: signalCfg.max_contacts || 1,
        });
      }
      if (slackCfg) {
        setSlackForm({
          enabled: slackEnabled,
          botToken: slackCfg.bot_token || '',
          appToken: slackCfg.app_token || '',
          profile: slackCfg.profile || defaultChannelProfile,
          maxHistory: slackCfg.max_history || 50,
          maxContacts: slackCfg.max_contacts || 1,
        });
      }

      const summaryMap: Record<string, CredentialSummary> = {};
      for (const entry of credentialsList || []) {
        if (entry?.pattern) {
          summaryMap[entry.pattern] = entry;
        }
      }
      setCredentialSummaries(summaryMap);

      const telegramPattern = channelCredentialPattern('telegram', 'bot_token');
      const signalTokenPattern = channelCredentialPattern('signal', 'api_token');
      const slackBotPattern = channelCredentialPattern('slack', 'bot_token');
      const slackAppPattern = channelCredentialPattern('slack', 'app_token');

      const telegramStored = Boolean(summaryMap[telegramPattern] || telegramCfg?.bot_token_ref);
      const signalStored = Boolean(summaryMap[signalTokenPattern] || signalCfg?.api_token_ref);
      const slackStored = Boolean(
        summaryMap[slackBotPattern] || summaryMap[slackAppPattern] || slackCfg?.bot_token_ref || slackCfg?.app_token_ref
      );

      setTelegramUseVault(telegramStored || !telegramCfg?.bot_token);
      setSignalUseVault(signalStored || !signalCfg?.api_token);
      setSlackUseVault(slackStored || (!slackCfg?.bot_token && !slackCfg?.app_token));

      setChannelRows([
        {
          id: 'telegram',
          name: 'telegram',
          label: 'Telegram',
          enabled: tgEnabled,
          status: status['telegram'] || t('channels.status.disconnected'),
        },
        {
          id: 'signal',
          name: 'signal',
          label: 'Signal',
          enabled: sigEnabled,
          status: status['signal'] || t('channels.status.disconnected'),
        },
        {
          id: 'slack',
          name: 'slack',
          label: 'Slack',
          enabled: slackEnabled,
          status: status['slack'] || t('channels.status.disconnected'),
        },
      ]);

    } catch (error) {
      logger.error('Erro ao carregar canais:', error);
      addToast(t('channels.error.loadFailed'), 'error');
    } finally {
      setLoading(false);
    }
  }, [addToast, channelCredentialPattern, defaultChannelProfile]);

  useEffect(() => {
    loadAll();
    loadTemplates();
  }, [loadAll, loadTemplates]);

  useEffect(() => {
    return () => {
      if (linkPollRef.current) clearTimeout(linkPollRef.current);
    };
  }, []);

  // ── Create Channel Handler ───────────────────────────────────────

  const handleChannelCreated = () => {
    // suppressAnnounce: o announce() abaixo já fala (evita anúncio duplicado).
    addToast(t('channels.toast.channelCreated'), 'success', undefined, undefined, { suppressAnnounce: true });
    announce(t('channels.announce.channelCreated'));
    setCreateModalTemplateType(null);
    setShowCreateChannelModal(false);
    loadAll();
  };

  const openCreateMenu = () => {
    if (createMenuVisible) {
      setCreateMenuVisible(false);
      return;
    }

    const button = createMenuButtonRef.current;
    if (button) {
      const rect = button.getBoundingClientRect();
      setCreateMenuPosition({ x: rect.left, y: rect.bottom + 6 });
    }
    setCreateMenuVisible(true);
  };

  const closeCreateMenu = () => {
    setCreateMenuVisible(false);
  };

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (isModalOpen()) return;
      if (e.ctrlKey && !e.shiftKey && !e.altKey && (e.key === 'n' || e.key === 'N')) {
        e.preventDefault();
        openCreateMenu();
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [openCreateMenu]);


  const handleQuickCreate = async (template: channels.ChannelTemplate) => {
    setCreateMenuVisible(false);

    if (template.type === 'telegram' || template.type === 'signal') {
      try {
        const existing = await GetChannelConfig(template.type);
        if (!existing) {
          const defaultConfig = channels.ChannelConfig.createFrom({
            enabled: false,
            bot_token: '',
            api_url: '',
            account: '',
            profile: '',
            max_history: 50,
            max_contacts: 1,
          });
          await SaveChannelConfig(template.type, defaultConfig);
        }

        await loadAll();
        setEditingChannel(template.type);
      } catch (error: unknown) {
        logger.error('Erro ao criar canal:', error);
        addToast(getErrorMessage(error) || t('channels.error.createFailed'), 'error');
      }
      return;
    }

    setCreateModalTemplateType(template.type);
    setShowCreateChannelModal(true);
  };

  // ── Save channel ─────────────────────────────────────────────────

  const handleSaveChannel = async (channelName: string) => {
    setSaving(true);
    try {
      if (channelName === 'telegram') {
        const botPattern = channelCredentialPattern('telegram', 'bot_token');
        const botToken = telegramForm.botToken.trim();
        const storedBot = credentialSummaries[botPattern];

        if (telegramUseVault) {
          if (botToken) {
            await UpsertCredential({
              pattern: botPattern,
              type: 'secret',
              token: botToken,
            });
          } else if (telegramForm.enabled && !storedBot) {
            throw new Error(t('channels.error.telegramTokenRequired'));
          }
        }

        await SaveChannelConfig('telegram', channels.ChannelConfig.createFrom({
          enabled: telegramForm.enabled,
          bot_token: telegramUseVault ? '' : botToken,
          bot_token_ref: telegramUseVault ? botPattern : '',
          profile: telegramForm.profile,
          max_history: telegramForm.maxHistory,
          max_contacts: telegramForm.maxContacts,
        }));
      } else if (channelName === 'signal') {
        const effectiveAccount = (signalForm.account || signalAccounts[0] || '').trim();
        const effectiveApiURL = signalForm.apiURL?.trim() || '';
        await SaveChannelConfig('signal', channels.ChannelConfig.createFrom({
          enabled: signalForm.enabled,
          api_url: effectiveApiURL,
          account: effectiveAccount,
          profile: signalForm.profile,
          max_history: signalForm.maxHistory,
          max_contacts: signalForm.maxContacts,
        }));
      } else if (channelName === 'slack') {
        const botPattern = channelCredentialPattern('slack', 'bot_token');
        const appPattern = channelCredentialPattern('slack', 'app_token');
        const botToken = slackForm.botToken.trim();
        const appToken = slackForm.appToken.trim();
        const storedBot = credentialSummaries[botPattern];
        const storedApp = credentialSummaries[appPattern];

        if (slackUseVault) {
          if (botToken) {
            await UpsertCredential({
              pattern: botPattern,
              type: 'secret',
              token: botToken,
            });
          }
          if (appToken) {
            await UpsertCredential({
              pattern: appPattern,
              type: 'secret',
              token: appToken,
            });
          }

          if (slackForm.enabled && ((!storedBot && !botToken) || (!storedApp && !appToken))) {
            throw new Error(t('channels.error.slackTokensRequired'));
          }
        }

        await SaveChannelConfig('slack', channels.ChannelConfig.createFrom({
          enabled: slackForm.enabled,
          bot_token: slackUseVault ? '' : botToken,
          bot_token_ref: slackUseVault ? botPattern : '',
          app_token: slackUseVault ? '' : appToken,
          app_token_ref: slackUseVault ? appPattern : '',
          profile: slackForm.profile,
          max_history: slackForm.maxHistory,
          max_contacts: slackForm.maxContacts,
        }));
      }
      addToast(t('channels.toast.channelSaved', { name: channelName }), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('channels.toast.channelSaved', { name: channelName }));
      await loadAll();
      setEditingChannel(null);
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('channels.error.saveFailed', { name: channelName }), 'error');
    } finally {
      setSaving(false);
    }
  };

  // ── Channel editor open/close ────────────────────────────────────

  const handleEditChannel = useCallback((row: ChannelRow) => {
    setEditingChannel(row.name);
    announce(t('channels.announce.editorOpened', { label: row.label }));
  }, [announce, t]);

  useResourceEditRequest('channels', {
    onEdit: (name) => {
      const found = channelRows.find((r) => r.name === name);
      if (found) handleEditChannel(found);
    },
    ready: !loading && channelRows.length > 0,
  });

  const handleCloseEditor = () => {
    setEditingChannel(null);
    announce(t('channels.announce.editorClosed'));
  };

  const handleReconnectChannel = async (channelName: string) => {
    setReconnecting(true);
    try {
      await RestartChannel(channelName);
      addToast(t('channels.toast.channelReconnected', { name: channelName }), 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
      announce(t('channels.toast.channelReconnected', { name: channelName }));
      await loadAll();
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('channels.error.reconnectFailed', { name: channelName }), 'error');
    } finally {
      setReconnecting(false);
    }
  };

  // ── Signal helpers ───────────────────────────────────────────────

  const handleSignalCheckAPI = async () => {
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
      addToast(msg, 'error');
    } finally {
      setSignalCheckingAPI(false);
    }
  };

  const handleSignalRegister = async (mode: 'sms' | 'voice' = 'sms') => {
    if (!signalForm.account || !signalForm.apiURL) {
      const msg = t('channels.error.signalAccountAndUrlRequired');
      setSignalRegError(msg);
      addToast(msg, 'error', undefined, undefined, { suppressAnnounce: true });
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
    }
  };

  const handleSignalVerify = async () => {
    if (!signalRegCode) {
      const msg = t('channels.error.verificationCodeRequired');
      setSignalRegError(msg);
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
    } catch (error: unknown) {
      setSignalRegStep('awaiting_code');
      const msg = getErrorMessage(error) || t('channels.error.signalVerifyFailed');
      setSignalRegError(msg);
      addToast(msg, 'error', undefined, undefined, { suppressAnnounce: true });
    }
  };

  const stopLinkPolling = () => {
    if (linkPollRef.current) {
      clearTimeout(linkPollRef.current);
      linkPollRef.current = null;
    }
  };

  const startLinkPolling = (startTime: number) => {
    const POLL_TIMEOUT_MS = 2 * 60 * 1000;
    linkPollRef.current = setTimeout(async () => {
      if (Date.now() - startTime > POLL_TIMEOUT_MS) {
        setSignalLinking(false);
        setSignalRegError(t('channels.error.signalLinkTimeoutDetails'));
        const linkTimeoutMessage = t('channels.announce.linkTimeout');
        addToast(linkTimeoutMessage, 'error', undefined, undefined, { suppressAnnounce: true });
        announce(linkTimeoutMessage);
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
      startLinkPolling(startTime);
    }, 5000);
  };

  const handleSignalLink = async () => {
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
      const qr = await SignalLink(signalForm.apiURL, 'Assistente', apiToken);
      setSignalLinkQR(qr);
      announce(t('channels.announce.qrGenerated'));
      startLinkPolling(Date.now());
    } catch (error: unknown) {
      const errorMessage = getErrorMessage(error) || t('channels.error.signalLinkQrFailedDetailed');
      setSignalRegError(errorMessage);
      addToast(errorMessage, 'error');
      setSignalLinking(false);
    }
  };

  const handleSignalUnregister = async (account: string) => {
    const shouldRemove = await requestConfirm({
      title: t('channels.confirm.removeSignalAccountTitle'),
      message: `Remover a conta ${account} do servidor Signal?\n\nIsto irá desregistrar e apagar os dados locais.`,
      confirmText: 'Remover',
      cancelText: 'Cancelar',
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
  };

  const handleRemoveCredential = useCallback(async (pattern: string, label: string) => {
    const shouldRemove = await requestConfirm({
      title: t('channels.confirm.removeCredentialTitle'),
      message: `Remover a credencial ${label}?`,
      confirmText: 'Remover',
      cancelText: 'Cancelar',
      variant: 'danger',
    });

    if (!shouldRemove) return;

    try {
      await DeleteCredential(pattern);
      addToast(t('channels.toast.credentialRemoved'), 'success', undefined, undefined, { suppressAnnounce: true });
      announce(t('channels.announce.credentialRemoved'));
      await loadAll();
    } catch (error: unknown) {
      addToast(getErrorMessage(error) || t('channels.error.removeCredentialFailed'), 'error');
    }
  }, [addToast, announce, loadAll, requestConfirm, t]);

  // ── Grid columns ─────────────────────────────────────────────────

  const channelColumns: DataGridColumn<ChannelRow>[] = [
    { key: 'label', label: t('channels.columns.channel'), width: '150px' },
    {
      key: 'enabled', label: t('channels.columns.enabled'), width: '100px',
      format: (val) => val ? t('channels.format.yes') : t('channels.format.no'),
    },
    { key: 'status', label: t('channels.columns.status'), width: '200px', truncate: true },
    {
      key: 'actions', label: t('common.actions'), width: '80px',
      format: (_val, row) => (
        <MenuButton
          items={getChannelRowActions(row)}
          buttonLabel={t('channels.actions.actions', 'Ações')}
        />
      ),
    },
  ];

  function getChannelRowActions(row: ChannelRow) {
    return [
      {
        id: 'edit',
        label: t('channels.actions.edit', 'Editar'),
        icon: <EditOutlined aria-hidden="true" />,
        onClick: () => handleEditChannel(row),
      },
      {
        id: 'reconnect',
        label: t('channels.actions.reconnectChannel', 'Reconectar'),
        icon: <ReloadOutlined aria-hidden="true" />,
        onClick: () => handleReconnectChannel(row.name),
      },
    ];
  }

  // ── Stable DataGrid callbacks (memoization) ─────────────────────────

  const getChannelRowId = useCallback((item: ChannelRow) => item.id, []);
  const handleActivateChannelRow = useCallback((item: ChannelRow) => handleEditChannel(item), [handleEditChannel]);
  const handleChannelFocusChange = useCallback((item: ChannelRow | null) => setFocusedChannel(item), []);

  const createMenuItems: MenuItem[] = channelTemplates.length > 0
    ? channelTemplates.map((template) => ({
      id: `create-${template.type}`,
      label: template.display_name,
      icon: template.icon,
      ariaLabel: `Criar canal ${template.display_name}`,
      action: () => handleQuickCreate(template),
    }))
    : [{
      id: 'no-templates',
      label: t('channels.empty.noChannels'),
      icon: <WarningOutlined aria-hidden="true" />,
      ariaLabel: t('channels.empty.noChannels'),
      action: closeCreateMenu,
    }];

  // ── Editor title ─────────────────────────────────────────────────

  const editorTitle = editingChannel === 'telegram'
    ? 'Telegram'
    : editingChannel === 'signal'
      ? 'Signal'
      : editingChannel === 'slack'
        ? 'Slack'
        : '';

  const toolbarActions = [
    {
      key: 'edit-channel',
      label: t('channels.actions.edit', 'Editar'),
      icon: <EditOutlined aria-hidden="true" />,
      onClick: () => focusedChannel && handleEditChannel(focusedChannel),
      disabled: !focusedChannel,
    },
    {
      key: 'reconnect-channel',
      label: t('channels.actions.reconnectChannel', 'Reconectar'),
      icon: <ReloadOutlined aria-hidden="true" />,
      onClick: () => focusedChannel && handleReconnectChannel(focusedChannel.name),
      disabled: !focusedChannel,
    },
    {
      key: 'reload',
      label: t('channels.buttons.reload', 'Recarregar'),
      icon: <ReloadOutlined aria-hidden="true" />,
      variant: 'secondary' as const,
      onClick: loadAll,
      disabled: false,
    },
  ];

  // ── Render: Telegram editor ──────────────────────────────────────

  const renderTelegramEditor = () => (
    <ChannelsTelegramSection
      form={telegramForm}
      onChange={setTelegramForm}
      onAnnounce={announce}
      vaultEnabled={telegramUseVault}
      onToggleVault={setTelegramUseVault}
      credentialStored={Boolean(credentialSummaries[channelCredentialPattern('telegram', 'bot_token')])}
      credentialMasked={credentialSummaries[channelCredentialPattern('telegram', 'bot_token')]?.masked || ''}
      onRemoveCredential={() => handleRemoveCredential(channelCredentialPattern('telegram', 'bot_token'), 'Telegram Bot Token')}
    />
  );

  // ── Render: Signal editor ────────────────────────────────────────

  const renderSignalEditor = () => (
    <ChannelsSignalSection
      form={signalForm}
      onChange={setSignalForm}
      onAnnounce={announce}
      vaultEnabled={signalUseVault}
      onToggleVault={setSignalUseVault}
      tokenStored={Boolean(credentialSummaries[channelCredentialPattern('signal', 'api_token')])}
      tokenMasked={credentialSummaries[channelCredentialPattern('signal', 'api_token')]?.masked || ''}
      onRemoveToken={() => handleRemoveCredential(channelCredentialPattern('signal', 'api_token'), 'Signal API Token')}
      apiReady={signalAPIReady}
      apiInfo={signalAPIInfo}
      regError={signalRegError}
      regStep={signalRegStep}
      regCode={signalRegCode}
      regCaptcha={signalRegCaptcha}
      smsSent={signalSmsSent}
      accounts={signalAccounts}
      connectionMode={signalConnectionMode}
      linkQR={signalLinkQR}
      linking={signalLinking}
      unregistering={signalUnregistering}
      checkingAPI={signalCheckingAPI}
      onSetApiReady={setSignalAPIReady}
      onSetApiInfo={setSignalAPIInfo}
      onSetRegError={setSignalRegError}
      onSetRegStep={setSignalRegStep}
      onSetRegCode={setSignalRegCode}
      onSetRegCaptcha={setSignalRegCaptcha}
      onSetSmsSent={setSignalSmsSent}
      onSetAccounts={setSignalAccounts}
      onSetConnectionMode={setSignalConnectionMode}
      onSetLinkQR={setSignalLinkQR}
      onSetLinking={setSignalLinking}
      onCheckAPI={handleSignalCheckAPI}
      onRegister={handleSignalRegister}
      onVerify={handleSignalVerify}
      onLink={handleSignalLink}
      onUnregister={handleSignalUnregister}
      onStopLinkPolling={stopLinkPolling}
    />
  );

  // ── Render: Slack editor ─────────────────────────────────────────

  const renderSlackEditor = () => (
    <ChannelsSlackSection
      form={slackForm}
      onChange={setSlackForm}
      onAnnounce={announce}
      vaultEnabled={slackUseVault}
      onToggleVault={setSlackUseVault}
      botTokenStored={Boolean(credentialSummaries[channelCredentialPattern('slack', 'bot_token')])}
      botTokenMasked={credentialSummaries[channelCredentialPattern('slack', 'bot_token')]?.masked || ''}
      appTokenStored={Boolean(credentialSummaries[channelCredentialPattern('slack', 'app_token')])}
      appTokenMasked={credentialSummaries[channelCredentialPattern('slack', 'app_token')]?.masked || ''}
      onRemoveBotToken={() => handleRemoveCredential(channelCredentialPattern('slack', 'bot_token'), 'Slack Bot Token')}
      onRemoveAppToken={() => handleRemoveCredential(channelCredentialPattern('slack', 'app_token'), 'Slack App Token')}
    />
  );

  // ── Main render ──────────────────────────────────────────────────

  if (loading) {
    return (
      <div className="channels-page">
        <div className="channels-page__loading">{t('channels.loading', 'Carregando canais...')}</div>
      </div>
    );
  }

  return (
    <div className="channels-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('channels.title', 'Canais de Comunicação')}</h1>}
        right={
          <ToolbarButton
            ref={createMenuButtonRef}
            label="Novo"
            icon={<PlusOutlined aria-hidden="true" />}
            endIcon={<DownOutlined aria-hidden="true" />}
            shortcut="Ctrl+N"
            variant="primary"
            onClick={openCreateMenu}
            aria-haspopup="menu"
            aria-expanded={createMenuVisible}
            aria-label="Novo canal"
          />
        }
        actions={toolbarActions}
        ariaLabel="Barra de ferramentas de canais"
      />

      <div className="channels-page__content">
        <DataGrid
          items={channelRows}
          columns={channelColumns}
          label="Canais de comunicação"
          autoFocusOnMount={false}
          getItemId={getChannelRowId}
          onActivate={handleActivateChannelRow}
          onGridReady={channelsHandleGridReady}
          getRowActions={getChannelRowActions}
          onFocusChange={handleChannelFocusChange}
        />
      </div>

      {/* Editor Modal */}
      <Modal
        isOpen={!!editingChannel}
        onClose={handleCloseEditor}
        title={`Editor de canal: ${editorTitle}`}
        size="lg"
      >
        <EditorPanelFields className="channels-page__fields">
          {editingChannel === 'telegram' && renderTelegramEditor()}
          {editingChannel === 'signal' && renderSignalEditor()}
          {editingChannel === 'slack' && renderSlackEditor()}
        </EditorPanelFields>
        <EditorPanelFooter>
          {editingChannel && (
            <Button
              variant="outline"
              onClick={() => handleReconnectChannel(editingChannel)}
              loading={reconnecting}
            >
              Reconectar
            </Button>
          )}
          <Button variant="ghost" onClick={handleCloseEditor}>
            Cancelar
          </Button>
          {editingChannel && (
            <Button onClick={() => handleSaveChannel(editingChannel)} loading={saving}>
              Salvar
            </Button>
          )}
        </EditorPanelFooter>
      </Modal>

      <ContextMenu
        items={createMenuItems}
        x={createMenuPosition.x}
        y={createMenuPosition.y}
        visible={createMenuVisible}
        ariaLabel="Menu de criação de canais"
        onClose={closeCreateMenu}
      />

      {/* Create Channel Modal */}
      <CreateChannelModal
        isOpen={showCreateChannelModal}
        onClose={() => {
          setShowCreateChannelModal(false);
          setCreateModalTemplateType(null);
        }}
        onSuccess={handleChannelCreated}
        initialTemplateType={createModalTemplateType}
      />
    </div>
  );
}
