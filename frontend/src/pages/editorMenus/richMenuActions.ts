import type { RefObject } from 'react';

import type { EditorTab } from '../../store/editorStore';
import type { AddToastFn } from './types';

export type RichMenuActions = {
  /** True quando o editor rico pode ser usado com segurança nesse contexto. */
  canUseRich: boolean;
  /** Snapshot do editor rico atual (best-effort). */
  rich: unknown | null;
  /** Retorna o editor rico ou dispara toast informativo. */
  getRichOrToast: () => unknown | null;
  /** Executa uma ação best-effort com o editor rico. */
  run: (fn: (rich: any) => void) => boolean;
  /** Verifica se uma ação pode rodar (best-effort) com o editor rico. */
  canRun: (fn: (rich: any) => boolean) => boolean;
};

export function createRichMenuActions(args: {
  activeTab: EditorTab | null;
  isAsking: boolean;
  richEditorRef: RefObject<unknown>;
  /** Por padrão, só habilita ações quando a aba está em modo rico. */
  requireTabMode?: 'rich';
  addToast?: AddToastFn;
  notReadyToastMessage?: string;
}): RichMenuActions {
  const requireTabMode = args.requireTabMode ?? 'rich';

  const rich = (args.richEditorRef.current as any) ?? null;

  const canUseRich =
    !!args.activeTab &&
    !args.isAsking &&
    (requireTabMode ? args.activeTab.mode === requireTabMode : true) &&
    !!rich;

  const getRichOrToast = (): unknown | null => {
    if (canUseRich) return rich;
    const toast = args.addToast;
    if (toast) {
      toast(args.notReadyToastMessage ?? 'Editor rico ainda não está pronto.', 'info');
    }
    return null;
  };

  const run = (fn: (rich: any) => void): boolean => {
    if (!canUseRich || !rich) return false;
    try {
      fn(rich);
      return true;
    } catch {
      // best-effort
      return false;
    }
  };

  const canRun = (fn: (rich: any) => boolean) => {
    if (!canUseRich || !rich) return false;
    try {
      return !!fn(rich);
    } catch {
      return false;
    }
  };

  return { canUseRich, rich, getRichOrToast, run, canRun };
}
