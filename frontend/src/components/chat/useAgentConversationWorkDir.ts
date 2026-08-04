import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  GetAgentConversationWorkDir,
  SetAgentConversationWorkDir,
} from '@wailsjs/go/app/App';
import { logger } from '../../utils/logger';

export interface AgentWorkDirState {
  conversationId: string;
  /** Diretório que vale para o próximo turno desta conversa. */
  dir: string;
  /** Diretório do app, que é o padrão de quem não escolheu. */
  workspaceDir: string;
  /** Verdadeiro quando esta conversa escolheu o diretório dela. */
  pinned: boolean;
  /** Diretório da sessão que o agente tem de pé, quando há uma. */
  sessionDir?: string;
}

export interface UseAgentConversationWorkDirResult {
  /** Estado atual, ou nulo quando não há agente de código nesta conversa. */
  state: AgentWorkDirState | null;
  /** Verdadeiro enquanto uma escolha está sendo gravada. */
  saving: boolean;
  /**
   * Grava o diretório desta conversa. Caminho vazio devolve a conversa ao
   * workspace ativo. Devolve o estado que valeu, ou lança o erro do backend —
   * caminho que não existe precisa chegar à tela como texto, e não sumir.
   */
  save: (dir: string) => Promise<AgentWorkDirState>;
}

/**
 * useAgentConversationWorkDir liga a barra da conversa ao diretório em que o
 * agente de código dela trabalha (AEP-0084 D5).
 *
 * O diretório é o alcance do que o agente pode ler e editar, e por isso é lido
 * do backend em vez de guardado na tela: o que vale é o que a montagem da
 * sessão vai usar, com caminho já resolvido, e não o texto que alguém digitou.
 */
export function useAgentConversationWorkDir(
  conversationId?: string | null,
): UseAgentConversationWorkDirResult {
  const [state, setState] = useState<AgentWorkDirState | null>(null);
  const [saving, setSaving] = useState(false);
  // conversationRef diz qual conversa está na tela agora: gravar é uma ida ao
  // backend, e a pessoa pode trocar de conversa antes da volta.
  const conversationRef = useRef(conversationId ?? '');

  useEffect(() => {
    conversationRef.current = conversationId ?? '';
    setSaving(false);
    if (!conversationId) {
      setState(null);
      return;
    }
    let current = true;
    GetAgentConversationWorkDir(conversationId)
      .then((next) => {
        if (!current) return;
        setState(next as AgentWorkDirState);
      })
      .catch((error: unknown) => {
        if (!current) return;
        // Conversa que não fala com agente de código é o caso comum, não uma
        // falha: o controle some em silêncio em vez de acusar erro numa
        // conversa que nunca teve diretório.
        setState(null);
        logger.warn('[AgentWorkDir] não foi possível ler o diretório da conversa:', error);
      });
    return () => {
      current = false;
    };
  }, [conversationId]);

  const save = useCallback(async (dir: string): Promise<AgentWorkDirState> => {
    const requested = conversationRef.current;
    if (!requested) throw new Error('conversa sem identificador');
    setSaving(true);
    try {
      const next = (await SetAgentConversationWorkDir(requested, dir)) as AgentWorkDirState;
      if (conversationRef.current === requested) setState(next);
      return next;
    } finally {
      if (conversationRef.current === requested) setSaving(false);
    }
  }, []);

  return useMemo(() => ({ state, saving, save }), [state, saving, save]);
}

/**
 * pendingRecreate diz que a escolha feita ainda não chegou ao agente: a sessão
 * de pé é de outro diretório e será recriada no próximo turno, custando a
 * memória da conversa.
 */
export function pendingRecreate(state: AgentWorkDirState | null): boolean {
  if (!state?.sessionDir) return false;
  return !sameDir(state.sessionDir, state.dir);
}

/**
 * sameDir compara dois caminhos do jeito que o sistema compara: no Windows a
 * caixa das letras não distingue diretórios, e tratá-la como diferença
 * anunciaria uma recriação que não vai acontecer.
 */
function sameDir(a: string, b: string): boolean {
  const normalize = (value: string) => value.replace(/[\\/]+$/, '');
  const left = normalize(a);
  const right = normalize(b);
  if (left === right) return true;
  return left.toLowerCase() === right.toLowerCase() && /^[a-zA-Z]:/.test(left);
}

/**
 * dirName é o nome curto que cabe num botão de barra: a última pasta do
 * caminho. O caminho inteiro continua sendo dito por extenso a quem usa leitor
 * de telas — é ele que descreve o alcance do agente, e a pasta sozinha não
 * distingue dois projetos com o mesmo nome.
 */
export function dirName(dir: string): string {
  const trimmed = dir.replace(/[\\/]+$/, '');
  if (!trimmed) return '';
  const parts = trimmed.split(/[\\/]/);
  return parts[parts.length - 1] || trimmed;
}
