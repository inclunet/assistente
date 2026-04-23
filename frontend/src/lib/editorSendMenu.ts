import type { MenuItem } from '../components/menu';

export interface EditorSendTargetOption {
  id: string;
  title: string;
}

interface SendToEditorPayloadBase {
  format: 'markdown' | 'html' | 'plain';
  title?: string;
  content: string;
}

export type SendToEditorPayload =
  | ({
      target: 'document';
      targetDocumentId: string;
    } & SendToEditorPayloadBase)
  | ({
      target: 'new_document';
      targetDocumentId?: never;
    } & SendToEditorPayloadBase);

export interface EditorSendFormatOption<TPayload extends object = {}> {
  id: string;
  label: string;
  payload: SendToEditorPayloadBase & TPayload;
}

interface BuildEditorDestinationSubmenuParams<TPayload extends object> {
  baseId: string;
  editorTargets: EditorSendTargetOption[];
  formats: Array<EditorSendFormatOption<TPayload>>;
  onSendToEditor: (payload: SendToEditorPayload & TPayload) => void;
  newDocumentLabel: string;
  fallbackDocumentTitle: string;
}

function normalizeEditorTarget(target: EditorSendTargetOption) {
  const id = String(target.id || '').trim();
  if (!id) return null;
  return {
    id,
    title: String(target.title || '').trim(),
  };
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

  const buildFormatItems = (
    destination:
      | { target: 'document'; targetDocumentId: string }
      | { target: 'new_document' }
  ) =>
    formats.map((format, formatIndex) => ({
      id: `${baseId}-${destination.target}-${('targetDocumentId' in destination ? destination.targetDocumentId : 'new') || 'new'}-${format.id || formatIndex}`,
      label: format.label,
      action: () => {
        if (destination.target === 'document') {
          onSendToEditor({
            ...format.payload,
            target: 'document',
            targetDocumentId: destination.targetDocumentId,
          });
          return;
        }
        onSendToEditor({
          ...format.payload,
          target: 'new_document',
        });
      },
    }));

  const items = editorTargets
    .map((target, index) => {
      const normalized = normalizeEditorTarget(target);
      if (!normalized) return null;
      const item: MenuItem = {
        id: `${baseId}-document-${normalized.id || index}`,
        label: normalized.title || fallbackDocumentTitle,
        submenu: buildFormatItems({
          target: 'document',
          targetDocumentId: normalized.id,
        }),
      };
      return item;
    })
    .filter((item): item is MenuItem => item !== null);

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
