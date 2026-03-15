import type { RefObject } from 'react';

import type { EditorTab } from '../../store/editorStore';
import type { AddToastFn } from './types';

type RichEditorChain = {
  chain?: () => RichEditorChain;
  focus?: () => RichEditorChain;
  run?: () => boolean;
  setParagraph?: () => RichEditorChain;
  setHeading?: (args: { level: number }) => RichEditorChain;
  toggleBold?: () => RichEditorChain;
  toggleItalic?: () => RichEditorChain;
  toggleStrike?: () => RichEditorChain;
  extendMarkRange?: (name: string) => RichEditorChain;
  unsetLink?: () => RichEditorChain;
  unsetAllMarks?: () => RichEditorChain;
  toggleBlockquote?: () => RichEditorChain;
  toggleCodeBlock?: () => RichEditorChain;
  toggleBulletList?: () => RichEditorChain;
  toggleOrderedList?: () => RichEditorChain;
  insertTable?: (args: { rows: number; cols: number; withHeaderRow: boolean }) => RichEditorChain;
  setCodeBlock?: (args: { language: string }) => RichEditorChain;
  insertContent?: (content: string) => RichEditorChain;
  addRowBefore?: () => RichEditorChain;
  addRowAfter?: () => RichEditorChain;
  deleteRow?: () => RichEditorChain;
  addColumnBefore?: () => RichEditorChain;
  addColumnAfter?: () => RichEditorChain;
  deleteColumn?: () => RichEditorChain;
  toggleHeaderRow?: () => RichEditorChain;
  toggleHeaderColumn?: () => RichEditorChain;
  toggleHeaderCell?: () => RichEditorChain;
  mergeCells?: () => RichEditorChain;
  splitCell?: () => RichEditorChain;
  goToPreviousCell?: () => RichEditorChain;
  goToNextCell?: () => RichEditorChain;
  deleteTable?: () => RichEditorChain;
};

export type RichEditorLike = {
  chain?: () => RichEditorChain;
  can?: () => RichEditorChain;
  isActive?: (name: string) => boolean;
};

export type RichMenuActions = {
  /** True quando o editor rico pode ser usado com segurança nesse contexto. */
  canUseRich: boolean;
  /** Snapshot do editor rico atual (best-effort). */
  rich: RichEditorLike | null;
  /** Retorna o editor rico ou dispara toast informativo. */
  getRichOrToast: () => RichEditorLike | null;
  /** Executa uma ação best-effort com o editor rico. */
  run: (fn: (rich: RichEditorLike) => void) => boolean;
  /** Verifica se uma ação pode rodar (best-effort) com o editor rico. */
  canRun: (fn: (rich: RichEditorLike) => boolean | undefined) => boolean;
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

  const rich = (args.richEditorRef.current as RichEditorLike | null) ?? null;

  const canUseRich =
    !!args.activeTab &&
    !args.isAsking &&
    (requireTabMode ? args.activeTab.mode === requireTabMode : true) &&
    !!rich;

  const getRichOrToast = (): RichEditorLike | null => {
    if (canUseRich) return rich;
    const toast = args.addToast;
    if (toast) {
      toast(args.notReadyToastMessage ?? 'Editor rico ainda não está pronto.', 'info');
    }
    return null;
  };

  const run = (fn: (rich: RichEditorLike) => void): boolean => {
    if (!canUseRich || !rich) return false;
    try {
      fn(rich);
      return true;
    } catch {
      // best-effort
      return false;
    }
  };

  const canRun = (fn: (rich: RichEditorLike) => boolean | undefined) => {
    if (!canUseRich || !rich) return false;
    try {
      return Boolean(fn(rich));
    } catch {
      return false;
    }
  };

  return { canUseRich, rich, getRichOrToast, run, canRun };
}
