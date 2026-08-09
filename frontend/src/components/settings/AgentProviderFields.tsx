import { useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { DetectACPAgent, TestACPAgent } from '@wailsjs/go/app/App';
import type { app } from '@wailsjs/go/models';
import { Button, FormField, Input, Textarea } from '../';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { AgentCredentialEnv } from './AgentCredentialEnv';
import { AgentInstall } from './AgentInstall';
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

/**
 * Lê os argumentos digitados: uma linha por argumento porque caminho de arquivo
 * tem espaço, e separar por espaço partiria `C:\Program Files\...` em dois
 * argumentos que o agente não entenderia. Linha vazia não é argumento — mandá-la
 * viraria um `""` na linha de comando do agente.
 */
const lerArgumentos = (texto: string): string[] =>
  texto.split('\n').map((linha) => linha.trim()).filter(Boolean);

/**
 * Monta o comando de login a partir da configuração que está na tela, para o
 * agente cujo login é o mesmo CLI com outro subcomando: `acp` — que é o que sobe
 * o protocolo — sai e `login` entra. Quando o login é outro programa, quem diz
 * qual é ele é a detecção, que sabe de que agente se trata.
 *
 * Um `cursor-agent login` fixo mandaria a pessoa a um comando que pode não
 * existir: no Windows a detecção configura `node.exe ...\index.js acp`, e não há
 * `cursor-agent` no PATH (o CLI instala `cursor-agent.cmd` na pasta dele). Já
 * `node.exe ...\index.js login` é o login do mesmo agente.
 *
 * O `acp` nos argumentos é a premissa da troca. Sem ele o comando não é um CLI
 * com subcomando, e sim outro programa rodando um script — o caso do adaptador
 * npm do Claude Code, cujo login é o CLI `claude` e nunca um `...\index.js
 * login`. Sem premissa não se deriva nada, e quem sabe o comando passa a ser só
 * a detecção. Comando sem argumento nenhum continua valendo: é o CLI puro.
 */
export const agentLoginCommand = (command: string, args: string[]): string => {
  const executavel = command.trim();
  if (!executavel) return '';
  const argumentos = args.map((arg) => arg.trim()).filter(Boolean);
  if (argumentos.length > 0 && !argumentos.includes('acp')) return '';
  const partes = [executavel, ...argumentos.filter((arg) => arg !== 'acp'), 'login'];
  // Caminho com espaço precisa de aspas para quem for copiar a linha para o
  // terminal — é o caso comum no Windows (`C:\Program Files\...`).
  return partes.map((parte) => (/\s/.test(parte) ? `"${parte}"` : parte)).join(' ');
};

export interface AgentProviderFieldsProps {
  /**
   * O agente deste provedor, nomeado pelo `id` do registro ACP (ex.: `cursor`,
   * `claude-acp`). Vazio é agente apontado à mão, sem linha no catálogo: os
   * campos de comando continuam valendo, e o que depende de saber qual agente é
   * — procurar no disco, instalar pelo catálogo — não tem o que oferecer.
   */
  agentId: string;
  command: string;
  args: string[];
  onCommandChange: (command: string) => void;
  onArgsChange: (args: string[]) => void;
  commandError?: string;
  /**
   * Os pares de variável de ambiente e entrada do cofre que este agente recebe
   * ao subir (AEP-0086 D12). É referência, não segredo.
   */
  credentialEnv: Record<string, string>;
  onCredentialEnvChange: (value: Record<string, string>) => void;
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
  agentId,
  command,
  args,
  onCommandChange,
  onArgsChange,
  commandError,
  credentialEnv,
  onCredentialEnvChange,
  autoFill,
}: AgentProviderFieldsProps) => {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const idBase = useId();
  const detectHelpId = `${idBase}-detect-help`;
  const testHelpId = `${idBase}-test-help`;
  // O resultado da procura guarda de qual agente ele fala. Trocar o tipo do
  // provedor não desmonta estes campos, e a procura nova leva um tempo: sem a
  // marca, o que está na tela nesse intervalo descreve o agente anterior — e um
  // deles é o comando de login, que mandaria rodar o login do Claude Code para
  // autenticar o Cursor.
  const [detected, setDetected] = useState<{ kind: string; result: AgentSetup } | null>(null);
  const setup = detected?.kind === agentId ? detected.result : null;
  const [detecting, setDetecting] = useState(false);
  const [detectError, setDetectError] = useState('');
  const [testing, setTesting] = useState(false);
  // O resultado do teste guarda a configuração testada junto. Sem isso, mexer no
  // comando depois de testar deixaria na tela um "conectado" que se refere a
  // outro comando — e alguém salvaria confiando nele.
  const [tested, setTested] = useState<{ signature: string; health: AgentHealth | null; error: string } | null>(null);

  // Os campos atuais em refs: a detecção é assíncrona e precisa consultá-los no
  // instante em que a resposta chega, não no instante em que começou.
  const commandRef = useRef(command);
  commandRef.current = command;
  const argsRef = useRef(args);
  argsRef.current = args;

  // O texto dos argumentos é estado daqui, e não `args.join`, porque quem digita
  // precisa abrir a linha do próximo argumento com Enter. Como linha vazia não é
  // argumento, o valor derivado da lista apagava essa linha na tecla seguinte à
  // que a abriu, e só dava para configurar um argumento sem colar texto pronto.
  const [argsText, setArgsText] = useState(() => args.join('\n'));
  // Quem escreve de fora — a detecção, ou a volta ao provedor salvo — manda no
  // texto. Enquanto os dois lados descreverem os mesmos argumentos, o rascunho
  // fica como está, com as linhas em branco que a pessoa abriu.
  //
  // O ajuste acontece no render, e não num efeito. Pelo efeito, o campo ficava
  // um render atrás da lista: havia um instante em que o comando já estava
  // preenchido pela detecção e a caixa dos argumentos ainda aparecia vazia.
  //
  // Ajustar estado durante o render é o padrão que o React documenta para este
  // caso ("You Might Not Need an Effect", seção de ajustar estado quando uma
  // prop muda): vale porque o `set` é do próprio componente e está sob guarda,
  // e então o React refaz o render antes de pintar, sem efeito colateral
  // externo e sem o quadro intermediário que o efeito deixava aparecer.
  const [argsDeFora, setArgsDeFora] = useState(() => args.join('\n'));
  const deFora = args.join('\n');
  if (deFora !== argsDeFora) {
    setArgsDeFora(deFora);
    if (deFora !== lerArgumentos(argsText).join('\n')) {
      setArgsText(deFora);
    }
  }

  // Uma procura só vale se ainda é a última e se ainda há formulário de agente
  // para receber o que ela achou. Trocar o tipo desmonta estes campos, e uma
  // resposta que chegasse depois repovoaria comando e argumentos de um provedor
  // que agora é HTTP; trocar de agente deixa em voo a procura do anterior, que
  // descreve outra coisa.
  //
  // A montagem reafirma que há formulário vivo, e não só a desmontagem o
  // desmente: o `StrictMode` em que o app roda monta, desmonta e remonta cada
  // componente, e uma ref que só sabe apagar ficaria apagada no componente que
  // voltou. Aí toda resposta pareceria de um formulário que já não existe, e a
  // procura ficaria eternamente "procurando o agente...", com o botão
  // desabilitado e nada preenchido.
  const searchSeq = useRef(0);
  const probeSeq = useRef(0);
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

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
    const seq = ++searchSeq.current;
    const kind = agentId;
    const obsoleta = () => seq !== searchSeq.current || !mountedRef.current;
    if (!kind) {
      // Sem agente escolhido não há o que procurar, e uma chamada com nome
      // vazio só voltaria dizendo que o app não sabe procurar "".
      //
      // A procura que estava em voo já foi aposentada pela sequência acima, e
      // o `finally` dela não vai mais mexer em nada. Quem desliga a luz que
      // ela acendeu é esta chamada — sem isso a tela fica dizendo "procurando
      // agente" para sempre, sobre uma procura que ninguém mais espera.
      setDetected(null);
      setDetectError('');
      setDetecting(false);
      return;
    }
    setDetecting(true);
    setDetectError('');
    try {
      const result = await DetectACPAgent(kind);
      if (obsoleta()) return;
      setDetected({ kind, result });

      // As decisões de preencher são tomadas agora, com os valores atuais dos
      // campos, e não antes do await: quem começou a digitar enquanto a detecção
      // estava em voo perderia o que digitou. Comando e argumentos decidem
      // separado porque são campos separados — quem digitou só os argumentos
      // mantém os seus e ganha o comando que faltava.
      const pedida = applyCommand === 'always';
      const preencheComando = pedida || (applyCommand === 'ifEmpty' && commandRef.current.trim() === '');
      const preencheArgumentos = pedida || (applyCommand === 'ifEmpty' && argsRef.current.length === 0);

      if (result?.found) {
        if (preencheComando) {
          onCommandChange(result.command);
        }
        if (preencheArgumentos) {
          onArgsChange(result.args || []);
        }
        if (announceFound) {
          announce(t('providerForm.agent.announce.found', { command: result.command }), 'polite');
        }
        return;
      }
      // Não encontrado interrompe com anúncio assertivo exatamente quando é o
      // estado que exige ação: a procura pedida no botão e a automática que
      // deixaria o campo do comando vazio por falta de instalação — quem usa
      // leitor de telas não descobriria sozinho que ele veio vazio. Na edição de
      // um provedor que já tem comando, a procura é informativa: o comando salvo
      // é a escolha de quem configurou, e o alarme não descreveria problema
      // nenhum. O texto continua na tela nos dois casos.
      //
      // O app não saber procurar aquele agente não entra aqui: não há o que
      // resolver, e alarmar sobre isso a cada agente do catálogo faria o
      // anúncio deixar de significar alguma coisa.
      if (preencheComando && result?.detectable) {
        announce(t('providerForm.agent.announce.notFound'), 'assertive');
      }
    } catch (error: unknown) {
      if (obsoleta()) return;
      const err = error as { message?: unknown } | null;
      const message = String(err?.message || error || t('providerForm.agent.detectFailed'));
      setDetected(null);
      setDetectError(message);
      // A procura ter quebrado é anomalia em qualquer modo: mesmo com um comando
      // salvo, quem configura precisa saber que não deu para conferir a máquina.
      announce(message, 'assertive');
    } finally {
      if (!obsoleta()) {
        setDetecting(false);
      }
    }
  };

  useEffect(() => {
    // A detecção automática nunca substitui um comando que já existe: na edição
    // ele é o que está salvo, e na criação é o que a pessoa acabou de digitar.
    void detectRef.current?.({
      applyCommand: autoFill ? 'ifEmpty' : 'never',
      announceFound: false,
    });
  }, [agentId, autoFill]);

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

    // Resultado que não descreve mais o que está na tela não é dito nem
    // guardado: os campos seguem editáveis durante a sonda, e trocar o tipo
    // desmonta o formulário. A tela já escondia o resultado de outra
    // configuração pela assinatura, mas o anúncio saía de qualquer jeito, e quem
    // usa leitor de telas ouviria "conectado" sobre o comando anterior.
    //
    // O número da sonda entra pelo mesmo motivo que na detecção. Duas sondas em
    // voo não deveriam acontecer — o botão fica desabilitado enquanto uma roda —,
    // e é justamente por isso que a guarda é barata: se um dia acontecer, a
    // resposta velha não desliga o "testando..." da nova nem fala por ela.
    const seq = ++probeSeq.current;
    const outraConfiguracao = () =>
      !mountedRef.current ||
      seq !== probeSeq.current ||
      signature !== configSignature(commandRef.current, argsRef.current);

    setTesting(true);
    setTested(null);
    try {
      const result = await TestACPAgent(trimmed, args);
      if (outraConfiguracao()) return;
      setTested({ signature, health: result, error: '' });
      announce(healthAnnouncement(t, result), result.state === 'online' ? 'polite' : 'assertive');
    } catch (error: unknown) {
      if (outraConfiguracao()) return;
      const err = error as { message?: unknown } | null;
      const message = String(err?.message || error || t('providerForm.agent.test.failed'));
      setTested({ signature, health: null, error: message });
      announce(message, 'assertive');
    } finally {
      if (mountedRef.current && seq === probeSeq.current) {
        setTesting(false);
      }
    }
  };

  // Procurar é coisa de agente que o app sabe procurar. Para os outros a tela
  // diz isso, em vez de oferecer um botão cuja única resposta possível já é
  // conhecida — e em vez de chamar de "não encontrado" uma procura que nunca
  // aconteceu (AEP-0086 D1).
  //
  // Sem resposta ainda, o botão fica: some só quem já se soube que o app não
  // procura. Enquanto a primeira procura corre ele é o rótulo "procurando", e
  // se ela falhar ele é a única forma de tentar de novo sem sair da tela — foi
  // por escondê-lo nesse caso que a falha virava beco sem saída.
  //
  // Sem agente escolhido não há nem pergunta: é o estado de quem acabou de
  // escolher o tipo e ainda não abriu o catálogo, e ali um botão de procurar
  // procuraria o quê.
  const detectable = agentId !== '' && (setup ? setup.detectable : true);

  const status = (() => {
    if (detecting) return t('providerForm.agent.detecting');
    if (detectError) return detectError;
    if (!setup) return '';
    if (!setup.detectable) return t('providerForm.agent.noDetection');
    if (setup.found) {
      return setup.version
        ? t('providerForm.agent.foundVersion', { source: setup.source, version: setup.version })
        : t('providerForm.agent.found', { source: setup.source });
    }
    return t('providerForm.agent.notFound');
  })();

  // "Não encontrado" é uma resposta, e só existe depois de haver uma: sem ela,
  // o bloco que manda instalar o CLI apareceria no instante entre escolher o
  // agente e a procura começar, dizendo que faltou o que ninguém procurou.
  const notFound = !!setup?.detectable && !setup.found && !detecting;

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
  // O que o agente escreveu sobre o login, com o nome do método na frente para
  // separar um do outro quando há mais de um. Nem todo agente publica comando,
  // e vários explicam em texto — o `opencode auth login` chega assim.
  const loginNotes = (health?.login_methods ?? [])
    .filter((method) => method.description)
    .map((method) => ({
      id: method.id,
      // A junção do nome com a descrição é uma frase, e frase é do locale: a
      // pontuação entre as duas partes muda de idioma para idioma, e deixá-la
      // no código a congelaria em português.
      text: method.name
        ? t('providerForm.agent.test.loginNote', {
            name: method.name,
            description: method.description,
          })
        : (method.description as string),
    }));
  // A ordem é do mais informado para o mais adivinhado: primeiro o que o
  // próprio agente publicou no handshake, que é quem sabe como se autentica
  // nele; depois o que a procura conhece daquele agente; e o palpite por
  // último, que só acerta em quem sobe o ACP por subcomando.
  //
  // O palpite sai de cena quando o agente explicou o login em texto: ali as
  // duas instruções apareceriam juntas, e a que tem cara de comando pronto —
  // um `opencode login` que não existe — é justamente a errada.
  const loginCommand =
    health?.login_command ||
    setup?.login_command ||
    (loginNotes.length > 0 ? '' : agentLoginCommand(command, args));

  // As variáveis que o agente pediu no handshake, e o emissor da credencial que
  // ele nomeou. É o que o bloco do cofre oferece preenchido, em vez de pedir que
  // se adivinhe qual variável ele lê (AEP-0086 D12).
  const suggestedVars = (health?.login_methods ?? []).flatMap((method) => method.env_vars ?? []);
  const suggestedProvider =
    (health?.login_methods ?? []).find((method) => method.credential_provider)?.credential_provider || '';

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
          value={argsText}
          onChange={(e) => {
            setArgsText(e.target.value);
            onArgsChange(lerArgumentos(e.target.value));
          }}
          rows={3}
          fullWidth
        />
      </FormField>

      {/*
        Cada botão vem com a descrição do que o clique faz, ligada por
        `aria-describedby` e visível ao lado dele. Lado a lado e sem texto, os
        dois pareciam duas formas de conferir a instalação, e só um deles
        sobrescreve o comando e os argumentos que estão na tela — quem descobre
        isso clicando descobre depois de perder o que havia digitado.
      */}
      <div className="agent-fields__actions">
        {detectable && (
          <div className="agent-fields__action">
            <Button
              type="button"
              variant="secondary"
              onClick={handleRedetect}
              disabled={detecting}
              aria-describedby={detectHelpId}
            >
              {detecting ? t('providerForm.agent.detecting') : t('providerForm.agent.detectBtn')}
            </Button>
            <p id={detectHelpId} className="agent-fields__action-help">
              {t('providerForm.agent.detectBtnHelp')}
            </p>
          </div>
        )}
        <div className="agent-fields__action">
          <Button
            type="button"
            variant="secondary"
            onClick={handleTest}
            disabled={testing}
            aria-describedby={testHelpId}
          >
            {testing ? t('providerForm.agent.test.testing') : t('providerForm.agent.test.btn')}
          </Button>
          <p id={testHelpId} className="agent-fields__action-help">
            {t('providerForm.agent.test.btnHelp')}
          </p>
        </div>
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
          {/*
            O agente vem primeiro quando ele informa o comando, porque quem o
            escreveu sabe como se autentica nele — e o palpite do app erra
            justamente em quem não sobe o ACP por subcomando. Depois vem a
            procura: no Claude Code o que sobe o ACP é um adaptador npm sem
            login nenhum, e derivar dali mandaria a pessoa a um comando que não
            existe. Quando ninguém sabe dizer o comando, a tela pede a procura
            em vez de chutar um: comando errado aqui é a pessoa indo ao terminal
            para ver um "not found".
          */}
          <p>
            {health.login_command
              ? t('providerForm.agent.test.loginFromAgent')
              : loginCommand
                ? t('providerForm.agent.test.loginHelp')
                : loginNotes.length > 0
                  ? t('providerForm.agent.test.loginDescribed')
                  : t('providerForm.agent.test.loginUnknown')}
          </p>
          {!!loginCommand && <code className="agent-fields__login-command">{loginCommand}</code>}
          {!!health.login_methods?.length && (
            <p className="agent-fields__login-methods">
              {t('providerForm.agent.test.loginMethods', {
                methods: health.login_methods.map((method) => method.name || method.id).join(', '),
              })}
            </p>
          )}
          {/*
            O que o agente escreveu sobre o login, palavra por palavra. Vale
            para quem explica o comando em texto em vez de publicá-lo, que é a
            maioria: resumir isso em um rótulo jogaria fora a única instrução
            que existe.
          */}
          {loginNotes.length > 0 && (
            <ul className="agent-fields__login-notes">
              {loginNotes.map((note) => (
                <li key={note.id}>{note.text}</li>
              ))}
            </ul>
          )}
        </div>
      )}

      {/*
        A passagem de credencial fica logo depois do diagnóstico porque é ali
        que ela se decide: o agente que responde pedindo autenticação é
        exatamente o que pode ser autenticado por variável, e é dali que sai o
        nome que o bloco oferece preenchido. Ele aparece sempre, e não só nesse
        estado, porque também é onde a passagem já configurada se mostra e se
        desfaz.
      */}
      <AgentCredentialEnv
        value={credentialEnv}
        onChange={onCredentialEnvChange}
        suggestedVars={suggestedVars}
        suggestedProvider={suggestedProvider}
      />

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

      {/*
        Instalar pelo catálogo é a alternativa a mandar a pessoa ao terminal
        (AEP-0086 Fase 3). O bloco fica aqui, e não só no estado "não
        encontrado", porque ele também é onde a instalação feita pelo app se
        mostra e se desfaz — e isso vale mesmo quando o agente está na máquina.
      */}
      <AgentInstall
        agentId={agentId}
        onResolved={(installedCommand, installedArgs) => {
          onCommandChange(installedCommand);
          onArgsChange(installedArgs);
        }}
      />

      {/*
        O diretório é informação lida, e não campo a preencher: como `Input`
        somente-leitura ele convidava a digitar e ocupava uma parada de Tab que
        não fazia nada. O par `dt`/`dd` mantém rótulo e valor ligados para quem
        usa leitor de telas, sem prometer edição que não existe.
      */}
      <div className="agent-fields__workdir">
        <dl className="agent-fields__workdir-pair">
          <dt className="agent-fields__workdir-term">{t('providerForm.agent.workDir')}</dt>
          <dd
            className="agent-fields__workdir-value"
            data-empty={setup?.work_dir ? undefined : 'true'}
          >
            {setup?.work_dir || t('providerForm.agent.workDirUnknown')}
          </dd>
        </dl>
        <p className="agent-fields__workdir-help">{t('providerForm.agent.workDirHelp')}</p>
      </div>
    </div>
  );
};
