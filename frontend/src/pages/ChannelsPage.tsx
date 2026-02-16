import { useState, useEffect, useRef, useCallback, KeyboardEvent as ReactKeyboardEvent } from 'react';
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
} from '../../wailsjs/go/main/App';
import { channels } from '../../wailsjs/go/models';
import { useUIStore } from '../store/uiStore';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridFocus } from '../hooks/useGridFocus';
import { Input, Button, Checkbox } from '../components';
import { ProfilePicker } from '../components/pickers/ProfilePicker';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { ConfirmDialog } from '../components/ui/ConfirmDialog';
import { SimpleModal } from '../components/ui/SimpleModal';
import { playBumpSound } from '../services/audioFeedback';
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

  // ── Tab ──────────────────────────────────────────────────────────
  const [activeTab, setActiveTab] = useState<ActiveTab>('channels');
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);
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
    enabled: false, apiURL: '', account: '', profile: '', maxHistory: 50, maxContacts: 1,
  });

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
  const [deleteContactOpen, setDeleteContactOpen] = useState(false);
  const [deleteContactTarget, setDeleteContactTarget] = useState<ContactRow | null>(null);

  // ── Load ─────────────────────────────────────────────────────────

  const loadAll = useCallback(async () => {
    setLoading(true);
    try {
      const [telegramCfg, signalCfg, status, allContacts] = await Promise.all([
        GetChannelConfig('telegram'),
        GetChannelConfig('signal'),
        GetMessagingStatus(),
        GetAuthorizedContacts(),
      ]);

      const tgEnabled = telegramCfg?.enabled || false;
      const sigEnabled = signalCfg?.enabled || false;

      if (telegramCfg) {
        setTelegramForm({
          enabled: tgEnabled,
          botToken: telegramCfg.bot_token || '',
          profile: telegramCfg.profile || '',
          maxHistory: telegramCfg.max_history || 50,
          maxContacts: telegramCfg.max_contacts || 1,
        });
      }
      if (signalCfg) {
        setSignalForm({
          enabled: sigEnabled,
          apiURL: signalCfg.api_url || '',
          account: signalCfg.account || '',
          profile: signalCfg.profile || '',
          maxHistory: signalCfg.max_history || 50,
          maxContacts: signalCfg.max_contacts || 1,
        });
      }

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
      ]);

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
    } catch (error) {
      console.error('Erro ao carregar canais:', error);
      addToast('Erro ao carregar canais', 'error');
    } finally {
      setLoading(false);
    }
  }, [addToast]);

  useEffect(() => {
    loadAll();
  }, [loadAll]);

  useEffect(() => {
    return () => {
      if (linkPollRef.current) clearTimeout(linkPollRef.current);
    };
  }, []);

  // ── Tab keyboard navigation (ARIA tabs pattern) ──────────────────

  const handleTabKeyDown = (e: ReactKeyboardEvent<HTMLButtonElement>, index: number) => {
    let nextIndex = index;

    switch (e.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        e.preventDefault();
        if (index === tabs.length - 1) { playBumpSound(); return; }
        nextIndex = index + 1;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        e.preventDefault();
        if (index === 0) { playBumpSound(); return; }
        nextIndex = index - 1;
        break;
      case 'Home':
        e.preventDefault();
        if (index === 0) { playBumpSound(); return; }
        nextIndex = 0;
        break;
      case 'End':
        e.preventDefault();
        if (index === tabs.length - 1) { playBumpSound(); return; }
        nextIndex = tabs.length - 1;
        break;
      default:
        return;
    }

    const tab = tabs[nextIndex];
    setActiveTab(tab.id);
    setEditingChannel(null);
    tabRefs.current[nextIndex]?.focus();
    announce(`Aba ${tab.label} selecionada`);
  };

  const handleTabClick = (tab: { id: ActiveTab; label: string }) => {
    setActiveTab(tab.id);
    setEditingChannel(null);
    announce(`Aba ${tab.label} selecionada`);
  };

  // ── Save channel ─────────────────────────────────────────────────

  const handleSaveChannel = async (channelName: string) => {
    setSaving(true);
    try {
      if (channelName === 'telegram') {
        await SaveChannelConfig('telegram', channels.ChannelConfig.createFrom({
          enabled: telegramForm.enabled,
          bot_token: telegramForm.botToken,
          profile: telegramForm.profile,
          max_history: telegramForm.maxHistory,
          max_contacts: telegramForm.maxContacts,
        }));
      } else if (channelName === 'signal') {
        await SaveChannelConfig('signal', channels.ChannelConfig.createFrom({
          enabled: signalForm.enabled,
          api_url: signalForm.apiURL,
          account: signalForm.account,
          profile: signalForm.profile,
          max_history: signalForm.maxHistory,
          max_contacts: signalForm.maxContacts,
        }));
      }
      addToast(`Canal ${channelName} salvo!`, 'success');
      announce(`Canal ${channelName} salvo`);
      await loadAll();
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
    setSignalCheckingAPI(true);
    setSignalAPIInfo('');
    setSignalAPIReady(false);
    setSignalRegError('');
    setSignalAccounts([]);
    try {
      const [info, accounts] = await Promise.all([
        SignalCheckAPI(signalForm.apiURL),
        SignalListAccounts(signalForm.apiURL).catch(() => [] as string[]),
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
    setSignalRegStep('registering');
    setSignalRegError('');
    try {
      await SignalRegister(signalForm.apiURL, signalForm.account, mode, signalRegCaptcha);
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
    setSignalRegStep('verifying');
    setSignalRegError('');
    try {
      await SignalVerify(signalForm.apiURL, signalForm.account, signalRegCode);
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
        const accounts = await SignalListAccounts(signalForm.apiURL);
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
    setSignalLinkQR('');
    setSignalRegError('');
    setSignalLinking(true);
    stopLinkPolling();
    try {
      const qr = await SignalLink(signalForm.apiURL, 'Assistente');
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
    if (!confirm(`Remover a conta ${account} do servidor Signal?\n\nIsto irá desregistrar e apagar os dados locais.`)) return;
    setSignalUnregistering(account);
    try {
      await SignalUnregister(signalForm.apiURL, account, true);
      const accounts = await SignalListAccounts(signalForm.apiURL).catch(() => [] as string[]);
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

  const handleDeleteContact = (row: ContactRow) => {
    setDeleteContactTarget(row);
    setDeleteContactOpen(true);
  };

  const confirmDeleteContact = async () => {
    if (!deleteContactTarget) return;
    try {
      await RemoveAuthorizedContact(deleteContactTarget.channel, deleteContactTarget.contactId);
      addToast('Contato removido', 'success');
      announce('Contato removido');
      await loadAll();
    } catch (error: any) {
      addToast(error.message || 'Erro ao remover contato', 'error');
    } finally {
      setDeleteContactOpen(false);
      setDeleteContactTarget(null);
    }
  };

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

  // ── Editor title ─────────────────────────────────────────────────

  const editorTitle = editingChannel === 'telegram' ? 'Telegram' : editingChannel === 'signal' ? 'Signal' : '';

  // ── Render: Telegram editor ──────────────────────────────────────

  const renderTelegramEditor = () => (
    <>
      <Checkbox
        label="Habilitado"
        checked={telegramForm.enabled}
        onChange={(e) => setTelegramForm((prev) => ({ ...prev, enabled: e.target.checked }))}
      />
      {telegramForm.enabled && (
        <>
          <Input
            label="Bot Token"
            type="password"
            value={telegramForm.botToken}
            onChange={(e) => setTelegramForm((prev) => ({ ...prev, botToken: e.target.value }))}
            placeholder="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
            fullWidth
          />
          <p className="channels-page__hint">
            Crie um bot pelo @BotFather no Telegram e cole o token aqui.
          </p>
          <Input
            label="Max. contatos autorizados"
            type="number"
            value={String(telegramForm.maxContacts)}
            onChange={(e) => setTelegramForm((prev) => ({ ...prev, maxContacts: parseInt(e.target.value) || 1 }))}
            fullWidth
          />
          <p className="channels-page__hint">
            Ao atingir o limite, novos contatos são ignorados silenciosamente.
          </p>
          <ProfilePicker
            value={telegramForm.profile}
            onChange={(slug) => setTelegramForm((prev) => ({ ...prev, profile: slug }))}
            label="Perfil do Canal"
            maxWidth="100%"
            onAnnounce={announce}
          />
          <p className="channels-page__hint">
            Perfil usado para conversas deste canal. Define modelo, voz, STT e comportamento.
            Vazio usa o perfil ativo global.
          </p>
          <Input
            label="Máximo de Histórico"
            type="number"
            min="1"
            max="200"
            value={telegramForm.maxHistory}
            onChange={(e) => setTelegramForm((prev) => ({ ...prev, maxHistory: parseInt(e.target.value) || 50 }))}
            fullWidth
          />
        </>
      )}
    </>
  );

  // ── Render: Signal editor ────────────────────────────────────────

  const renderSignalEditor = () => (
    <>
      <Checkbox
        label="Habilitado"
        checked={signalForm.enabled}
        onChange={(e) => setSignalForm((prev) => ({ ...prev, enabled: e.target.checked }))}
      />
      {signalForm.enabled && (
        <>
          <Input
            label="URL da API"
            value={signalForm.apiURL}
            onChange={(e) => {
              setSignalForm((prev) => ({ ...prev, apiURL: e.target.value }));
              setSignalAPIReady(false);
              setSignalAPIInfo('');
              setSignalRegError('');
              setSignalAccounts([]);
            }}
            placeholder="http://localhost:8080"
            fullWidth
          />
          <div className="channels-page__row">
            <Button
              variant="outline"
              onClick={handleSignalCheckAPI}
              loading={signalCheckingAPI}
              disabled={!signalForm.apiURL}
            >
              Testar Conexão
            </Button>
          </div>
          <div aria-live="polite">
            {signalAPIInfo && <p className="channels-page__hint" role="status">{signalAPIInfo}</p>}
          </div>

          {!signalAPIReady && (
            <p className="channels-page__hint" role="status">Teste a conexão para avançar.</p>
          )}

          {signalAPIReady && signalAccounts.length > 0 && (
            <div className="channels-page__subsection">
              <h4>Conta Conectada</h4>
              <div className="channels-page__account-row">
                <span className="channels-page__account-number">{signalAccounts[0]}</span>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => handleSignalUnregister(signalAccounts[0])}
                  loading={signalUnregistering === signalAccounts[0]}
                  disabled={signalUnregistering !== null}
                >
                  Desconectar
                </Button>
              </div>
            </div>
          )}

          {signalAPIReady && signalAccounts.length === 0 && (
            <div className="channels-page__subsection">
              <h4>Conectar Conta</h4>
              <p className="channels-page__hint">
                Cadastre um novo número ou conecte uma conta existente via QR Code.
              </p>

              <Input
                label="Novo número de telefone"
                value={signalForm.account}
                onChange={(e) => setSignalForm((prev) => ({ ...prev, account: e.target.value }))}
                placeholder="+5511999999999"
                fullWidth
              />

              <div className="channels-page__row">
                <Button
                  variant={signalConnectionMode === 'register' ? 'primary' : 'outline'}
                  onClick={() => {
                    setSignalConnectionMode('register');
                    setSignalRegStep('idle');
                    setSignalRegCode('');
                    setSignalRegCaptcha('');
                    setSignalRegError('');
                    setSignalSmsSent(false);
                    setSignalLinkQR('');
                    setSignalLinking(false);
                  }}
                >
                  Cadastrar número
                </Button>
                <Button
                  variant={signalConnectionMode === 'link' ? 'primary' : 'outline'}
                  onClick={() => {
                    setSignalConnectionMode('link');
                    setSignalRegStep('idle');
                    setSignalRegCode('');
                    setSignalRegCaptcha('');
                    setSignalRegError('');
                    setSignalSmsSent(false);
                  }}
                >
                  Conectar conta existente
                </Button>
              </div>

              <div aria-live="assertive" aria-atomic="true">
                {signalRegError && (
                  <div className="channels-page__alert" role="alert">
                    <strong>Erro:</strong> {signalRegError}
                  </div>
                )}
              </div>

              {signalConnectionMode === 'register' && (
                <>
                  {signalRegStep === 'idle' && (
                    <div className="channels-page__fields">
                      <Input
                        label="Token de Verificação"
                        value={signalRegCaptcha}
                        onChange={(e) => setSignalRegCaptcha(e.target.value)}
                        placeholder="signalcaptcha://signal-hcaptcha.abcdef..."
                        fullWidth
                      />
                      <p className="channels-page__hint">
                        Abra{' '}
                        <a href="https://signalcaptchas.org/registration/generate.html" target="_blank" rel="noopener noreferrer">
                          esta página
                        </a>
                        , complete o desafio, clique direito em "Open Signal" e copie o link.
                      </p>
                      <div className="channels-page__row">
                        <Button variant="outline" onClick={() => handleSignalRegister('sms')} disabled={!signalForm.apiURL || !signalForm.account || !signalRegCaptcha}>
                          Enviar código por SMS
                        </Button>
                      </div>
                    </div>
                  )}

                  {signalRegStep === 'registering' && (
                    <p className="channels-page__hint" role="status" aria-live="polite">Enviando código...</p>
                  )}

                  {(signalRegStep === 'awaiting_code' || signalRegStep === 'verifying') && (
                    <div className="channels-page__fields">
                      <Input
                        label="Código de Verificação"
                        value={signalRegCode}
                        onChange={(e) => setSignalRegCode(e.target.value)}
                        placeholder="123-456"
                        fullWidth
                      />
                      <div className="channels-page__row">
                        <Button variant="outline" onClick={handleSignalVerify} loading={signalRegStep === 'verifying'} disabled={!signalRegCode}>
                          Verificar
                        </Button>
                        <Button variant="outline" onClick={() => handleSignalRegister('voice')} disabled={!signalSmsSent}>
                          Reenviar por Ligação
                        </Button>
                        <Button variant="ghost" onClick={() => { setSignalRegStep('idle'); setSignalRegCode(''); setSignalRegError(''); setSignalSmsSent(false); setSignalRegCaptcha(''); }}>
                          Cancelar
                        </Button>
                      </div>
                    </div>
                  )}

                  {signalRegStep === 'done' && (
                    <div className="channels-page__fields">
                      <div className="channels-page__success" role="status" aria-live="polite">
                        Número {signalForm.account} registrado com sucesso!
                      </div>
                      <Button variant="ghost" onClick={() => { setSignalRegStep('idle'); setSignalSmsSent(false); }}>OK</Button>
                    </div>
                  )}
                </>
              )}

              {signalConnectionMode === 'link' && (
                <div className="channels-page__fields">
                  <div className="channels-page__row">
                    <Button variant="outline" onClick={handleSignalLink} disabled={!signalForm.apiURL || signalLinking} loading={signalLinking}>
                      Gerar QR Code
                    </Button>
                  </div>

                  {(signalLinkQR || signalLinking) && (
                    <div className="channels-page__qr-container" role="region" aria-label="QR Code de vinculação Signal">
                      {signalLinkQR ? (
                        <>
                          <p className="channels-page__hint">Escaneie o QR Code com o Signal no celular:</p>
                          <img src={signalLinkQR} alt="QR Code para vincular dispositivo Signal" className="channels-page__qr-image" />
                        </>
                      ) : (
                        <p className="channels-page__hint" role="status" aria-live="polite">Gerando QR Code...</p>
                      )}
                      {signalLinking && <p className="channels-page__hint" role="status" aria-live="polite">Aguardando vinculação...</p>}
                      <Button variant="ghost" onClick={() => { setSignalLinkQR(''); setSignalLinking(false); stopLinkPolling(); }}>Cancelar</Button>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

          <Input
            label="Max. contatos autorizados"
            type="number"
            value={String(signalForm.maxContacts)}
            onChange={(e) => setSignalForm((prev) => ({ ...prev, maxContacts: parseInt(e.target.value) || 1 }))}
            fullWidth
          />
          <p className="channels-page__hint">
            Ao atingir o limite, novos contatos são ignorados silenciosamente.
          </p>
          <ProfilePicker
            value={signalForm.profile}
            onChange={(slug) => setSignalForm((prev) => ({ ...prev, profile: slug }))}
            label="Perfil do Canal"
            maxWidth="100%"
            onAnnounce={announce}
          />
          <p className="channels-page__hint">
            Perfil usado para conversas deste canal. Define modelo, voz, STT e comportamento.
            Vazio usa o perfil ativo global.
          </p>
          <Input
            label="Máximo de Histórico"
            type="number"
            min="1"
            max="200"
            value={signalForm.maxHistory}
            onChange={(e) => setSignalForm((prev) => ({ ...prev, maxHistory: parseInt(e.target.value) || 50 }))}
            fullWidth
          />
        </>
      )}
    </>
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
      {/* Tabs — ARIA tabs pattern with roving tabindex and arrow keys */}
      <div className="channels-page__tabs" role="tablist" aria-label="Seções de canais">
        {tabs.map((tab, index) => (
          <button
            key={tab.id}
            ref={(el) => { tabRefs.current[index] = el; }}
            role="tab"
            id={`channels-tab-${tab.id}`}
            aria-selected={activeTab === tab.id}
            aria-controls={`channels-tabpanel-${tab.id}`}
            tabIndex={activeTab === tab.id ? 0 : -1}
            className={`channels-page__tab ${activeTab === tab.id ? 'channels-page__tab--active' : ''}`}
            onClick={() => handleTabClick(tab)}
            onKeyDown={(e) => handleTabKeyDown(e, index)}
          >
            {tab.id === 'contacts' ? `${tab.label} (${contactRows.length})` : tab.label}
          </button>
        ))}
      </div>

      <Toolbar
        left={<h1 className="page-toolbar__title">Canais de Comunicação</h1>}
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
        <div
          id="channels-tabpanel-channels"
          role="tabpanel"
          aria-labelledby="channels-tab-channels"
          className="channels-page__content"
        >
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
        </div>
      )}

      {/* Contacts tab panel */}
      {activeTab === 'contacts' && (
        <div
          id="channels-tabpanel-contacts"
          role="tabpanel"
          aria-labelledby="channels-tab-contacts"
          className="channels-page__content"
        >
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
              onCellAction={(item) => handleDeleteContact(item)}
              onDelete={(item) => handleDeleteContact(item)}
              onGridReady={contactsHandleGridReady}
            />
          ) : (
            <p className="channels-page__empty" role="status">Nenhum contato autorizado.</p>
          )}
        </div>
      )}

      {/* Editor Modal */}
      <SimpleModal
        isOpen={!!editingChannel}
        onClose={handleCloseEditor}
        title={`Editor de canal: ${editorTitle}`}
        size="lg"
      >
        <div className="channels-page__fields">
          {editingChannel === 'telegram' && renderTelegramEditor()}
          {editingChannel === 'signal' && renderSignalEditor()}
        </div>
        <div className="channels-page__editor-footer">
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
        </div>
      </SimpleModal>

      {/* Confirm dialog */}
      <ConfirmDialog
        isOpen={deleteContactOpen && deleteContactTarget !== null}
        title="Remover Contato"
        message={deleteContactTarget ? `Remover ${deleteContactTarget.displayName || deleteContactTarget.contactId} do canal ${deleteContactTarget.channel}?` : ''}
        confirmText="Remover"
        cancelText="Cancelar"
        onConfirm={confirmDeleteContact}
        onCancel={() => { setDeleteContactOpen(false); setDeleteContactTarget(null); }}
        variant="danger"
      />
    </div>
  );
}
