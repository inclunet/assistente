import type { MenuItem } from '../components/menu';

export interface EditorSendTargetOption {
  id: string;
  title: string;
}

export interface SendToEditorPayload {
  target: 'document' | 'new_document';
  targetDocumentId?: string;
  format: 'markdown' | 'html' | 'plain';
  title?: string;
  content: string;
}

export interface EditorSendFormatOption<TPayload extends object = {}> {
  id: string;
  label: string;
  payload: Omit<SendToEditorPayload, 'target' | 'targetDocumentId'> & TPayload;
}

interface BuildEditorDestinationSubmenuParams<TPayload extends object> {
  baseId: string;
  editorTargets: EditorSendTargetOption[];
  formats: Array<EditorSendFormatOption<TPayload>>;
  onSendToEditor: (payload: SendToEditorPayload & TPayload) => void;
  newDocumentLabel: string;
  fallbackDocumentTitle: string;
}

export function buildEditorDestinationSubmenu<TPayload extends object = {}>(
  params: BuildEditorDestinationSubmenuParams<TPayload>,
): MenuItem[] {
  const {
    baseId,
    editorTargets,
    formats,
    onSendToEditor,
    newDocumentLabel,
    fallbackDocumentTitle,
  } = params;

  const buildFormatItems = (destination: { target: 'document' | 'new_document'; targetDocumentId?: string }) =>
    formats.map((format, formatIndex) => ({
      id: `${baseId}-${destination.target}-${destination.targetDocumentId || 'new'}-${format.id || formatIndex}`,
      label: format.label,
      action: () =>
        onSendToEditor({
          ...format.payload,
          target: destination.target,
          targetDocumentId: destination.targetDocumentId,
        }),
    }));

  const items: MenuItem[] = editorTargets.map((target, index) => ({
    id: `${baseId}-document-${target.id || index}`,
    label: String(target.title || '').trim() || fallbackDocumentTitle,
    submenu: buildFormatItems({
      target: 'document',
      targetDocumentId: target.id,
    }),
  }));

  if (items.length > 0) {
    items.push({ id: `${baseId}-separator`, separator: true });
  }

  items.push({
    id: `${baseId}-new-document`,
    label: newDocumentLabel,
    submenu: buildFormatItems({
      target: 'new_document',
    }),
  });

  return items;
}
