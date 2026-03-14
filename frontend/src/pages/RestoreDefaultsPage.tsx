import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ResetDatabase,
  ClearMessages,
  ClearAllCredentials,
  ClearAllProfiles,
  ClearAllSkills,
  ClearAllChannels,
} from '@wailsjs/go/main/App';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { Button } from '../components';
import { CollapsibleSection } from '../components/ui/CollapsibleSection';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useTheme, THEMES, type ThemeId } from '../hooks/useTheme';
import './RestoreDefaultsPage.css';

export default function RestoreDefaultsPage() {
  const { t } = useTranslation();
  const { addToast } = useUIStore();
  const { handleDatabaseReset } = useChatStore();
  const { announce } = useAnnouncer();
  const { theme: currentTheme, setTheme } = useTheme();

  const [loadingOps, setLoadingOps] = useState<Set<string>>(new Set());
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['appearance', 'quick']));

  const isLoading = (opId: string) => loadingOps.has(opId);

  const toggleSection = (sectionId: string) => {
    setExpandedSections((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(sectionId)) {
        newSet.delete(sectionId);
      } else {
        newSet.add(sectionId);
      }
      return newSet;
    });
  };

  const isSectionOpen = (sectionId: string) => expandedSections.has(sectionId);

  const performReset = async (
    opId: string,
    confirmMessage: string,
    requiresDual: boolean,
    fn: () => Promise<void>,
    onSuccess?: () => void
  ) => {
    if (!confirm(confirmMessage)) return;
    if (requiresDual && !confirm('⚠️ Esta é sua ÚLTIMA CHANCE!\n\nConfirmar a operação?')) return;

    setLoadingOps((prev) => new Set([...prev, opId]));
    try {
      await fn();
      addToast(`${opId} concluído com sucesso!`, 'success');
      announce(`${opId} realizado`);
      onSuccess?.();
    } catch (error: any) {
      console.error(`Erro em ${opId}:`, error);
      addToast(error.message || `Erro ao executar ${opId}`, 'error');
    } finally {
      setLoadingOps((prev) => {
        const newSet = new Set(prev);
        newSet.delete(opId);
        return newSet;
      });
    }
  };

  const handleClearMessages = () =>
    performReset(
      'Limpar Mensagens',
      'Tem certeza que deseja APAGAR todas as mensagens e conversas?\n\nIsso irá remover permanentemente todas as abas e históricos.\n\nEsta ação NÃO pode ser desfeita!',
      true,
      async () => await ClearMessages()
    );

  const handleClearCredentials = () =>
    performReset(
      'Limpar Credenciais',
      'Tem certeza que deseja APAGAR todas as credenciais armazenadas?\n\nVocê precisará configurar novos provedores.\n\nEsta ação NÃO pode ser desfeita!',
      true,
      async () => await ClearAllCredentials()
    );

  const handleClearProfiles = () =>
    performReset(
      'Limpar Perfis',
      'Tem certeza que deseja APAGAR todos os perfis?\n\nTodos os perfis customizados serão removidos.\n\nEsta ação NÃO pode ser desfeita!',
      true,
      async () => await ClearAllProfiles()
    );

  const handleClearSkills = () =>
    performReset(
      'Limpar Skills',
      'Tem certeza que deseja APAGAR todos os skills?\n\nTodos os skills customizados serão removidos.\n\nEsta ação NÃO pode ser desfeita!',
      true,
      async () => await ClearAllSkills()
    );

  const handleClearChannels = () =>
    performReset(
      'Limpar Canais',
      'Tem certeza que deseja APAGAR todas as configurações de canais de comunicação?\n\nTodos os canais (Telegram, Slack, Signal, etc) serão desconfigurados.\n\nEsta ação NÃO pode ser desfeita!',
      true,
      async () => await ClearAllChannels()
    );

  const handleResetDatabase = () =>
    performReset(
      'Apagar Banco de Dados',
      'ATENÇÃO: Tem certeza que deseja apagar o banco de dados INTEIRO?\n\nIsso irá REMOVER PERMANENTEMENTE:\n- Todas as conversas\n- Todos os históricos\n- Todas as abas\n\nEsta ação NÃO pode ser desfeita!',
      true,
      async () => {
        await ResetDatabase();
        handleDatabaseReset();
      }
    );

  const handleClearAll = () =>
    performReset(
      'Limpar Tudo',
      '🚨 OPERAÇÃO NUCLEAR 🚨\n\nTem certeza que deseja LIMPAR COMPLETAMENTE o assistente?\n\nIsso irá remover permanentemente:\n✓ Todas as conversas e mensagens\n✓ Todas as credenciais\n✓ Todos os perfis\n✓ Todos os skills\n✓ Todas as configurações de canais\n✓ Banco de dados inteiro\n\nO assistente voltará ao estado inicial.\n\nEsta ação NÃO pode ser desfeita!',
      true,
      async () => {
        await ClearMessages();
        await ClearAllCredentials();
        await ClearAllProfiles();
        await ClearAllSkills();
        await ClearAllChannels();
        await ResetDatabase();
        handleDatabaseReset();
      }
    );

  return (
    <div className="restore-defaults-page">
      <header className="restore-header">
        <h1>{t('restore.pageTitle')}</h1>
        <p>{t('restore.description')}</p>
      </header>

      <main className="restore-content">
        {/* Appearance - Aparência */}
        <CollapsibleSection
          title={t('restore.sections.appearance')}
          isOpen={isSectionOpen('appearance')}
          onToggle={() => toggleSection('appearance')}
          ariaLabel={t('restore.aria.appearance')}
        >
          <div className="theme-grid" role="radiogroup" aria-label={t('restore.aria.selectTheme')}>
            {THEMES.map((theme) => (
              <button
                key={theme.id}
                className={`theme-card${currentTheme === theme.id ? ' theme-card--active' : ''}`}
                role="radio"
                aria-checked={currentTheme === theme.id}
                onClick={() => {
                  setTheme(theme.id as ThemeId);
                  announce(t('restore.announce.themeChanged', { label: theme.label }));
                }}
              >
                <div className="theme-card__preview">
                  {theme.id === 'assistente' && (
                    <>
                      <div className="theme-card__swatch" style={{ background: '#0a1628' }} />
                      <div className="theme-card__swatch" style={{ background: '#0f1f3a' }} />
                      <div className="theme-card__swatch" style={{ background: '#2b7ef4' }} />
                      <div className="theme-card__swatch" style={{ background: '#eef2f9' }} />
                    </>
                  )}
                  {theme.id === 'amethyst' && (
                    <>
                      <div className="theme-card__swatch" style={{ background: '#12082a' }} />
                      <div className="theme-card__swatch" style={{ background: '#1c1040' }} />
                      <div className="theme-card__swatch" style={{ background: '#a78bfa' }} />
                      <div className="theme-card__swatch" style={{ background: '#f0ecf9' }} />
                    </>
                  )}
                  {theme.id === 'midnight' && (
                    <>
                      <div className="theme-card__swatch" style={{ background: '#0c0f14' }} />
                      <div className="theme-card__swatch" style={{ background: '#151921' }} />
                      <div className="theme-card__swatch" style={{ background: '#60a5fa' }} />
                      <div className="theme-card__swatch" style={{ background: '#e8ecf2' }} />
                    </>
                  )}
                  {theme.id === 'light' && (
                    <>
                      <div className="theme-card__swatch" style={{ background: '#f0f4fa' }} />
                      <div className="theme-card__swatch" style={{ background: '#ffffff' }} />
                      <div className="theme-card__swatch" style={{ background: '#2b7ef4' }} />
                      <div className="theme-card__swatch" style={{ background: '#0a1628' }} />
                    </>
                  )}
                  {theme.id === 'high-contrast' && (
                    <>
                      <div className="theme-card__swatch" style={{ background: '#000000' }} />
                      <div className="theme-card__swatch" style={{ background: '#1a1a1a' }} />
                      <div className="theme-card__swatch" style={{ background: '#5babff' }} />
                      <div className="theme-card__swatch" style={{ background: '#ffffff' }} />
                    </>
                  )}
                </div>
                <span className="theme-card__name">{theme.label}</span>
                <span className="theme-card__desc">{theme.description}</span>
              </button>
            ))}
          </div>
        </CollapsibleSection>

        {/* Quick Actions - Operações Rápidas */}
        <CollapsibleSection
          title={t('restore.sections.quickActions')}
          isOpen={isSectionOpen('quick')}
          onToggle={() => toggleSection('quick')}
          ariaLabel={t('restore.aria.quickActions')}
        >
          <div className="restore-item">
            <div className="restore-item-info">
              <h3>{t('restore.items.clearMessages')}</h3>
              <p>{t('restore.items.clearMessagesDesc')}</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearMessages}
              loading={isLoading('Limpar Mensagens')}
            >
              {t('restore.buttons.clear')}
            </Button>
          </div>
        </CollapsibleSection>

        {/* Granular Cleanup - Limpeza Granular */}
        <CollapsibleSection
          title={t('restore.sections.granular')}
          isOpen={isSectionOpen('granular')}
          onToggle={() => toggleSection('granular')}
          ariaLabel={t('restore.aria.granular')}
        >
          <div className="restore-item">
            <div className="restore-item-info">
              <h3>{t('restore.items.clearCredentials')}</h3>
              <p>{t('restore.items.clearCredentialsDesc')}</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearCredentials}
              loading={isLoading('Limpar Credenciais')}
            >
              {t('restore.buttons.clear')}
            </Button>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>{t('restore.items.clearProfiles')}</h3>
              <p>{t('restore.items.clearProfilesDesc')}</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearProfiles}
              loading={isLoading('Limpar Perfis')}
            >
              {t('restore.buttons.clear')}
            </Button>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>{t('restore.items.clearSkills')}</h3>
              <p>{t('restore.items.clearSkillsDesc')}</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearSkills}
              loading={isLoading('Limpar Skills')}
            >
              {t('restore.buttons.clear')}
            </Button>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>{t('restore.items.clearChannels')}</h3>
              <p>{t('restore.items.clearChannelsDesc')}</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearChannels}
              loading={isLoading('Limpar Canais')}
            >
              {t('restore.buttons.clear')}
            </Button>
          </div>
        </CollapsibleSection>

        {/* Nuclear Options - Opções Nucleares */}
        <CollapsibleSection
          title={t('restore.sections.nuclear')}
          isOpen={isSectionOpen('nuclear')}
          onToggle={() => toggleSection('nuclear')}
          ariaLabel={t('restore.aria.nuclear')}
        >
          <div className="restore-item restore-item-danger">
            <div className="restore-item-info">
              <h3>{t('restore.items.resetDatabase')}</h3>
              <p>{t('restore.items.resetDatabaseDesc')}</p>
            </div>
            <Button
              variant="danger"
              onClick={handleResetDatabase}
              loading={isLoading('Apagar Banco de Dados')}
            >
              {t('restore.buttons.delete')}
            </Button>
          </div>

          <div className="restore-item restore-item-danger">
            <div className="restore-item-info">
              <h3>{t('restore.items.clearAll')}</h3>
              <p>{t('restore.items.clearAllDesc')}</p>
            </div>
            <Button
              variant="danger"
              onClick={handleClearAll}
              loading={isLoading('Limpar Tudo')}
            >
              {t('restore.buttons.clearAll')}
            </Button>
          </div>
        </CollapsibleSection>

        {/* Master Password Management - Gerenciamento de Senha Mestre */}
        <CollapsibleSection
          title={t('restore.sections.security')}
          isOpen={isSectionOpen('security')}
          onToggle={() => toggleSection('security')}
          ariaLabel={t('restore.aria.security')}
        >
          <div className="security-info">
            <p>
              {t('restore.security.masterPasswordInfo')}
            </p>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>{t('restore.items.resetMasterPassword')}</h3>
              <p>{t('restore.items.resetMasterPasswordDesc')}</p>
            </div>
            <Button variant="outline" disabled>
              {t('restore.buttons.resetSoon')}
            </Button>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>{t('restore.items.recoveryCode')}</h3>
              <p>{t('restore.items.recoveryCodeDesc')}</p>
            </div>
            <Button variant="outline" disabled>
              {t('restore.buttons.viewCodeSoon')}
            </Button>
          </div>

          <div className="restore-item restore-item-warning">
            <div className="restore-item-info">
              <h3>{t('restore.items.removeMasterPassword')}</h3>
              <p>{t('restore.items.removeMasterPasswordDesc')}</p>
            </div>
            <Button variant="outline" disabled>
              {t('restore.buttons.removeSoon')}
            </Button>
          </div>
        </CollapsibleSection>
      </main>
    </div>
  );
}
