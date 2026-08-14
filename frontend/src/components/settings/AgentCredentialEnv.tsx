import { useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ListCredentials } from '@wailsjs/go/wailsapi/Credentials';
import type { apidto } from '@wailsjs/go/models';
import { Button, FormField, Input, Select } from '../';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import './AgentCredentialEnv.css';

/** Uma entrada do cofre, como a lista de credenciais a descreve. */
interface VaultEntry {
  pattern: string;
  masked: string;
}

/**
 * Nome de variável de ambiente que o backend aceita. As mesmas recusas de
 * `validateCredentialEnv`, em `internal/llm/provider.go`: `=` parte o par que o
 * sistema operacional monta, e espaço em branco no nome não sobrevive à
 * passagem. Conferir aqui é o que transforma um erro de salvamento — que chega
 * depois, junto com tudo o mais que o formulário mandou — em uma frase ao lado
 * do campo que a causou.
 */
const nomeDeVariavelValido = (name: string): boolean =>
  name !== '' && !name.includes('=') && !/\s/.test(name) && !name.includes('\u0000');

/**
 * Qual entrada do cofre combina com o emissor que o agente nomeou. O emissor
 * vem como `openai`, e as entradas do cofre são padrões de host — `openai` casa
 * com `api.openai.com`. A comparação é por conter, e só serve para pré-escolher
 * algo no seletor: quem confirma é quem configura, e escolher sozinho mandaria
 * uma credencial para um agente sem ninguém ter dito que podia.
 */
const entradaSugerida = (entries: VaultEntry[], provider: string): string => {
  const alvo = provider.trim().toLowerCase();
  if (!alvo) return '';
  const achada = entries.find((entry) => entry.pattern.toLowerCase().includes(alvo));
  return achada?.pattern || '';
};

export interface AgentCredentialEnvProps {
  /**
   * Os pares já configurados: nome da variável de ambiente para padrão do
   * cofre. É referência, nunca segredo — o valor sai do cofre só na hora de
   * subir o agente (AEP-0086 D12).
   */
  value: Record<string, string>;
  onChange: (value: Record<string, string>) => void;
  /**
   * As variáveis que o próprio agente pediu no handshake, quando ele as nomeia.
   * Elas viram o preenchimento inicial do campo: quem escreveu o agente sabe
   * qual variável ele lê, e perguntar isso a quem configura é pedir que se
   * adivinhe o que já está publicado.
   */
  suggestedVars?: apidto.ACPAuthEnvVar[];
  /** O emissor da credencial que o agente pede, quando ele o nomeia. */
  suggestedProvider?: string;
}

/**
 * Liga a passagem de uma credencial do cofre ao agente por variável de ambiente
 * (AEP-0086 D12, Fase 8).
 *
 * O que o bloco guarda é referência: qual variável recebe qual entrada do
 * cofre. O valor não passa por aqui nem volta para a tela — ele é lido na hora
 * de subir o processo e redigido do log. É por isso que dá para editar o que
 * está ligado sem ter de reconfigurar o segredo.
 *
 * A tela diz o que ligar isso implica antes de ligar: o agente é um programa de
 * terceiro, e entregar uma chave a ele é dar a ele o que aquela chave abre.
 */
export const AgentCredentialEnv = ({
  value,
  onChange,
  suggestedVars,
  suggestedProvider,
}: AgentCredentialEnvProps) => {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const idBase = useId();
  const titleId = `${idBase}-title`;
  const warningId = `${idBase}-warning`;

  const [entries, setEntries] = useState<VaultEntry[]>([]);
  const [loading, setLoading] = useState(true);
  // Nulo é "o cofre respondeu"; string é a falha, com o motivo que veio dele
  // quando há um. O texto de recurso é escolhido no render, e não guardado
  // aqui: traduzir dentro do efeito o amarraria ao `t`, e uma função de
  // tradução que troca de identidade recarregaria a lista a cada render.
  const [vaultError, setVaultError] = useState<string | null>(null);
  const [novaVar, setNovaVar] = useState('');
  const [novoPadrao, setNovoPadrao] = useState('');
  const [erro, setErro] = useState('');

  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    void (async () => {
      try {
        const list = await ListCredentials();
        if (!mountedRef.current) return;
        setEntries((list || []).map((c) => ({ pattern: c.pattern, masked: c.masked || '' })));
        setVaultError(null);
      } catch (error: unknown) {
        if (!mountedRef.current) return;
        const err = error as { message?: unknown } | null;
        setVaultError(String(err?.message || error || ''));
      } finally {
        if (mountedRef.current) setLoading(false);
      }
    })();
  }, []);

  const pares = Object.entries(value).sort(([a], [b]) => a.localeCompare(b));

  // A primeira variável que o agente pede e que ainda não está ligada. Só
  // entram as que ele marcou como segredo: variável que não é segredo não tem
  // por que sair do cofre, e oferecê-la aqui mandaria uma URL de base pelo
  // caminho reservado a chaves.
  const sugestao =
    (suggestedVars || []).find((v) => v.secret && !(v.name in value))?.name || '';
  const padraoSugerido = entradaSugerida(entries, suggestedProvider || '');

  // O que o agente sugere entra no campo enquanto ninguém tiver escrito nada
  // ali. O ajuste acontece no render, como nos argumentos ao lado: por efeito,
  // haveria um quadro em que o teste do agente já respondeu e o campo ainda
  // aparece vazio. É o padrão que o React documenta para ajustar estado quando
  // uma prop muda, e a guarda é a marca do que já foi aplicado.
  const [aplicado, setAplicado] = useState({ nome: '', padrao: '' });
  if (sugestao !== aplicado.nome || padraoSugerido !== aplicado.padrao) {
    setAplicado({ nome: sugestao, padrao: padraoSugerido });
    if (sugestao && novaVar.trim() === '') setNovaVar(sugestao);
    if (padraoSugerido && novoPadrao === '') setNovoPadrao(padraoSugerido);
  }

  const handleAdd = () => {
    const nome = novaVar.trim();
    if (!nomeDeVariavelValido(nome)) {
      const message = t('providerForm.agent.credential.error.invalidName');
      setErro(message);
      announce(message, 'assertive');
      return;
    }
    if (!novoPadrao) {
      const message = t('providerForm.agent.credential.error.needsEntry');
      setErro(message);
      announce(message, 'assertive');
      return;
    }
    setErro('');
    onChange({ ...value, [nome]: novoPadrao });
    // Os campos voltam ao vazio para a próxima variável, e a sugestão já
    // aplicada não volta sozinha: ela descreve a variável que acabou de ser
    // ligada. A marca fica como está, e a próxima sugestão — se o agente pedir
    // mais de uma — é que preenche de novo.
    setNovaVar('');
    setNovoPadrao('');
    announce(t('providerForm.agent.credential.announce.added', { name: nome, entry: novoPadrao }), 'polite');
  };

  const handleRemove = (name: string) => {
    const next = { ...value };
    delete next[name];
    onChange(next);
    announce(t('providerForm.agent.credential.announce.removed', { name }), 'polite');
  };

  // Sem lista não há o que ligar, e o motivo muda o que dizer. Cofre que não
  // respondeu não vira seletor vazio ao lado de um botão que só sabe recusar:
  // a pessoa ficaria presa num campo obrigatório sem opção nenhuma. Cofre que
  // respondeu vazio é outra história — falta cadastrar, e é isso que se diz.
  const cofreIlegivel = vaultError !== null;
  const cofreVazio = !loading && !cofreIlegivel && entries.length === 0;

  const opcoes = [
    { value: '', label: t('providerForm.agent.credential.entryPlaceholder') },
    ...entries.map((entry) => ({
      value: entry.pattern,
      label: entry.masked
        ? t('providerForm.agent.credential.entryOption', { pattern: entry.pattern, masked: entry.masked })
        : entry.pattern,
    })),
  ];

  return (
    // O bloco inteiro é um grupo com nome: sem isso, quem navega por regiões
    // encontraria um campo de texto e um seletor soltos no meio do formulário
    // do agente, sem o que os liga. O anúncio de cada ação continua indo pelo
    // announcer global (AEP-0058) — uma região viva aqui repetiria tudo.
    <div
      className="agent-credential"
      role="group"
      aria-labelledby={titleId}
      aria-busy={loading || undefined}
    >
      <p id={titleId} className="agent-credential__title">
        {t('providerForm.agent.credential.title')}
      </p>

      {/*
        O que ligar isto implica, escrito antes de ligar: o agente é programa de
        terceiro, roda fora do app e recebe o valor inteiro. Quem decide isso
        precisa ler isso na hora de decidir, e não descobrir depois.
      */}
      <p id={warningId} className="agent-credential__warning">
        {t('providerForm.agent.credential.warning')}
      </p>

      {pares.length > 0 && (
        <ul className="agent-credential__list">
          {pares.map(([name, pattern]) => (
            <li key={name} className="agent-credential__item">
              <span className="agent-credential__pair">
                {t('providerForm.agent.credential.pair', { name, entry: pattern })}
              </span>
              <Button
                type="button"
                variant="ghost"
                onClick={() => handleRemove(name)}
                aria-label={t('providerForm.agent.credential.removeBtnLabel', { name })}
              >
                {t('providerForm.agent.credential.removeBtn')}
              </Button>
            </li>
          ))}
        </ul>
      )}

      {cofreIlegivel && (
        <p className="agent-credential__status" data-state="missing">
          {vaultError || t('providerForm.agent.credential.vaultFailed')}
        </p>
      )}

      {cofreVazio && (
        <p className="agent-credential__status" data-state="missing">
          {t('providerForm.agent.credential.emptyVault')}
        </p>
      )}

      {!cofreIlegivel && !cofreVazio && (
        <div className="agent-credential__add">
          <FormField
            label={t('providerForm.agent.credential.varLabel')}
            description={
              sugestao
                ? t('providerForm.agent.credential.varFromAgent')
                : t('providerForm.agent.credential.varHelp')
            }
            error={erro || undefined}
          >
            <Input
              value={novaVar}
              onChange={(e) => {
                setNovaVar(e.target.value);
                if (erro) setErro('');
              }}
              placeholder={t('providerForm.agent.credential.varPlaceholder')}
              fullWidth
            />
          </FormField>

          <FormField
            label={t('providerForm.agent.credential.entryLabel')}
            description={t('providerForm.agent.credential.entryHelp')}
          >
            <Select
              value={novoPadrao}
              onChange={(e) => {
                setNovoPadrao(e.target.value);
                if (erro) setErro('');
              }}
              options={opcoes}
              disabled={loading}
              fullWidth
            />
          </FormField>

          <Button
            type="button"
            variant="secondary"
            onClick={handleAdd}
            disabled={loading}
            aria-describedby={warningId}
          >
            {t('providerForm.agent.credential.addBtn')}
          </Button>
        </div>
      )}
    </div>
  );
};
