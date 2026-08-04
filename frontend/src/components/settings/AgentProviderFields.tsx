import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DetectACPAgent } from '@wailsjs/go/app/App';
import type { app } from '@wailsjs/go/models';
import { Button, FormField, Input, Textarea } from '../';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './AgentProviderFields.css';

/** O que a detecção do backend devolve sobre o agente instalado. */
type AgentSetup = app.ACPAgentSetup;

export interface AgentProviderFieldsProps {
  /** Tipo do agente procurado na máquina (ex.: `cursor`). */
  agentKind: string;
  command: string;
  args: string[];
  onCommandChange: (command: string) => void;
  onArgsChange: (args: string[]) => void;
  commandError?: string;
  /**
   * Deixa a detecção preencher o comando sozinha. Vale na criação; na edição o
   * comando salvo é a escolha de quem configurou, e sobrescrevê-lo ao abrir a
   * tela desfaria um ajuste manual sem ninguém pedir.
   */
  autoFill: boolean;
}

/**
 * Campos de um provedor que é um agente de código local (AEP-0084 Fase 3):
 * comando, argumentos e o diretório sobre o qual o agente age.
 *
 * O diretório é leitura, não escolha: nesta fase ele é o workspace ativo do app
 * (D5). Ele aparece porque é onde o agente edita arquivos — esconder isso
 * esconderia o alcance do que a pessoa está autorizando.
 */
export const AgentProviderFields = ({
  agentKind,
  command,
  args,
  onCommandChange,
  onArgsChange,
  commandError,
  autoFill,
}: AgentProviderFieldsProps) => {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [setup, setSetup] = useState<AgentSetup | null>(null);
  const [detecting, setDetecting] = useState(false);
  const [detectError, setDetectError] = useState('');

  // A detecção fica em uma ref, e não em useCallback, porque ela usa o comando
  // digitado e os callbacks do pai. Como dependência de efeito, qualquer um
  // deles disparia uma detecção nova a cada tecla ou a cada render do pai.
  const detectRef = useRef<(options: { applyCommand: boolean; announceFound: boolean }) => Promise<void>>();
  detectRef.current = async ({ applyCommand, announceFound }) => {
    setDetecting(true);
    setDetectError('');
    try {
      const result = await DetectACPAgent(agentKind);
      setSetup(result);
      if (result?.found) {
        if (applyCommand) {
          onCommandChange(result.command);
          onArgsChange(result.args || []);
        }
        if (announceFound) {
          announce(t('providerForm.agent.announce.found', { command: result.command }), 'polite');
        }
        return;
      }
      // Não encontrado é sempre anunciado, mesmo na detecção automática: é o
      // estado que exige ação, e quem usa leitor de telas não descobriria
      // sozinho que o campo veio vazio por falta de instalação.
      announce(t('providerForm.agent.announce.notFound'), 'assertive');
    } catch (error: unknown) {
      const err = error as { message?: unknown } | null;
      const message = String(err?.message || error || t('providerForm.agent.detectFailed'));
      setSetup(null);
      setDetectError(message);
      announce(message, 'assertive');
    } finally {
      setDetecting(false);
    }
  };

  const commandRef = useRef(command);
  commandRef.current = command;

  useEffect(() => {
    // A detecção automática nunca substitui um comando que já existe: na edição
    // ele é o que está salvo, e na criação é o que a pessoa acabou de digitar.
    void detectRef.current?.({
      applyCommand: autoFill && commandRef.current.trim() === '',
      announceFound: false,
    });
  }, [agentKind, autoFill]);

  const handleRedetect = () => {
    // Clique explícito aplica o que achou: é justamente para isso que alguém
    // pede a detecção de novo depois de o CLI se atualizar e mudar de caminho.
    void detectRef.current?.({ applyCommand: true, announceFound: true });
  };

  const status = (() => {
    if (detecting) return t('providerForm.agent.detecting');
    if (detectError) return detectError;
    if (!setup) return '';
    if (setup.found) {
      return setup.version
        ? t('providerForm.agent.foundVersion', { source: setup.source, version: setup.version })
        : t('providerForm.agent.found', { source: setup.source });
    }
    return t('providerForm.agent.notFound');
  })();

  const notFound = !!setup && !setup.found && !detecting;

  return (
    <div className="agent-fields">
      <FormField
        label={t('providerForm.agent.command')}
        required
        error={commandError}
        description={t('providerForm.agent.commandHelp')}
      >
        <Input
          value={command}
          onChange={(e) => onCommandChange(e.target.value)}
          placeholder={t('providerForm.agent.commandPlaceholder')}
          fullWidth
        />
      </FormField>

      <FormField
        label={t('providerForm.agent.args')}
        description={t('providerForm.agent.argsHelp')}
      >
        <Textarea
          value={args.join('\n')}
          onChange={(e) =>
            // Uma linha por argumento porque caminho de arquivo tem espaço:
            // separar por espaço partiria `C:\Program Files\...` em dois
            // argumentos que o agente não entenderia.
            onArgsChange(e.target.value.split('\n').map((line) => line.trim()).filter(Boolean))
          }
          rows={3}
          fullWidth
        />
      </FormField>

      <div className="agent-fields__detection">
        <Button type="button" variant="secondary" onClick={handleRedetect} disabled={detecting}>
          {detecting ? t('providerForm.agent.detecting') : t('providerForm.agent.detectBtn')}
        </Button>
        {status && (
          <p className="agent-fields__status" data-state={notFound || detectError ? 'missing' : 'ok'}>
            {status}
          </p>
        )}
      </div>

      {notFound && (
        <div className="agent-fields__install">
          <p>{t('providerForm.agent.installHelp')}</p>
          {!!setup?.searched?.length && (
            <p className="agent-fields__searched">
              {t('providerForm.agent.searchedIn', { places: setup.searched.join(', ') })}
            </p>
          )}
        </div>
      )}

      <FormField
        label={t('providerForm.agent.workDir')}
        description={t('providerForm.agent.workDirHelp')}
      >
        <Input
          value={setup?.work_dir || ''}
          readOnly
          fullWidth
          placeholder={t('providerForm.agent.workDirUnknown')}
        />
      </FormField>
    </div>
  );
};
