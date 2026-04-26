import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ResetDatabase,
  ClearMessages,
  ClearAllCredentials,
  ClearAllProfiles,
  ClearAllSkills,
  ClearAllChannels,
} from '@wailsjs/go/app/App';
import { useUIStore } from '../store/uiStore';
import { useChatStore } from '../store/chatStore';
import { Button } from '../components';
import { CollapsibleSection } from '../components/ui/CollapsibleSection';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useContentPageLandmarks } from '../hooks/useContentPageLandmarks';
import { useConfirm } from '../hooks/useConfirm';
import './RestoreDefaultsPage.css';

export default function RestoreDefaultsPage() {
  const { t } = useTranslation();
  const requestConfirm = useConfirm();
  const addToast = useUIStore((s) => s.addToast);
  const handleDatabaseReset = useChatStore((s) => s.handleDatabaseReset);
  const { announce } = useAnnouncer();
  useContentPageLandmarks({ pageClass: 'restore-defaults-page' });

  const [loadingOps, setLoadingOps] = useState<Set<string>>(new Set());
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set(['quick']));

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
    title: string,
    message: string,
    requiresDual: boolean,
    firstVariant: 'danger' | 'warning' | 'info',
    fn: () => Promise<void>,
    onSuccess?: () => void
  ) => {
    const confirmedFirst = await requestConfirm({
      title,
      message,
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: firstVariant,
    });
    if (!confirmedFirst) return;

    if (requiresDual) {
      const confirmedLast = await requestConfirm({
        title: t('restore.confirm.lastChanceTitle'),
        message: t('restore.confirm.lastChanceMessage'),
        confirmText: t('common.confirm'),
        cancelText: t('common.cancel'),
        variant: 'danger',
      });
      if (!confirmedLast) return;
    }

    setLoadingOps((prev) => new Set([...prev, opId]));
    try {
      await fn();
      addToast(`${opId} concluído com sucesso!`, 'success');
      announce(`${opId} realizado`);
      onSuccess?.();
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error ?? '');
      console.error(`Erro em ${opId}:`, error);
      addToast(message || `Erro ao executar ${opId}`, 'error');
    } finally {
      setLoadingOps((prev) => {
        const newSet = new Set(prev);
        newSet.delete(opId);
        return newSet;
      });
    }
  };

  const handleClearMessages = () =>
    void performReset(
      'Limpar Mensagens',
      t('restore.confirm.clearMessagesTitle'),
      t('restore.confirm.clearMessagesMessage'),
      true,
      'warning',
      async () => await ClearMessages()
    );

  const handleClearCredentials = () =>
    void performReset(
      'Limpar Credenciais',
      t('restore.confirm.clearCredentialsTitle'),
      t('restore.confirm.clearCredentialsMessage'),
      true,
      'warning',
      async () => await ClearAllCredentials()
    );

  const handleClearProfiles = () =>
    void performReset(
      'Limpar Perfis',
      t('restore.confirm.clearProfilesTitle'),
      t('restore.confirm.clearProfilesMessage'),
      true,
      'warning',
      async () => await ClearAllProfiles()
    );

  const handleClearSkills = () =>
    void performReset(
      'Limpar Skills',
      t('restore.confirm.clearSkillsTitle'),
      t('restore.confirm.clearSkillsMessage'),
      true,
      'warning',
      async () => await ClearAllSkills()
    );

  const handleClearChannels = () =>
    void performReset(
      'Limpar Canais',
      t('restore.confirm.clearChannelsTitle'),
      t('restore.confirm.clearChannelsMessage'),
      true,
      'warning',
      async () => await ClearAllChannels()
    );

  const handleResetDatabase = () =>
    void performReset(
      'Apagar Banco de Dados',
      t('restore.confirm.resetDatabaseTitle'),
      t('restore.confirm.resetDatabaseMessage'),
      true,
      'danger',
      async () => {
        await ResetDatabase();
        handleDatabaseReset();
      }
    );

  const handleClearAll = () =>
    void performReset(
      'Limpar Tudo',
      t('restore.confirm.clearAllTitle'),
      t('restore.confirm.clearAllMessage'),
      true,
      'danger',
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
