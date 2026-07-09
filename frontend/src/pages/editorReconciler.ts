import type { DiskInfo } from '../lib/editorMergeUtils';

/**
 * Reconciliador único de mudanças externas do editor.
 *
 * Este módulo concentra, em uma função pura e testável
 * ({@link decideExternalChange}), a decisão que antes ficava duplicada em dois
 * caminhos paralelos:
 *
 * 1. pré-autosave (`persistTabContentNow`), que comparava metadados de disco
 *    (`diskInfoByTabRef` + `diskInfoEquals`); e
 * 2. o evento `editor:fileChanged` (`syncOrPromptExternalChangeForTab`), que
 *    comparava hash de conteúdo (`diskContentHashByTabRef`).
 *
 * Os dois pontos de entrada agora montam um {@link ReconcileInput} com o
 * estado consolidado da aba ({@link TabDiskState}) e executam a ação retornada.
 * A função não faz IO nem toca em estado: quem chama fornece as evidências e
 * aplica o efeito correspondente à decisão.
 */

/** Estado de disco consolidado por aba (metadados + baseline de conteúdo). */
export interface TabDiskState {
  /** Metadados (exists/isDir/size/mtime) da última leitura conhecida do disco. */
  info: DiskInfo | null;
  /** Hash FNV-1a do último conteúdo de disco conhecido (baseline). */
  baselineHash: number | null;
  /** Conteúdo do baseline (última versão conhecida do disco). */
  baselineContent: string | null;
}

/** Cria um estado de disco vazio para uma aba recém-registrada. */
export function createEmptyTabDiskState(): TabDiskState {
  return { info: null, baselineHash: null, baselineContent: null };
}

/** Ponto de entrada que originou a reconciliação. */
export type ReconcileTrigger =
  /** Evento `editor:fileChanged` do watcher (ou sync assistido explícito). */
  | 'file_changed'
  /** Checagem de metadados antes do autosave gravar no disco. */
  | 'pre_save'
  /** Re-checagem ao focar/mostrar a janela (sem auto-reload). */
  | 'focus_recheck';

/**
 * Evidências consolidadas para decidir o que fazer com uma possível mudança
 * externa em uma aba. Campos de disco são opcionais porque nem todo ponto de
 * entrada já leu o conteúdo (nesse caso a decisão pode ser `defer_read`).
 */
export interface ReconcileInput {
  trigger: ReconcileTrigger;

  /** Evento marcado pelo backend como escrita do próprio editor (`origin: 'editor_ui'` ou flag `selfWrite`). */
  selfWrite?: boolean;
  /** Evento marcado como escrita assistida (`origin: 'assistant_tool'` ou flag `assisted`). */
  assisted?: boolean;
  /**
   * Fallback defensivo (janela de tempo de `isProbablySelfWrite`) para eventos
   * SEM origin. Eventos com origin conhecido nunca devem setar este campo.
   */
  probablySelfWrite?: boolean;

  /** A aba está travada por conflito externo pendente. */
  conflictLocked: boolean;
  /** Já existe um questionário de resolução de conflito em voo para a aba. */
  promptInFlight: boolean;
  /** A aba está em uma sessão de merge (estilo Git) ativa. */
  hasMergeSession: boolean;
  /** A aba tem edições locais não salvas. */
  tabIsDirty: boolean;

  /** Metadados do disco divergem do último `TabDiskState.info` conhecido (usado no pré-autosave). */
  diskInfoChanged?: boolean;
  /** A leitura do conteúdo do disco falhou. */
  diskReadError?: boolean;
  /** Hash FNV-1a do conteúdo lido do disco (ausente quando ainda não lido). */
  diskHash?: number;
  /** Hash FNV-1a do conteúdo local atual (cache) da aba. */
  localHash?: number;
  /** Baseline de conteúdo conhecido do disco (`TabDiskState.baselineHash`), null se nunca visto. */
  lastKnownDiskHash?: number | null;

  /** Aba limpa (ou assistida convergente) pode recarregar do disco sem prompt. */
  allowAutoReload?: boolean;
}

/** Ação decidida pelo reconciliador. */
export type ReconcileAction =
  /** Nada a fazer para esta aba (motivo em `reason`). */
  | { action: 'ignore'; reason: 'merge_session' | 'locked' | 'self_write_window' | 'no_change' }
  /**
   * Atualizar o estado de disco conhecido sem mexer no conteúdo do editor.
   * - `info_only`: só metadados (selfWrite ou conteúdo do disco == baseline conhecido);
   * - `adopt_local`: disco convergiu com o local → baseline vira o conteúdo local e a aba fica limpa.
   */
  | { action: 'update_baseline'; scope: 'info_only' | 'adopt_local' }
  /** Evidência insuficiente: ler o conteúdo do disco e decidir novamente. */
  | { action: 'defer_read' }
  /** Aba pode acompanhar o disco silenciosamente (auto-reload com toast). */
  | { action: 'auto_reload' }
  /** Travar a aba e pedir decisão explícita (abrir prompt só se `openPrompt`). */
  | { action: 'prompt_conflict'; openPrompt: boolean };

/**
 * Decide, de forma pura, como reagir a uma possível mudança externa.
 *
 * A ordem dos guards preserva o comportamento histórico dos dois caminhos:
 * selfWrite vem antes do lock (o baseline de disco de abas travadas também era
 * atualizado em eventos selfWrite), e o fallback `probablySelfWrite` só vale
 * para eventos `file_changed` sem origin conhecido.
 */
export function decideExternalChange(input: ReconcileInput): ReconcileAction {
  // Escrita do próprio editor, marcada deterministicamente pelo backend:
  // basta acompanhar os metadados do disco — sem reload, sem prompt.
  if (input.selfWrite) {
    return { action: 'update_baseline', scope: 'info_only' };
  }

  // Reconciliação externa fica suspensa enquanto há merge/conflito pendente.
  if (input.hasMergeSession) return { action: 'ignore', reason: 'merge_session' };
  if (input.conflictLocked) return { action: 'ignore', reason: 'locked' };

  // Fallback defensivo para eventos SEM origin dentro da janela de self-write
  // (ex.: eventos duplicados do SO após o TTL da marcação no backend).
  if (input.trigger === 'file_changed' && !input.assisted && input.probablySelfWrite) {
    return { action: 'ignore', reason: 'self_write_window' };
  }

  // Falha ao ler o disco: só o usuário pode decidir o que fazer.
  if (input.diskReadError) {
    return { action: 'prompt_conflict', openPrompt: !input.promptInFlight };
  }

  // Sem conteúdo do disco em mãos: no pré-autosave a decisão é por metadados
  // (comportamento histórico); nos demais casos, ler o disco e decidir de novo.
  if (typeof input.diskHash !== 'number') {
    if (input.trigger === 'pre_save') {
      return input.diskInfoChanged
        ? { action: 'prompt_conflict', openPrompt: !input.promptInFlight }
        : { action: 'ignore', reason: 'no_change' };
    }
    return { action: 'defer_read' };
  }

  // Sem conflito real: disco e editor já convergiram para o mesmo conteúdo.
  if (typeof input.localHash === 'number' && input.diskHash === input.localHash) {
    return { action: 'update_baseline', scope: 'adopt_local' };
  }

  // Mudou o metadado, mas o conteúdo do disco continua sendo o baseline conhecido.
  const hasKnownBaseline = typeof input.lastKnownDiskHash === 'number';
  if (hasKnownBaseline && input.lastKnownDiskHash === input.diskHash) {
    return { action: 'update_baseline', scope: 'info_only' };
  }

  // Aba limpa (ou assistida cujo local ainda é o baseline conhecido) pode
  // acompanhar a escrita externa sem intervenção.
  const localMatchesKnownDisk =
    hasKnownBaseline && typeof input.localHash === 'number' && input.lastKnownDiskHash === input.localHash;
  if (input.allowAutoReload && (!input.tabIsDirty || (input.assisted && localMatchesKnownDisk))) {
    return { action: 'auto_reload' };
  }

  return { action: 'prompt_conflict', openPrompt: !input.promptInFlight };
}
