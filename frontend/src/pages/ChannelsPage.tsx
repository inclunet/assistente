import { useState, useEffect, useRef, useCallback } from 'react';
import {
  GetChannelConfig,
  SaveChannelConfig,
  GetMessagingStatus,
  GetAuthorizedContacts,
  RemoveAuthorizedContact,
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
} from '@wailsjs/go/main/App';
import { channels } from '../../wailsjs/go/models';
import { useUIStore } from '../store/uiStore';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridFocus } from '../hooks/useGridFocus';
import { Button } from '../components';
import { ChannelsTelegramSection, ChannelsSignalSection, ChannelsSlackSection } from '../components/channels';
import { Toolbar, ToolbarButton } from '../components/ui/Toolbar';
import { Tabs, TabList, Tab, TabPanel } from '../components/ui/tabs';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Modal, isModalOpen } from '../components/ui/Modal';
import { EditorPanelFields, EditorPanelFooter } from '../components/ui/EditorPanel';
import { ContextMenu, MenuItem } from '../components/menu';
import CreateChannelModal from '../components/modals/CreateChannelModal';
import { playBumpSound } from '../services/audioFeedback';
import { useConfirm } from '../hooks/useConfirm';
import './ChannelsPage.css';

// ─── Types ───────────────────────────────────────────────────────────

type ActiveTab = 'channels' | 'contacts';

interface ChannelRow {
  id: string;
  name: string;
  label: string;
  enabled: boolean;
  status: string;
}

interface ContactRow {
  id: string;
  channel: string;
  contactId: string;
  displayName: string;
  username: string;
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
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const { focusFirstCell: channelsFocusFirstCell, handleGridReady: channelsHandleGridReady } = useGridFocus();
  const { focusFirstCell: contactsFocusFirstCell, handleGridReady: contactsHandleGridReady } = useGridFocus();
  const defaultChannelProfile = 'canais-comunicacao';
  const requestConfirm = useConfirm();

  const channelCredentialPattern = useCallback((channel: string, key: string) => `channel:${channel}:${key}`, []);

  // ── Tab ──────────────────────────────────────────────────────────
  const [activeTab, setActiveTab] = useState<ActiveTab>('channels');
  const tabs: { id: ActiveTab; label: string }[] = [
    { id: 'channels', label: 'Canais' },
    { id: 'contacts', label: 'Contatos' },
  ];

  // ── Loading ──────────────────────────────────────────────────────
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);

  // ── Channels grid ────────────────────────────────────────────────
  const [channelRows, setChannelRows] = useState<ChannelRow[]>([]);
  const [editingChannel, setEditingChannel] = useState<string | null>(null);

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

  // ── Contacts grid ────────────────────────────────────────────────
  const [contactRows, setContactRows] = useState<ContactRow[]>([]);

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
      console.error('Erro ao carregar templates de canal:', error);
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
          status: status['telegram'] || 'desconectado',
        },
        {
          id: 'signal',
          name: 'signal',
          label: 'Signal',
          enabled: sigEnabled,
          status: status['signal'] || 'desconectado',
        },
        {
          id: 'slack',
          name: 'slack',
          label: 'Slack',
          enabled: slackEnabled,
          status: status['slack'] || 'desconectado',
        },
      ]);

      // Load contacts separately, don't block if it fails
      try {
        const allContacts = await GetAuthorizedContacts();
        const rows: ContactRow[] = [];
        if (allContacts) {
          for (const [ch, contacts] of Object.entries(allContacts)) {
            if (Array.isArray(contacts)) {
              for (const c of contacts as any[]) {
                rows.push({
                  id: `${ch}:${c.id}`,
                  channel: ch,
                  contactId: c.id || '',
                  displayName: c.display_name || '',
                  username: c.username || '',
                });
              }
            }
          }
        }
        setContactRows(rows);
      } catch (contactError) {
        console.error('Erro ao carregar contatos (não-crítico):', contactError);
        setContactRows([]);
        // Don't show toast for non-critical contact loading error
      }
    } catch (error) {
      console.error('Erro ao carregar canais:', error);
      addToast('Erro ao carregar canais', 'error');
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
    addToast('Canal criado com sucesso!', 'success');
    announce('Canal criado');
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
      } catch (error: any) {
        console.error('Erro ao criar canal:', error);
        addToast(error.message || 'Erro ao criar canal', 'error');
      }
      return;
    }

    setCreateModalTemplateType(template.type);
    setShowCreateChannelModal(true);
  };

  const handleTabChange = useCallback(
    (tabId: ActiveTab) => {
      const tab = tabs.find((t) => t.id === tabId);
      setActiveTab(tabId);
      setEditingChannel(null);
      announce(`Aba ${tab?.label ?? tabId} selecionada`);
    },
    [announce, tabs]
  );

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
            throw new Error('Informe o token do Telegram ou desative o cofre.');
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
            throw new Error('Informe os tokens do Slack ou desative o cofre.');
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
      addToast(`Canal ${channelName} salvo!`, 'success');
      announce(`Canal ${channelName} salvo`);
      await loadAll();
      setEditingChannel(null);
      setActiveTab('channels');
      setTimeout(() => channelsFocusFirstCell?.(), 0);
    } catch (error: any) {
      addToast(error.message || `Erro ao salvar canal ${channelName}`, 'error');
    } finally {
      setSaving(false);
    }
  };

  // ── Channel editor open/close ────────────────────────────────────

  const handleEditChannel = (row: ChannelRow) => {
    setEditingChannel(row.name);
    announce(`Editor aberto para ${row.label}`);
  };

  const handleCloseEditor = () => {
    setEditingChannel(null);
    announce('Editor fechado');
  };

  const handleReconnectChannel = async (channelName: string) => {
    setReconnecting(true);
    try {
      await RestartChannel(channelName);
      addToast(`Canal ${channelName} reconectado!`, 'success');
      announce(`Canal ${channelName} reconectado`);
      await loadAll();
    } catch (error: any) {
      addToast(error.message || `Erro ao reconectar ${channelName}`, 'error');
    } finally {
      setReconnecting(false);
    }
  };

  // ── Signal helpers ───────────────────────────────────────────────

  const handleSignalCheckAPI = async () => {
    if (!signalForm.apiURL) {
      addToast('Informe a URL da API Signal', 'error');
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
      let infoText = `API acessível (v${info['version'] || '?'}, build ${info['build'] || '?'}).`;
      if (accounts && accounts.length > 0) {
        infoText += ` Contas registradas: ${accounts.join(', ')}`;
        if (!signalForm.account) {
          setSignalForm((prev) => ({ ...prev, account: accounts[0] }));
        }
      } else {
        infoText += ' Nenhuma conta registrada — registre ou vincule abaixo.';
      }
      setSignalAPIInfo(infoText);
      setSignalAPIReady(true);
      addToast('Signal API acessível!', 'success');
      announce(infoText);
    } catch (error: any) {
      setSignalAPIInfo('');
      const msg = error.message || 'Não foi possível conectar à API Signal';
      setSignalRegError(msg);
      setSignalAPIReady(false);
      addToast(msg, 'error');
    } finally {
      setSignalCheckingAPI(false);
    }
  };

  const handleSignalRegister = async (mode: 'sms' | 'voice' = 'sms') => {
    if (!signalForm.account || !signalForm.apiURL) {
      const msg = 'Informe a URL da API e o número de telefone no campo Conta';
      setSignalRegError(msg);
      addToast(msg, 'error');
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
      const modeLabel = mode === 'voice' ? 'ligação' : 'SMS';
      addToast(`Código enviado por ${modeLabel} para ${signalForm.account}`, 'success');
      announce(`Código enviado por ${modeLabel}`);
    } catch (error: any) {
      setSignalRegStep(signalSmsSent ? 'awaiting_code' : 'idle');
      const msg = error.message || 'Erro ao registrar número';
      setSignalRegError(msg);
      addToast(msg, 'error');
    }
  };

  const handleSignalVerify = async () => {
    if (!signalRegCode) {
      setSignalRegError('Informe o código de verificação');
      return;
    }
    const apiToken = signalForm.apiToken.trim();
    setSignalRegStep('verifying');
    setSignalRegError('');
    try {
      await SignalVerify(signalForm.apiURL, signalForm.account, signalRegCode, apiToken);
      setSignalRegStep('done');
      setSignalSmsSent(false);
      addToast(`Número ${signalForm.account} verificado com sucesso`, 'success');
      announce('Número verificado com sucesso');
    } catch (error: any) {
      setSignalRegStep('awaiting_code');
      const msg = error.message || 'Erro ao verificar código';
      setSignalRegError(msg);
      addToast(msg, 'error');
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
        setSignalRegError('Tempo esgotado. Verifique os logs do container signal-cli-rest-api.');
        addToast('Tempo esgotado na vinculação', 'error');
        announce('Tempo esgotado na vinculação');
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
          addToast(`Dispositivo vinculado! Conta: ${accounts[0]}`, 'success');
          announce(`Dispositivo vinculado. Conta: ${accounts[0]}`);
          return;
        }
      } catch { /* polling */ }
      startLinkPolling(startTime);
    }, 5000);
  };

  const handleSignalLink = async () => {
    if (!signalForm.apiURL) {
      setSignalRegError('Informe a URL da API Signal');
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
      announce('QR Code gerado. Escaneie com o Signal no celular.');
      startLinkPolling(Date.now());
    } catch (error: any) {
      setSignalRegError(error.message || 'Erro ao gerar QR de vinculação');
      addToast(error.message || 'Erro ao gerar QR', 'error');
      setSignalLinking(false);
    }
  };

  const handleSignalUnregister = async (account: string) => {
    const shouldRemove = await requestConfirm({
      title: 'Remover conta do Signal',
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
      addToast(`Conta ${account} removida`, 'success');
      announce(`Conta ${account} removida`);
    } catch (error: any) {
      addToast(error.message || 'Erro ao remover conta', 'error');
    } finally {
      setSignalUnregistering(null);
    }
  };

  // ── Contact removal ──────────────────────────────────────────────

  const handleDeleteContact = useCallback(async (row: ContactRow) => {
    const name = row.displayName || row.contactId;
    const shouldRemove = await requestConfirm({
      title: 'Remover Contato',
      message: `Remover ${name} do canal ${row.channel}?`,
      confirmText: 'Remover',
      cancelText: 'Cancelar',
      variant: 'danger',
    });

    if (!shouldRemove) return;

    try {
      await RemoveAuthorizedContact(row.channel, row.contactId);
      addToast('Contato removido', 'success');
      announce('Contato removido');
      await loadAll();
    } catch (error: any) {
      addToast(error.message || 'Erro ao remover contato', 'error');
    }
  }, [addToast, announce, loadAll, requestConfirm]);

  const handleRemoveCredential = useCallback(async (pattern: string, label: string) => {
    const shouldRemove = await requestConfirm({
      title: 'Remover credencial',
      message: `Remover a credencial ${label}?`,
      confirmText: 'Remover',
      cancelText: 'Cancelar',
      variant: 'danger',
    });

    if (!shouldRemove) return;

    try {
      await DeleteCredential(pattern);
      addToast('Credencial removida', 'success');
      announce('Credencial removida');
      await loadAll();
    } catch (error: any) {
      addToast(error.message || 'Erro ao remover credencial', 'error');
    }
  }, [addToast, announce, loadAll, requestConfirm]);

  // ── Grid columns ─────────────────────────────────────────────────

  const channelColumns: DataGridColumn<ChannelRow>[] = [
    { key: 'label', label: 'Canal', width: '150px' },
    {
      key: 'enabled', label: 'Habilitado', width: '100px',
      format: (val) => val ? '✓ Sim' : '✗ Não',
    },
    { key: 'status', label: 'Status', width: '200px', truncate: true },
    {
      key: '_reconnect', label: 'Reconectar', width: '80px',
      action: true, actionIcon: '🔄', actionLabel: 'Reconectar canal',
    },
  ];

  const contactColumns: DataGridColumn<ContactRow>[] = [
    { key: 'channel', label: 'Canal', width: '100px' },
    { key: 'displayName', label: 'Nome', width: '200px', truncate: true },
    { key: 'username', label: 'Usuário', width: '150px', truncate: true },
    { key: 'contactId', label: 'ID', width: '200px', truncate: true },
    {
      key: '_delete', label: 'Remover', width: '80px',
      action: true, actionIcon: '🗑️', actionLabel: 'Remover contato',
    },
  ];

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
      label: 'Nenhum canal disponível',
      icon: '⚠️',
      ariaLabel: 'Nenhum canal disponível',
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
        <div className="channels-page__loading" role="status">Carregando canais...</div>
      </div>
    );
  }

  return (
    <div className="channels-page">
      <Tabs
        value={activeTab}
        onValueChange={(v) => handleTabChange(v as ActiveTab)}
        idBase="channels"
        onBump={playBumpSound}
      >
        <TabList className="channels-page__tabs" ariaLabel="Seções de canais">
          {tabs.map((tab) => (
            <Tab
              key={tab.id}
              value={tab.id}
              className="channels-page__tab"
              activeClassName="channels-page__tab--active"
            >
              {tab.id === 'contacts' ? `${tab.label} (${contactRows.length})` : tab.label}
            </Tab>
          ))}
        </TabList>
      </Tabs>

      <Toolbar
        left={<h1 className="page-toolbar__title">Canais de Comunicação</h1>}
        right={
          <ToolbarButton
            ref={createMenuButtonRef}
            label="Novo"
            icon="➕"
            endIcon="▾"
            shortcut="Ctrl+N"
            variant="primary"
            onClick={openCreateMenu}
            aria-haspopup="menu"
            aria-expanded={createMenuVisible}
            aria-label="Novo canal"
          />
        }
        actions={[
          {
            key: 'reload',
            label: 'Recarregar',
            icon: '🔄',
            variant: 'secondary',
            onClick: loadAll,
          },
        ]}
        ariaLabel="Barra de ferramentas de canais"
        onFocusGrid={activeTab === 'channels' ? channelsFocusFirstCell : contactsFocusFirstCell}
      />

      {/* Channels tab panel */}
      {activeTab === 'channels' && (
        <TabPanel value="channels" className="channels-page__content">
          <DataGrid
            items={channelRows}
            columns={channelColumns}
            label="Canais de comunicação"
            autoFocusOnMount={false}
            getItemId={(item) => item.id}
            onActivate={(item) => handleEditChannel(item)}
            onCellAction={(item, column) => {
              if (column.key === '_reconnect') {
                handleReconnectChannel(item.name);
              }
            }}
            onGridReady={channelsHandleGridReady}
          />
        </TabPanel>
      )}

      {/* Contacts tab panel */}
      {activeTab === 'contacts' && (
        <TabPanel value="contacts" className="channels-page__content">
          <p className="channels-page__tab-description">
            Contatos que podem se comunicar com o assistente por cada canal.
            Remova um contato para liberar uma vaga para novas autorizações.
          </p>
          {contactRows.length > 0 ? (
            <DataGrid
              items={contactRows}
              columns={contactColumns}
              label="Contatos autorizados"
              autoFocusOnMount={false}
              getItemId={(item) => item.id}
              onCellAction={(item) => {
                void handleDeleteContact(item);
              }}
              onDelete={(item) => {
                void handleDeleteContact(item);
              }}
              onGridReady={contactsHandleGridReady}
            />
          ) : (
            <p className="channels-page__empty" role="status">Nenhum contato autorizado.</p>
          )}
        </TabPanel>
      )}

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
