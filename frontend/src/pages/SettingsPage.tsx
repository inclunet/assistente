import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { GetConfig, SaveSettings, ResetConfig, ResetDatabase, TestConnectionWithModels, GetMessagingConfig, SaveMessagingConfig, GetMessagingStatus, SignalCheckAPI, SignalListAccounts, SignalRegister, SignalVerify, SignalLink, SignalUnregister } from '../../wailsjs/go/main/App';
import { config, llm } from '../../wailsjs/go/models';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { Input, Button, Checkbox } from '../components';
import { ProfilePicker, ProfilePickerRef } from '../components/pickers/ProfilePicker';
import { useAnnouncer } from '../hooks/useAnnouncer';
import './SettingsPage.css';

interface FormData {
  apiKey: string;
  apiBaseURL: string;
  responseTimeout: number;
}

interface TelegramForm {
  enabled: boolean;
  botToken: string;
  allowedContacts: string;
  profile: string;
  maxHistory: number;
}

interface SignalForm {
  enabled: boolean;
  apiURL: string;
  account: string;
  allowedContacts: string;
  profile: string;
  maxHistory: number;
}

type SignalRegisterStep = 'idle' | 'registering' | 'awaiting_code' | 'verifying' | 'done';

export default function SettingsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { addToast } = useUIStore();
  const { handleDatabaseReset } = useChatStore();
  const { announce } = useAnnouncer();
  
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [availableModels, setAvailableModels] = useState<string[]>([]);
  
  const profilePickerRef = useRef<ProfilePickerRef>(null);
  
  const [formData, setFormData] = useState<FormData>({
    apiKey: '',
    apiBaseURL: 'https://api.openai.com/v1',
    responseTimeout: 180,
  });

  // Messaging state
  const [telegramForm, setTelegramForm] = useState<TelegramForm>({
    enabled: false, botToken: '', allowedContacts: '', profile: '', maxHistory: 50,
  });
  const [signalForm, setSignalForm] = useState<SignalForm>({
    enabled: false, apiURL: '', account: '', allowedContacts: '', profile: '', maxHistory: 50,
  });
  const [messagingStatus, setMessagingStatus] = useState<Record<string, string>>({});
  const [savingMessaging, setSavingMessaging] = useState(false);

  // Signal registration
  const [signalRegStep, setSignalRegStep] = useState<SignalRegisterStep>('idle');
  const [signalRegCode, setSignalRegCode] = useState('');
  const [signalRegCaptcha, setSignalRegCaptcha] = useState('');
  const [signalRegError, setSignalRegError] = useState('');
  const [signalSmsSent, setSignalSmsSent] = useState(false);
  const [signalCheckingAPI, setSignalCheckingAPI] = useState(false);
  const [signalAPIInfo, setSignalAPIInfo] = useState<string>('');
  const [signalAccounts, setSignalAccounts] = useState<string[]>([]);
  const [signalLinkQR, setSignalLinkQR] = useState('');
  const [signalLinking, setSignalLinking] = useState(false);
  const [signalUnregistering, setSignalUnregistering] = useState<string | null>(null);
  const linkPollRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    loadConfig();
    loadMessagingConfig();
  }, []);

  const loadConfig = async () => {
    try {
      setLoading(true);
      const config = await GetConfig();
      
      if (config) {
        setFormData({
          apiKey: config.api_key || '',
          apiBaseURL: config.api_base_url || 'https://api.openai.com/v1',
          responseTimeout: config.response_timeout || 180,
        });
      }
    } catch (error) {
      console.error('Erro ao carregar configuração:', error);
      addToast('Erro ao carregar configuração', 'error');
    } finally {
      setLoading(false);
    }
  };

  const loadMessagingConfig = async () => {
    try {
      const msgCfg = await GetMessagingConfig();
      if (msgCfg?.telegram) {
        setTelegramForm({
          enabled: msgCfg.telegram.enabled || false,
          botToken: msgCfg.telegram.bot_token || '',
          allowedContacts: (msgCfg.telegram.allowed_contacts || []).join(', '),
          profile: msgCfg.telegram.profile || '',
          maxHistory: msgCfg.telegram.max_history || 50,
        });
      }
      if (msgCfg?.signal) {
        setSignalForm({
          enabled: msgCfg.signal.enabled || false,
          apiURL: msgCfg.signal.api_url || '',
          account: msgCfg.signal.account || '',
          allowedContacts: (msgCfg.signal.allowed_contacts || []).join(', '),
          profile: msgCfg.signal.profile || '',
          maxHistory: msgCfg.signal.max_history || 50,
        });
      }
      const status = await GetMessagingStatus();
      setMessagingStatus(status);
    } catch (error) {
      console.error('Erro ao carregar configuração de mensageria:', error);
    }
  };

  const handleSaveMessaging = async () => {
    setSavingMessaging(true);
    try {
      const parseContacts = (s: string) =>
        s.split(',').map((c) => c.trim()).filter(Boolean);
      
      const msgCfg = config.MessagingConfig.createFrom({
        telegram: config.ChannelConfig.createFrom({
          enabled: telegramForm.enabled,
          bot_token: telegramForm.botToken,
          allowed_contacts: parseContacts(telegramForm.allowedContacts),
          profile: telegramForm.profile,
          max_history: telegramForm.maxHistory,
        }),
        signal: config.ChannelConfig.createFrom({
          enabled: signalForm.enabled,
          api_url: signalForm.apiURL,
          account: signalForm.account,
          allowed_contacts: parseContacts(signalForm.allowedContacts),
          profile: signalForm.profile,
          max_history: signalForm.maxHistory,
        }),
      });

      await SaveMessagingConfig(msgCfg);
      addToast('Configuração de mensageria salva!', 'success');
      announce('Configuração de mensageria salva');
    } catch (error: any) {
      console.error('Erro ao salvar configuração de mensageria:', error);
      addToast(error.message || 'Erro ao salvar configuração de mensageria', 'error');
    } finally {
      setSavingMessaging(false);
    }
  };

  // Signal API check + list accounts
  const handleSignalCheckAPI = async () => {
    if (!signalForm.apiURL) {
      const msg = 'Informe a URL da API Signal';
      addToast(msg, 'error');
      announce('Erro: ' + msg);
      return;
    }
    setSignalCheckingAPI(true);
    setSignalAPIInfo('');
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
        // Auto-preenche a conta se estiver vazia
        if (!signalForm.account) {
          setSignalForm((prev) => ({ ...prev, account: accounts[0] }));
        }
      } else {
        infoText += ' Nenhuma conta registrada — registre ou vincule abaixo.';
      }

      setSignalAPIInfo(infoText);
      addToast('Signal API acessível!', 'success');
      announce(infoText);
    } catch (error: any) {
      setSignalAPIInfo('');
      const msg = error.message || 'Não foi possível conectar à API Signal';
      setSignalRegError(msg);
      addToast(msg, 'error');
      announce('Erro: ' + msg);
    } finally {
      setSignalCheckingAPI(false);
    }
  };

  // Signal registration flow
  const handleSignalRegister = async (mode: 'sms' | 'voice' = 'sms') => {
    if (!signalForm.account || !signalForm.apiURL) {
      const msg = 'Informe a URL da API e o número de telefone no campo Conta';
      setSignalRegError(msg);
      addToast(msg, 'error');
      announce('Erro: ' + msg);
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
      const successMsg = `Código enviado por ${modeLabel} para ${signalForm.account}`;
      addToast(successMsg, 'success');
      announce(successMsg);
    } catch (error: any) {
      setSignalRegStep(signalSmsSent ? 'awaiting_code' : 'idle');
      const msg = error.message || 'Erro ao registrar número';
      setSignalRegError(msg);
      addToast(msg, 'error');
      announce('Erro no registro: ' + msg);
    }
  };

  const handleSignalVerify = async () => {
    if (!signalRegCode) {
      const msg = 'Informe o código de verificação';
      setSignalRegError(msg);
      addToast(msg, 'error');
      announce('Erro: ' + msg);
      return;
    }
    setSignalRegStep('verifying');
    setSignalRegError('');
    try {
      await SignalVerify(signalForm.apiURL, signalForm.account, signalRegCode);
      setSignalRegStep('done');
      setSignalSmsSent(false);
      const successMsg = `Número ${signalForm.account} verificado com sucesso`;
      addToast(successMsg, 'success');
      announce(successMsg);
    } catch (error: any) {
      setSignalRegStep('awaiting_code');
      const msg = error.message || 'Erro ao verificar código';
      setSignalRegError(msg);
      addToast(msg, 'error');
      announce('Erro na verificação: ' + msg);
    }
  };

  // Limpa polling ao desmontar
  useEffect(() => {
    return () => {
      if (linkPollRef.current) clearTimeout(linkPollRef.current);
    };
  }, []);

  const stopLinkPolling = () => {
    if (linkPollRef.current) {
      clearTimeout(linkPollRef.current);
      linkPollRef.current = null;
    }
  };

  // Polling recursivo: verifica a cada 5s se uma conta apareceu após vincular.
  const startLinkPolling = (startTime: number) => {
    const POLL_TIMEOUT_MS = 2 * 60 * 1000;

    linkPollRef.current = setTimeout(async () => {
      if (Date.now() - startTime > POLL_TIMEOUT_MS) {
        setSignalLinking(false);
        const msg = 'Tempo esgotado. Verifique os logs do container signal-cli-rest-api.';
        setSignalRegError(msg);
        addToast(msg, 'error');
        announce('Tempo esgotado na vinculação.');
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
          const msg = `Dispositivo vinculado com sucesso! Conta: ${accounts[0]}`;
          addToast(msg, 'success');
          announce(msg);
          return;
        }
      } catch {
        // Ignora erros de polling
      }

      // Continua polling
      startLinkPolling(startTime);
    }, 5000);
  };

  const handleSignalLink = async () => {
    if (!signalForm.apiURL) {
      const msg = 'Informe a URL da API Signal';
      setSignalRegError(msg);
      addToast(msg, 'error');
      announce('Erro: ' + msg);
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
      addToast('QR Code gerado!', 'success');

      // Inicia polling para detectar quando a vinculação concluir
      startLinkPolling(Date.now());
    } catch (error: any) {
      const msg = error.message || 'Erro ao gerar QR de vinculação';
      setSignalRegError(msg);
      addToast(msg, 'error');
      announce('Erro: ' + msg);
      setSignalLinking(false);
    }
  };

  const handleSignalUnregister = async (account: string) => {
    if (!confirm(`Tem certeza que deseja remover a conta ${account} do servidor Signal?\n\nIsto irá desregistrar a conta e apagar os dados locais.`)) {
      return;
    }
    setSignalUnregistering(account);
    try {
      await SignalUnregister(signalForm.apiURL, account, true);
      // Recarrega a lista de contas
      const accounts = await SignalListAccounts(signalForm.apiURL).catch(() => [] as string[]);
      setSignalAccounts(accounts || []);
      // Limpa a conta do formulário se era a que foi removida
      if (signalForm.account === account) {
        setSignalForm((prev) => ({ ...prev, account: accounts?.[0] || '' }));
      }
      const successMsg = `Conta ${account} removida com sucesso`;
      addToast(successMsg, 'success');
      announce(successMsg);
    } catch (error: any) {
      const msg = error.message || 'Erro ao remover conta';
      addToast(msg, 'error');
      announce('Erro: ' + msg);
    } finally {
      setSignalUnregistering(null);
    }
  };

  const handleChange = (field: keyof FormData, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const settings = llm.SettingsInput.createFrom({
        api_key: formData.apiKey,
        api_base_url: formData.apiBaseURL,
        response_timeout: formData.responseTimeout,
      });
      await SaveSettings(settings);
      addToast('Configurações salvas com sucesso!', 'success');
      announce('Configurações salvas');
    } catch (error: any) {
      console.error('Erro ao salvar configurações:', error);
      addToast(error.message || 'Erro ao salvar configurações', 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleTestConnection = async () => {
    setTesting(true);
    try {
      const models = await TestConnectionWithModels();
      setAvailableModels(models);
      addToast(`Conexão OK! ${models.length} modelos disponíveis.`, 'success');
      announce(`Conexão estabelecida. ${models.length} modelos disponíveis.`);
    } catch (error: any) {
      console.error('Erro ao testar conexão:', error);
      addToast(error.message || 'Erro ao conectar com a API', 'error');
      announce('Falha na conexão');
    } finally {
      setTesting(false);
    }
  };

  const handleResetConfig = async () => {
    if (!confirm('Tem certeza que deseja resetar as configurações?\n\nIsso irá apagar o arquivo config.json e restaurar os valores padrão.')) {
      return;
    }
    
    try {
      await ResetConfig();
      addToast('Configurações resetadas com sucesso!', 'success');
      announce('Configurações resetadas');
      await loadConfig();
    } catch (error: any) {
      console.error('Erro ao resetar configurações:', error);
      addToast(error.message || 'Erro ao resetar configurações', 'error');
    }
  };

  const handleResetDatabase = async () => {
    if (!confirm('ATENÇÃO: Tem certeza que deseja apagar o banco de dados?\n\nIsso irá REMOVER PERMANENTEMENTE todas as conversas.\n\nEsta ação NÃO pode ser desfeita!')) {
      return;
    }
    
    if (!confirm('Esta é sua ÚLTIMA CHANCE!\n\nConfirmar exclusão de todos os dados?')) {
      return;
    }
    
    try {
      await ResetDatabase();
      handleDatabaseReset();
      addToast('Banco de dados resetado com sucesso!', 'success');
      announce('Banco de dados resetado');
    } catch (error: any) {
      console.error('Erro ao resetar banco de dados:', error);
      addToast(error.message || 'Erro ao resetar banco de dados', 'error');
    }
  };

  if (loading) {
    return (
      <div className="settings-page">
        <div className="settings-loading">Carregando configurações...</div>
      </div>
    );
  }

  return (
    <div className="settings-page">
      <div className="settings-header">
        <h1>{t('settings.title', 'Configurações')}</h1>
        <p>{t('settings.subtitle', 'Configure as opções do assistente')}</p>
      </div>

      <div className="settings-content">
        {/* API Section */}
        <section className="settings-section">
          <h2>API</h2>
          <div className="settings-fields">
            <Input
              label="API Key"
              type="password"
              value={formData.apiKey}
              onChange={(e) => handleChange('apiKey', e.target.value)}
              placeholder="sk-..."
              fullWidth
            />

            <Input
              label="Base URL"
              value={formData.apiBaseURL}
              onChange={(e) => handleChange('apiBaseURL', e.target.value)}
              placeholder="https://api.openai.com/v1"
              fullWidth
            />
            
            <Button 
              variant="outline" 
              onClick={handleTestConnection} 
              loading={testing}
              disabled={!formData.apiKey}
            >
              Testar Conexão
            </Button>
            
            {availableModels.length > 0 && (
              <p className="settings-field-hint">
                {availableModels.length} modelos disponíveis na API
              </p>
            )}

            <Input
              label="Timeout de Resposta (segundos)"
              type="number"
              min="30"
              max="600"
              step="10"
              value={formData.responseTimeout}
              onChange={(e) => handleChange('responseTimeout', parseInt(e.target.value) || 180)}
              fullWidth
            />
            <p className="settings-field-hint">
              Tempo máximo para aguardar o início da resposta do modelo (padrão: 180s).
            </p>
          </div>
        </section>

        {/* Profile Section */}
        <section className="settings-section">
          <h2>Perfil Ativo</h2>
          <p className="settings-section-description">
            O perfil ativo define modelo, temperatura, voz e interação para todas as conversas.
          </p>
          <div className="settings-fields">
            <ProfilePicker
              ref={profilePickerRef}
              label="Perfil Ativo"
              maxWidth="100%"
              onAnnounce={announce}
            />
            <Button variant="outline" onClick={() => navigate('/profiles')}>
              Gerenciar Perfis
            </Button>
          </div>
        </section>

        {/* Telegram Section */}
        <section className="settings-section">
          <h2>Telegram</h2>
          <p className="settings-section-description">
            Configure um Bot do Telegram para comunicação remota.
            {messagingStatus['telegram'] && (
              <> — Status: <strong>{messagingStatus['telegram']}</strong></>
            )}
          </p>
          <div className="settings-fields">
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
                <p className="settings-field-hint">
                  Crie um bot pelo @BotFather no Telegram e cole o token aqui.
                </p>
                <Input
                  label="Contatos Permitidos (IDs separados por vírgula)"
                  value={telegramForm.allowedContacts}
                  onChange={(e) => setTelegramForm((prev) => ({ ...prev, allowedContacts: e.target.value }))}
                  placeholder="123456789, 987654321"
                  fullWidth
                />
                <p className="settings-field-hint">
                  IDs numéricos dos usuários Telegram autorizados. Deixe vazio para permitir todos.
                </p>
                <div className="settings-row">
                  <Input
                    label="Perfil"
                    value={telegramForm.profile}
                    onChange={(e) => setTelegramForm((prev) => ({ ...prev, profile: e.target.value }))}
                    placeholder="padrão"
                    fullWidth
                  />
                  <Input
                    label="Máximo de Histórico"
                    type="number"
                    min="1"
                    max="200"
                    value={telegramForm.maxHistory}
                    onChange={(e) => setTelegramForm((prev) => ({ ...prev, maxHistory: parseInt(e.target.value) || 50 }))}
                    fullWidth
                  />
                </div>
              </>
            )}
          </div>
        </section>

        {/* Signal Section */}
        <section className="settings-section">
          <h2>Signal</h2>
          <p className="settings-section-description">
            Configure o Signal via signal-cli-rest-api (HTTP + WebSocket).
            {messagingStatus['signal'] && (
              <> — Status: <strong>{messagingStatus['signal']}</strong></>
            )}
          </p>
          <div className="settings-fields">
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
                  onChange={(e) => setSignalForm((prev) => ({ ...prev, apiURL: e.target.value }))}
                  placeholder="http://localhost:8080"
                  fullWidth
                />
                <div className="settings-row">
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
                  {signalAPIInfo && (
                    <p className="settings-field-hint" role="status">{signalAPIInfo}</p>
                  )}
                </div>

                <Input
                  label="Conta (número de telefone)"
                  value={signalForm.account}
                  onChange={(e) => setSignalForm((prev) => ({ ...prev, account: e.target.value }))}
                  placeholder="+5511999999999"
                  fullWidth
                />

                {/* Signal Accounts - show when accounts exist on server */}
                {signalAccounts.length > 0 && (
                <div className="settings-subsection">
                  <h3>Contas Registradas</h3>
                  <div className="settings-fields">
                    {signalAccounts.map((acc) => (
                      <div key={acc} className="settings-row settings-account-row">
                        <span className="settings-account-number">{acc}</span>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => handleSignalUnregister(acc)}
                          loading={signalUnregistering === acc}
                          disabled={signalUnregistering !== null}
                        >
                          Remover Conta
                        </Button>
                      </div>
                    ))}
                  </div>
                </div>
                )}

                {/* Signal Registration - only show if no accounts on server */}
                {signalAccounts.length === 0 && (
                <div className="settings-subsection">
                  <h3>Registrar ou Vincular Conta</h3>
                  <p className="settings-field-hint">
                    Nenhuma conta encontrada no servidor. Registre um número novo (requer token + SMS)
                    ou vincule como dispositivo secundário (sem captcha).
                  </p>

                  <div aria-live="assertive" aria-atomic="true">
                    {signalRegError && (
                      <div className="settings-alert" role="alert">
                        <strong>Erro:</strong> {signalRegError}
                      </div>
                    )}
                  </div>

                  {signalRegStep === 'idle' && (
                    <div className="settings-fields">
                      <Input
                        label="Token de Verificação"
                        value={signalRegCaptcha}
                        onChange={(e) => setSignalRegCaptcha(e.target.value)}
                        placeholder="signalcaptcha://signal-hcaptcha.abcdef..."
                        fullWidth
                      />
                      <p className="settings-field-hint">
                        O Signal exige um token para registro. Abra{' '}
                        <a href="https://signalcaptchas.org/registration/generate.html" target="_blank" rel="noopener noreferrer">
                          esta página
                        </a>
                        , complete o desafio (há opção de áudio), clique com o botão direito em "Open Signal" e copie o link.
                      </p>
                      <div className="settings-row">
                        <Button
                          variant="outline"
                          onClick={() => handleSignalRegister('sms')}
                          disabled={!signalForm.apiURL || !signalForm.account || !signalRegCaptcha}
                        >
                          Enviar código por SMS
                        </Button>
                        <Button
                          variant="outline"
                          onClick={handleSignalLink}
                          disabled={!signalForm.apiURL || signalLinking}
                          loading={signalLinking}
                        >
                          Vincular Dispositivo (QR)
                        </Button>
                      </div>
                    </div>
                  )}

                  {signalRegStep === 'registering' && (
                    <p className="settings-field-hint" role="status" aria-live="polite">Enviando código de verificação...</p>
                  )}

                  {(signalRegStep === 'awaiting_code' || signalRegStep === 'verifying') && (
                    <div className="settings-fields">
                      <Input
                        label="Código de Verificação"
                        value={signalRegCode}
                        onChange={(e) => setSignalRegCode(e.target.value)}
                        placeholder="123-456"
                        fullWidth
                      />
                      <div className="settings-row">
                        <Button
                          variant="outline"
                          onClick={handleSignalVerify}
                          loading={signalRegStep === 'verifying'}
                          disabled={!signalRegCode}
                        >
                          Verificar Código
                        </Button>
                        <Button
                          variant="outline"
                          onClick={() => handleSignalRegister('voice')}
                          disabled={!signalSmsSent}
                        >
                          Reenviar por Ligação
                        </Button>
                        <Button
                          variant="ghost"
                          onClick={() => { setSignalRegStep('idle'); setSignalRegCode(''); setSignalRegError(''); setSignalSmsSent(false); setSignalRegCaptcha(''); }}
                        >
                          Cancelar
                        </Button>
                      </div>
                    </div>
                  )}

                  {signalRegStep === 'done' && (
                    <div className="settings-fields">
                      <div className="settings-success" role="status" aria-live="polite">
                        Número {signalForm.account} registrado com sucesso!
                      </div>
                      <Button variant="ghost" onClick={() => { setSignalRegStep('idle'); setSignalSmsSent(false); }}>
                        OK
                      </Button>
                    </div>
                  )}

                  {(signalLinkQR || signalLinking) && (
                    <div className="settings-qr-container" role="region" aria-label="QR Code de vinculação Signal">
                      {signalLinkQR ? (
                        <>
                          <p className="settings-field-hint">
                            Escaneie o QR Code com o Signal no celular:
                          </p>
                          <img
                            src={signalLinkQR}
                            alt="QR Code para vincular dispositivo Signal"
                            className="settings-qr-image"
                          />
                        </>
                      ) : (
                        <p className="settings-field-hint" role="status" aria-live="polite">
                          Gerando QR Code...
                        </p>
                      )}
                      {signalLinking && (
                        <p className="settings-field-hint" role="status" aria-live="polite">
                          Aguardando vinculação... Escaneie e confirme no celular.
                        </p>
                      )}
                      <Button variant="ghost" onClick={() => { setSignalLinkQR(''); setSignalLinking(false); stopLinkPolling(); }}>
                        Cancelar
                      </Button>
                    </div>
                  )}
                </div>
                )}

                <Input
                  label="Contatos Permitidos (separados por vírgula)"
                  value={signalForm.allowedContacts}
                  onChange={(e) => setSignalForm((prev) => ({ ...prev, allowedContacts: e.target.value }))}
                  placeholder="+5511999999999, 5ac5d632-2b9b-4967-..."
                  fullWidth
                />
                <p className="settings-field-hint">
                  Números de telefone ou UUIDs autorizados a enviar mensagens. Use * para permitir qualquer contato.
                  Contatos bloqueados geram um alerta com opção de autorizar diretamente.
                </p>
                <div className="settings-row">
                  <Input
                    label="Perfil"
                    value={signalForm.profile}
                    onChange={(e) => setSignalForm((prev) => ({ ...prev, profile: e.target.value }))}
                    placeholder="padrão"
                    fullWidth
                  />
                  <Input
                    label="Máximo de Histórico"
                    type="number"
                    min="1"
                    max="200"
                    value={signalForm.maxHistory}
                    onChange={(e) => setSignalForm((prev) => ({ ...prev, maxHistory: parseInt(e.target.value) || 50 }))}
                    fullWidth
                  />
                </div>
              </>
            )}
          </div>
          {(telegramForm.enabled || signalForm.enabled) && (
            <div className="settings-section-footer">
              <Button onClick={handleSaveMessaging} loading={savingMessaging}>
                Salvar Mensageria
              </Button>
            </div>
          )}
        </section>

        {/* Danger Zone */}
        <section className="settings-section settings-danger">
          <h2>Zona de Perigo</h2>
          <p className="settings-danger-warning">
            As ações abaixo são irreversíveis. Tenha certeza antes de prosseguir.
          </p>
          <div className="settings-danger-actions">
            <div className="settings-danger-item">
              <div>
                <strong>Resetar Configurações</strong>
                <p>Apaga o arquivo config.json e restaura os valores padrão.</p>
              </div>
              <Button variant="outline" onClick={handleResetConfig}>
                Resetar Config
              </Button>
            </div>
            
            <div className="settings-danger-item">
              <div>
                <strong>Apagar Banco de Dados</strong>
                <p>Remove permanentemente todas as conversas.</p>
              </div>
              <Button variant="danger" onClick={handleResetDatabase}>
                Apagar Tudo
              </Button>
            </div>
          </div>
        </section>
      </div>

      <div className="settings-footer">
        <Button variant="ghost" onClick={loadConfig} disabled={saving}>
          Recarregar
        </Button>
        <Button onClick={handleSave} loading={saving}>
          Salvar Configurações
        </Button>
      </div>
    </div>
  );
}
