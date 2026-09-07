import type { RefObject } from 'react';

import type { EditorInsertRequest, EditorMode, EditorDocument } from '../../store/editorStore';
import type { AddToastFn, FileMenuItem } from './types';
import type { RichTextEditorHandle } from '../../components/editor/RichTextEditor';

export type EditorMenuBaseContext = {
  activeTab: EditorDocument | null;
  isAsking: boolean;
  editorReadyNonce: number;
  richEditorRef: RefObject<unknown>;
};

export type InsertMenuContext = EditorMenuBaseContext & {
  applyInsertRequest: (req: EditorInsertRequest) => Promise<boolean>;
  appendMarkdownToDocument: (content: string) => void;
  focusEditorSoon: () => void;
  addToast: AddToastFn;
};

export type FormatMenuContext = EditorMenuBaseContext & {
  richEditorHandleRef: RefObject<RichTextEditorHandle | null>;
};

export type ModeMenuContext = {
  activeTab: EditorDocument | null;
  isAsking: boolean;
  setActiveTabMode: (nextMode: EditorMode) => void;
};

export type FileMenuContext = {
  fileMenuItems: FileMenuItem[];
  onSelect: (value: string) => void | Promise<void>;
};
