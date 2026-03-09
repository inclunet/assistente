import { useState } from 'react';
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
import './RestoreDefaultsPage.css';

export default function RestoreDefaultsPage() {
  const { addToast } = useUIStore();
  const { handleDatabaseReset } = useChatStore();
  const { announce } = useAnnouncer();

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
        <h1>Restaurar Padrões</h1>
        <p>Gerencie a restauração e limpeza de dados do assistente</p>
      </header>

      <main className="restore-content">
        {/* Quick Actions - Operações Rápidas */}
        <CollapsibleSection
          title="⚡ Operações Rápidas"
          isOpen={isSectionOpen('quick')}
          onToggle={() => toggleSection('quick')}
          ariaLabel="Operações Rápidas - limpar mensagens e conversas"
        >
          <div className="restore-item">
            <div className="restore-item-info">
              <h3>🗑️ Limpar Mensagens e Conversas</h3>
              <p>Apaga todas as mensagens e conversas, mantendo perfis e credenciais</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearMessages}
              loading={isLoading('Limpar Mensagens')}
            >
              Limpar
            </Button>
          </div>
        </CollapsibleSection>

        {/* Granular Cleanup - Limpeza Granular */}
        <CollapsibleSection
          title="🎛️ Limpeza Granular"
          isOpen={isSectionOpen('granular')}
          onToggle={() => toggleSection('granular')}
          ariaLabel="Limpeza Granular - limpar credenciais, perfis, skills e canais"
        >
          <div className="restore-item">
            <div className="restore-item-info">
              <h3>🔑 Limpar Todas as Credenciais</h3>
              <p>Remove todas as chaves de API e credenciais armazenadas. Você precisará reconfigurar os provedores.</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearCredentials}
              loading={isLoading('Limpar Credenciais')}
            >
              Limpar
            </Button>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>👤 Limpar Todos os Perfis</h3>
              <p>Remove todos os perfis de chat customizados. Os padrões podem ser recriados.</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearProfiles}
              loading={isLoading('Limpar Perfis')}
            >
              Limpar
            </Button>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>🛠️ Limpar Todos os Skills</h3>
              <p>Remove todos os skills customizados. Skills built-in podem ser reconfigurados.</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearSkills}
              loading={isLoading('Limpar Skills')}
            >
              Limpar
            </Button>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>📱 Limpar Todos os Canais</h3>
              <p>Remove configurações de todos os canais (Telegram, Slack, Signal, etc).</p>
            </div>
            <Button
              variant="outline"
              onClick={handleClearChannels}
              loading={isLoading('Limpar Canais')}
            >
              Limpar
            </Button>
          </div>
        </CollapsibleSection>

        {/* Nuclear Options - Opções Nucleares */}
        <CollapsibleSection
          title="💣 Opções Nucleares"
          isOpen={isSectionOpen('nuclear')}
          onToggle={() => toggleSection('nuclear')}
          ariaLabel="Opções Nucleares - operações irreversíveis"
        >
          <div className="restore-item restore-item-danger">
            <div className="restore-item-info">
              <h3>🔥 Apagar Banco de Dados Inteiro</h3>
              <p>Remove PERMANENTEMENTE todo o banco de dados incluindo conversas, abas e históricos. Perfis e credenciais podem ser preservados.</p>
            </div>
            <Button
              variant="danger"
              onClick={handleResetDatabase}
              loading={isLoading('Apagar Banco de Dados')}
            >
              Apagar
            </Button>
          </div>

          <div className="restore-item restore-item-danger">
            <div className="restore-item-info">
              <h3>🚨 LIMPAR TUDO (Nuclear)</h3>
              <p>Remove PERMANENTEMENTE TUDO. Banco de dados, credenciais, perfis, skills, canais. O assistente voltará ao estado inicial de instalação.</p>
            </div>
            <Button
              variant="danger"
              onClick={handleClearAll}
              loading={isLoading('Limpar Tudo')}
            >
              LIMPAR TUDO
            </Button>
          </div>
        </CollapsibleSection>

        {/* Master Password Management - Gerenciamento de Senha Mestre */}
        <CollapsibleSection
          title="🔐 Segurança - Senha Mestre"
          isOpen={isSectionOpen('security')}
          onToggle={() => toggleSection('security')}
          ariaLabel="Segurança - Gerenciamento de senha mestre"
        >
          <div className="security-info">
            <p>
              A senha mestre protege todas as suas credenciais criptografadas. Se você esquecer a senha, use o código de recuperação para redefini-la.
            </p>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>🔑 Redefinir Senha Mestre</h3>
              <p>Define uma nova senha mestre. Você precisará fornecer a senha atual ou código de recuperação.</p>
            </div>
            <Button variant="outline" disabled>
              Redefinir (em breve)
            </Button>
          </div>

          <div className="restore-item">
            <div className="restore-item-info">
              <h3>📋 Recuperar Código de Recuperação</h3>
              <p>Exibe o código de recuperação em caso de perda da senha mestre. Guarde em local seguro!</p>
            </div>
            <Button variant="outline" disabled>
              Ver Código (em breve)
            </Button>
          </div>

          <div className="restore-item restore-item-warning">
            <div className="restore-item-info">
              <h3>🚫 Remover Senha Mestre</h3>
              <p>Remove a proteção de senha mestre do computador. ⚠️ ATENÇÃO: As credenciais NÃO poderão ser descriptografadas ou usadas. Isso impede que o assistente use qualquer API key armazenada.</p>
            </div>
            <Button variant="outline" disabled>
              Remover (em breve)
            </Button>
          </div>
        </CollapsibleSection>
      </main>
    </div>
  );
}
