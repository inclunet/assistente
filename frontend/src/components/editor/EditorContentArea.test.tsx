import { type Ref, forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
import { render, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useEditorStore, type EditorDocument } from '../../store/editorStore';
import { EditorContentArea } from './EditorContentArea';
import type { RichTextEditorHandle } from './RichTextEditor';

const richEditorHandle = {
  flushMarkdown: vi.fn(),
  getMarkdown: vi.fn(),
  openLinkDialog: vi.fn(),
  applyMermaidById: vi.fn(),
  removeMermaidById: vi.fn(),
};
const announceMock = vi.hoisted(() => vi.fn());
const fakeRichEditorInstance = vi.hoisted(() => ({
  commands: {
    focus: vi.fn(),
  },
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, number>) => {
      if (key === 'editor.presentation.slideLabel') {
        return `Slide ${values?.current} de ${values?.total}`;
      }
      return key;
    },
  }),
}));

vi.mock('../ui/CodeEditor', () => ({
  CodeEditor: () => <div data-testid="code-editor" />,
}));

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: () => <div data-testid="markdown-renderer" />,
}));

vi.mock('./RevealRenderer', () => ({
  RevealRenderer: () => <div data-testid="reveal-renderer" />,
}));

vi.mock('./RichTextEditor', () => ({
  RichTextEditor: forwardRef(
    (
      props: { markdown: string; readOnly?: boolean; onEditorReady?: (editor: unknown) => void },
      ref: Ref<RichTextEditorHandle>
    ) => {
      useImperativeHandle(ref, () => richEditorHandle);
      const onEditorReadyRef = useRef(props.onEditorReady);
      onEditorReadyRef.current = props.onEditorReady;
      // Espelha o RichTextEditor real: notifica a instância na montagem e null na desmontagem,
      // permitindo detectar remontagens indevidas nos testes.
      useEffect(() => {
        onEditorReadyRef.current?.(fakeRichEditorInstance);
        return () => onEditorReadyRef.current?.(null);
      }, []);
      return <div data-testid="rich-text-editor" data-readonly={props.readOnly ? 'true' : 'false'}>{props.markdown}</div>;
    }
  ),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

const clearRichEditorHistoryMock = vi.hoisted(() => vi.fn());
vi.mock('./richEditorHistory', () => ({
  clearRichEditorHistory: clearRichEditorHistoryMock,
}));

function renderContentArea(
  activeTab: EditorDocument,
  props: Partial<Parameters<typeof EditorContentArea>[0]> = {}
) {
  return render(
    <EditorContentArea
      activeTab={activeTab}
      isAsking={false}
      debouncedMarkdownForPreview={activeTab.markdown}
      onMarkdownChange={vi.fn()}
      onMonacoMount={vi.fn()}
      onRichMarkdownChange={vi.fn()}
      onRichEditorReady={vi.fn()}
      revealAppendNonce={0}
      revealSlideNavigationRequest={null}
      revealFullscreenRequestNonce={0}
      richEditorHandleRef={{ current: richEditorHandle }}
      onRequestEditMermaid={vi.fn()}
      onOpenMermaid={vi.fn()}
      onRemoveMermaid={vi.fn()}
      {...props}
    />
  );
}

describe('EditorContentArea Reveal rich mode', () => {
  beforeEach(() => {
    richEditorHandle.flushMarkdown.mockReset();
    richEditorHandle.getMarkdown.mockReset();
    richEditorHandle.openLinkDialog.mockReset();
    richEditorHandle.applyMermaidById.mockReset();
    richEditorHandle.removeMermaidById.mockReset();
    announceMock.mockReset();
    fakeRichEditorInstance.commands.focus.mockReset();
    clearRichEditorHistoryMock.mockReset();
    useEditorStore.setState({ documents: {} });
  });

  it('troca de slide sem remontar o editor, aplica o novo conteúdo e foca o início', async () => {
    // Formato byte-idêntico ao round-trip de replaceRevealSlide (sem linha em
    // branco antes do separador), para poder afirmar zero emissões na troca.
    const markdown = `<!-- .slide: class="title-slide" -->

# Slide 1
---

## Slide 2`;
    const activeTab: EditorDocument = {
      id: 'doc-1',
      title: 'Deck',
      markdown,
      mode: 'rich',
    };
    const onRichEditorReady = vi.fn();
    const onRichMarkdownChange = vi.fn();
    // Conteúdo do slide atual sem edições pendentes: a troca não deve emitir nada.
    richEditorHandle.getMarkdown.mockReturnValue('# Slide 1');
    useEditorStore.getState().hydrate({ documents: { [activeTab.id]: activeTab } });

    const { rerender, getByTestId } = renderContentArea(activeTab, { onRichEditorReady, onRichMarkdownChange });

    expect(onRichEditorReady).toHaveBeenCalledTimes(1);
    expect(onRichEditorReady).toHaveBeenCalledWith(fakeRichEditorInstance);
    expect(getByTestId('rich-text-editor').textContent).toBe('# Slide 1');

    rerender(
      <EditorContentArea
        activeTab={activeTab}
        isAsking={false}
        debouncedMarkdownForPreview={markdown}
        onMarkdownChange={vi.fn()}
        onMonacoMount={vi.fn()}
        onRichMarkdownChange={onRichMarkdownChange}
        onRichEditorReady={onRichEditorReady}
        revealAppendNonce={0}
        revealSlideNavigationRequest={{ index: 1, nonce: 1 }}
        revealFullscreenRequestNonce={0}
        richEditorHandleRef={{ current: richEditorHandle }}
        onRequestEditMermaid={vi.fn()}
        onOpenMermaid={vi.fn()}
        onRemoveMermaid={vi.fn()}
      />
    );

    await waitFor(() => {
      expect(getByTestId('rich-text-editor').textContent).toBe('## Slide 2');
    });

    // Sem remontagem: onEditorReady não é chamado de novo nem recebe null.
    expect(onRichEditorReady).toHaveBeenCalledTimes(1);
    expect(onRichEditorReady).not.toHaveBeenCalledWith(null);
    // Cursor no início do novo slide, foco mantido no editor.
    expect(fakeRichEditorInstance.commands.focus).toHaveBeenCalledWith('start');
    // Histórico de undo limpo na troca: Ctrl+Z não pode restaurar o slide anterior.
    expect(clearRichEditorHistoryMock).toHaveBeenCalledWith(fakeRichEditorInstance);
    // Slide anterior sem edições: nenhuma emissão espúria de markdown na troca.
    expect(onRichMarkdownChange).not.toHaveBeenCalled();
  });

  it('mescla edições pendentes ao navegar para outro slide', async () => {
    const markdown = `<!-- .slide: class="title-slide" -->

# Slide 1

---

## Slide 2`;
    const activeTab: EditorDocument = {
      id: 'doc-1',
      title: 'Deck',
      markdown,
      mode: 'rich',
    };
    const onRichMarkdownChange = vi.fn();
    richEditorHandle.getMarkdown.mockReturnValue('# Slide 1 editado');
    useEditorStore.getState().hydrate({ documents: { [activeTab.id]: activeTab } });

    const { rerender } = renderContentArea(activeTab, { onRichMarkdownChange });

    rerender(
      <EditorContentArea
        activeTab={activeTab}
        isAsking={false}
        debouncedMarkdownForPreview={markdown}
        onMarkdownChange={vi.fn()}
        onMonacoMount={vi.fn()}
        onRichMarkdownChange={onRichMarkdownChange}
        onRichEditorReady={vi.fn()}
        revealAppendNonce={0}
        revealSlideNavigationRequest={{ index: 1, nonce: 1 }}
        revealFullscreenRequestNonce={0}
        richEditorHandleRef={{ current: richEditorHandle }}
        onRequestEditMermaid={vi.fn()}
        onOpenMermaid={vi.fn()}
        onRemoveMermaid={vi.fn()}
      />
    );

    await waitFor(() => {
      expect(onRichMarkdownChange).toHaveBeenCalled();
    });

    const nextMarkdown = onRichMarkdownChange.mock.calls[onRichMarkdownChange.mock.calls.length - 1]?.[0] as string;
    expect(richEditorHandle.flushMarkdown).toHaveBeenCalled();
    expect(nextMarkdown).toContain('<!-- .slide: class="title-slide" -->\n\n# Slide 1 editado');
    expect(nextMarkdown).toContain('## Slide 2');
    expect(announceMock).toHaveBeenCalledWith('Slide 2 de 2');
  });

  it('não normaliza separadores dentro de blocos fenced', async () => {
    const markdown = `<!-- .slide: class="content-slide" -->

# Slide 1

---

## Slide 2`;
    const activeTab: EditorDocument = {
      id: 'doc-1',
      title: 'Deck',
      markdown,
      mode: 'rich',
    };
    const onRichMarkdownChange = vi.fn();
    richEditorHandle.getMarkdown.mockReturnValue(`\`\`\`yaml
---
key: value
---
\`\`\`

---

Texto depois`);
    useEditorStore.getState().hydrate({ documents: { [activeTab.id]: activeTab } });

    const { rerender } = renderContentArea(activeTab, { onRichMarkdownChange });

    rerender(
      <EditorContentArea
        activeTab={activeTab}
        isAsking={false}
        debouncedMarkdownForPreview={markdown}
        onMarkdownChange={vi.fn()}
        onMonacoMount={vi.fn()}
        onRichMarkdownChange={onRichMarkdownChange}
        onRichEditorReady={vi.fn()}
        revealAppendNonce={0}
        revealSlideNavigationRequest={{ index: 1, nonce: 1 }}
        revealFullscreenRequestNonce={0}
        richEditorHandleRef={{ current: richEditorHandle }}
        onRequestEditMermaid={vi.fn()}
        onOpenMermaid={vi.fn()}
        onRemoveMermaid={vi.fn()}
      />
    );

    await waitFor(() => {
      expect(onRichMarkdownChange).toHaveBeenCalled();
    });

    const nextMarkdown = onRichMarkdownChange.mock.calls[onRichMarkdownChange.mock.calls.length - 1]?.[0] as string;
    expect(nextMarkdown).toContain('```yaml\n---\nkey: value\n---\n```');
    expect(nextMarkdown).toContain('\n___\n\nTexto depois');
  });

  it('não trata autolinks Markdown como HTML cru em slides Reveal', () => {
    const markdown = `<!-- .slide: class="content-slide" -->

Veja <https://example.com>`;
    const activeTab: EditorDocument = {
      id: 'doc-1',
      title: 'Deck',
      markdown,
      mode: 'rich',
    };
    useEditorStore.getState().hydrate({ documents: { [activeTab.id]: activeTab } });

    const { getByTestId } = renderContentArea(activeTab);

    expect(getByTestId('rich-text-editor')).toHaveAttribute('data-readonly', 'false');
  });

  it('mantém slides com HTML cru em modo somente leitura', () => {
    const markdown = `<!-- .slide: class="content-slide" -->

<div>HTML bruto</div>`;
    const activeTab: EditorDocument = {
      id: 'doc-1',
      title: 'Deck',
      markdown,
      mode: 'rich',
    };
    useEditorStore.getState().hydrate({ documents: { [activeTab.id]: activeTab } });

    const { getByTestId } = renderContentArea(activeTab);

    expect(getByTestId('rich-text-editor')).toHaveAttribute('data-readonly', 'true');
  });
});
