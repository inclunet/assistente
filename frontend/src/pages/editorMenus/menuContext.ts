import type { RefObject } from 'react';

import type { EditorInsertRequest, EditorMode, EditorTab } from '../../store/editorStore';
import type { AddToastFn, FileMenuItem } from './types';
import type { RichTextEditorHandle } from '../../components/editor/RichTextEditor';

export type EditorMenuBaseContext = {
  activeTab: EditorTab | null;
  isAsking: boolean;
  /** Apenas para forçar recomputação no consumidor (hook deps). */
  editorReadyNonce: number;
  /** Instância do TipTap editor (best-effort, tipado como unknown). */
  richEditorRef: RefObject<unknown>;
};

export type InsertMenuContext = EditorMenuBaseContext & {
  applyInsertRequest: (req: EditorInsertRequest) => Promise<boolean>;
  focusEditorSoon: () => void;
  addToast: AddToastFn;
};

export type FormatMenuContext = EditorMenuBaseContext & {
  richEditorHandleRef: RefObject<RichTextEditorHandle | null>;
};

export type ModeMenuContext = {
  activeTab: EditorTab | null;
  isAsking: boolean;
  setActiveTabMode: (nextMode: EditorMode) => void;
};

export type FileMenuContext = {
  fileMenuItems: FileMenuItem[];
  onSelect: (value: string) => void | Promise<void>;
};
