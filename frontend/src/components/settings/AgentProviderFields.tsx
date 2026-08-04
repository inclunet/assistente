import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { DetectACPAgent, TestACPAgent } from '@wailsjs/go/app/App';
import type { app } from '@wailsjs/go/models';
import { Button, FormField, Input, Textarea } from '../';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './AgentProviderFields.css';

/** O que a detecção do backend devolve sobre o agente instalado. */
type AgentSetup = app.ACPAgentSetup;

/** O que a sondagem do backend devolve sobre o agente configurado. */
type AgentHealth = app.ACPAgentHealth;

/** Identifica a configuração testada, para o resultado não sobreviver a ela. */
const configSignature = (command: string, args: string[]): string => `${command.trim()}\u0000${args.join('\n')}`;

/**
 * Frase que descreve o resultado do teste. É a mesma para a tela e para o
 * anúncio: o estado precisa chegar por texto a quem usa leitor de telas, e um
 * texto só para o anúncio divergiria do que está escrito na tela.
 */
const healthAnnouncement = (t: TFunction, health: AgentHealth): string => {
  if (health.state === 'online') {
    return health.agent_name
      ? t('providerForm.agent.test.onlineNamed', { agent: health.agent_name })
      : t('providerForm.agent.test.online');
  }
  if (health.state === 'unauthenticated') {
    return t('providerForm.agent.test.unauthenticated');
  }
  return health.error
    ? t('providerForm.agent.test.offlineDetail', { detail: health.error })
    : t('providerForm.agent.test.offline');
};

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
  const [testing, setTesting] = useState(false);
  // O resultado do teste guarda a configuração testada junto. Sem isso, mexer no
  // comando depois de testar deixaria na tela um "conectado" que se refere a
  // outro comando — e alguém salvaria confiando nele.
  const [tested, setTested] = useState<{ signature: string; health: AgentHealth | null; error: string } | null>(null);

  // O comando atual em uma ref: a detecção é assíncrona e precisa consultar o
  // campo no instante em que a resposta chega, não no instante em que começou.
  const commandRef = useRef(command);
  commandRef.current = command;

  // A detecção fica em uma ref, e não em useCallback, porque ela usa o comando
  // digitado e os callbacks do pai. Como dependência de efeito, qualquer um
  // deles disparia uma detecção nova a cada tecla ou a cada render do pai.
  //
  // `applyCommand` distingue a detecção pedida da automática: `always` é o
  // clique no botão, que existe justamente para sobrescrever; `ifEmpty` é a
  // automática, que só preenche campo vazio; `never` é a da edição, que apenas
  // informa o que há na máquina.
  const detectRef =
    useRef<(options: { applyCommand: 'always' | 'ifEmpty' | 'never'; announceFound: boolean }) => Promise<void>>();
  detectRef.current = async ({ applyCommand, announceFound }) => {
    setDetecting(true);
    setDetectError('');
    try {
      const result = await DetectACPAgent(agentKind);
      setSetup(result);
      if (result?.found) {
        // A decisão de preencher é tomada agora, com o valor atual do campo, e
        // não antes do await: quem começou a digitar o comando enquanto a
        // detecção automática estava em voo perderia o que digitou.
        const apply = applyCommand === 'always' || (applyCommand === 'ifEmpty' && commandRef.current.trim() === '');
        if (apply) {
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

  useEffect(() => {
    // A detecção automática nunca substitui um comando que já existe: na edição
    // ele é o que está salvo, e na criação é o que a pessoa acabou de digitar.
    void detectRef.current?.({
      applyCommand: autoFill ? 'ifEmpty' : 'never',
      announceFound: false,
    });
  }, [agentKind, autoFill]);

  const handleRedetect = () => {
    // Clique explícito aplica o que achou: é justamente para isso que alguém
    // pede a detecção de novo depois de o CLI se atualizar e mudar de caminho.
    void detectRef.current?.({ applyCommand: 'always', announceFound: true });
  };

  const handleTest = async () => {
    const trimmed = command.trim();
    const signature = configSignature(command, args);
    if (!trimmed) {
      const message = t('providerForm.agent.test.needsCommand');
      setTested({ signature, health: null, error: message });
      announce(message, 'assertive');
      return;
    }

    setTesting(true);
    setTested(null);
    try {
      const result = await TestACPAgent(trimmed, args);
      setTested({ signature, health: result, error: '' });
      announce(healthAnnouncement(t, result), result.state === 'online' ? 'polite' : 'assertive');
    } catch (error: unknown) {
      const err = error as { message?: unknown } | null;
      const message = String(err?.message || error || t('providerForm.agent.test.failed'));
      setTested({ signature, health: null, error: message });
      announce(message, 'assertive');
    } finally {
      setTesting(false);
    }
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

  // O resultado só vale para a configuração que foi testada.
  const current = tested?.signature === configSignature(command, args) ? tested : null;
  const health = current?.health ?? null;
  const result = (() => {
    if (testing) return t('providerForm.agent.test.testing');
    if (!current) return '';
    if (current.error) return current.error;
    return health ? healthAnnouncement(t, health) : '';
  })();
  const resultState = health?.state === 'online' ? 'ok' : 'missing';

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
        <Button type="button" variant="secondary" onClick={handleTest} disabled={testing}>
          {testing ? t('providerForm.agent.test.testing') : t('providerForm.agent.test.btn')}
        </Button>
        {status && (
          <p className="agent-fields__status" data-state={notFound || detectError ? 'missing' : 'ok'}>
            {status}
          </p>
        )}
      </div>

      {/*
        Resultado do teste em texto, com o estado no data-state apenas como
        reforço visual. Não é live region: o anúncio vai pelo announcer global.
      */}
      {result && (
        <p className="agent-fields__health" data-state={resultState}>
          {result}
        </p>
      )}

      {/*
        Sem login não é erro de configuração: o comando está certo e o agente
        respondeu. O que falta se resolve no terminal, então a tela mostra o
        comando a rodar em vez de mandar conferir caminho.
      */}
      {health?.state === 'unauthenticated' && (
        <div className="agent-fields__login">
          <p>{t('providerForm.agent.test.loginHelp')}</p>
          <code className="agent-fields__login-command">{t('providerForm.agent.test.loginCommand')}</code>
          {!!health.login_methods?.length && (
            <p className="agent-fields__login-methods">
              {t('providerForm.agent.test.loginMethods', {
                methods: health.login_methods.map((method) => method.name || method.id).join(', '),
              })}
            </p>
          )}
        </div>
      )}

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
