import { type Ref, forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
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
const isModalOpenMock = vi.hoisted(() => vi.fn(() => false));
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
  MarkdownRenderer: (props: { tabNavigation?: string }) => (
    <div
      data-testid="markdown-renderer"
      data-tab-navigation={props.tabNavigation}
    >
      <a href="https://example.com" tabIndex={props.tabNavigation === 'enabled' ? 0 : -1}>
        Link do documento
      </a>
    </div>
  ),
}));

vi.mock('./RevealRenderer', () => ({
  RevealRenderer: (props: { tabNavigation?: string }) => (
    <div data-testid="reveal-renderer" data-tab-navigation={props.tabNavigation} />
  ),
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
  announce: announceMock,
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

vi.mock('../ui/Modal', () => ({
  isModalOpen: isModalOpenMock,
}));

const clearRichEditorHistoryMock = vi.hoisted(() => vi.fn());
vi.mock('./richEditorHistory', () => ({
  clearRichEditorHistory: clearRichEditorHistoryMock,
}));

function contentAreaElement(
  activeTab: EditorDocument,
  props: Partial<Parameters<typeof EditorContentArea>[0]> = {}
) {
  return (
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

function renderContentArea(
  activeTab: EditorDocument,
  props: Partial<Parameters<typeof EditorContentArea>[0]> = {}
) {
  return render(contentAreaElement(activeTab, props));
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
    isModalOpenMock.mockReturnValue(false);
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

describe('EditorContentArea document view', () => {
  beforeEach(() => {
    announceMock.mockReset();
    isModalOpenMock.mockReturnValue(false);
  });

  it('renderiza projeção somente para leitura e anuncia o formato', () => {
    renderContentArea({
      id: 'docx-view',
      title: 'manual.docx',
      markdown: '# Manual',
      mode: 'view',
      filePath: 'C:/tmp/manual.docx',
      readOnly: true,
      projection: {
        format: 'docx',
        pages: 3,
        warnings: ['Conversão parcial'],
      },
    });

    expect(screen.getByText('editor.documentView.readOnlyBanner')).toBeInTheDocument();
    expect(screen.getByText('editor.documentView.partialExtraction')).toBeInTheDocument();
    expect(screen.queryByText('editor.hints.previewMermaid')).not.toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('editor.documentView.openedAnnouncement');
  });

  it('ativa leitura escopada sem prender Tab ou F6 e Escape devolve o conteúdo', async () => {
    const user = userEvent.setup();
    const { container } = renderContentArea({
      id: 'markdown-view',
      title: 'leitura.md',
      markdown: '# Leitura\n\n[Link](https://example.com)',
      mode: 'view',
      filePath: 'C:/tmp/leitura.md',
      readOnly: false,
      projection: null,
    });
    const renderedDocument = container.querySelector<HTMLElement>(
      '[data-editor-rendered-document="true"]',
    );
    expect(renderedDocument).not.toBeNull();
    expect(renderedDocument).toHaveAttribute('role', 'group');
    expect(renderedDocument).toHaveAttribute('tabindex', '0');
    expect(screen.getByTestId('markdown-renderer')).toHaveAttribute(
      'data-tab-navigation',
      'disabled',
    );

    renderedDocument!.focus();
    fireEvent.keyDown(renderedDocument!, { key: 'Enter' });

    expect(renderedDocument).toHaveAttribute('role', 'document');
    expect(renderedDocument).toHaveFocus();
    expect(screen.getByTestId('markdown-renderer')).toHaveAttribute(
      'data-tab-navigation',
      'enabled',
    );
    expect(announceMock).toHaveBeenCalledWith('editor.documentView.readingOpened');

    const outside = document.createElement('button');
    outside.textContent = 'Depois do documento';
    document.body.append(outside);
    const link = screen.getByRole('link', { name: 'Link do documento' });
    link.focus();
    await user.tab();
    expect(outside).toHaveFocus();

    // O perfil scoped não captura F6: a landmark global pode processá-lo.
    expect(fireEvent.keyDown(window, { key: 'F6' })).toBe(true);

    isModalOpenMock.mockReturnValue(true);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(outside).toHaveFocus();

    isModalOpenMock.mockReturnValue(false);
    const menu = document.createElement('div');
    menu.setAttribute('role', 'menu');
    document.body.append(menu);
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(outside).toHaveFocus();
    menu.remove();

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(renderedDocument).toHaveFocus();
    expect(announceMock).toHaveBeenCalledWith('editor.documentView.readingFocused');
    outside.remove();
  });

  it('oferece o mesmo documento focável para projeções somente leitura', () => {
    const { container } = renderContentArea({
      id: 'pdf-reading',
      title: 'manual.pdf',
      markdown: '# Manual',
      mode: 'view',
      filePath: 'C:/tmp/manual.pdf',
      readOnly: true,
      projection: { format: 'pdf', warnings: [] },
    });

    const renderedDocument = container.querySelector<HTMLElement>(
      '[data-editor-rendered-document="true"]',
    );
    renderedDocument?.focus();
    fireEvent.keyDown(renderedDocument!, { key: 'Enter' });

    expect(renderedDocument).toHaveAttribute('role', 'document');
    expect(screen.getByTestId('markdown-renderer')).toHaveAttribute(
      'data-tab-navigation',
      'enabled',
    );
  });

  it('habilita a ordem de Tab também no preview Reveal', () => {
    const { container } = renderContentArea({
      id: 'reveal-reading',
      title: 'slides.md',
      markdown: '<!-- .slide: class="title-slide" -->\n\n# Slide 1\n\n---\n\n# Slide 2\n\n---\n\n# Slide 3',
      mode: 'view',
    });
    const renderedDocument = container.querySelector<HTMLElement>(
      '[data-editor-rendered-document="true"]',
    );
    expect(screen.getByTestId('reveal-renderer')).toHaveAttribute(
      'data-tab-navigation',
      'disabled',
    );

    renderedDocument?.focus();
    fireEvent.keyDown(renderedDocument!, { key: 'Enter' });

    expect(screen.getByTestId('reveal-renderer')).toHaveAttribute(
      'data-tab-navigation',
      'enabled',
    );
  });

  it('preserva role document em rerenders e desativa a leitura ao ocultar o painel', () => {
    const activeTab: EditorDocument = {
      id: 'preview-lifecycle',
      title: 'preview.md',
      markdown: '# Preview',
      mode: 'view',
      filePath: 'C:/tmp/preview.md',
    };
    const { container, rerender } = renderContentArea(activeTab, { isPanelActive: true });
    const renderedDocument = container.querySelector<HTMLElement>(
      '[data-editor-rendered-document="true"]',
    );
    renderedDocument?.focus();
    fireEvent.keyDown(renderedDocument!, { key: 'Enter' });
    expect(renderedDocument).toHaveAttribute('role', 'document');

    rerender(contentAreaElement(activeTab, {
      isPanelActive: true,
      debouncedMarkdownForPreview: '# Preview atualizado',
    }));
    expect(renderedDocument).toHaveAttribute('role', 'document');

    const replacementTab = {
      ...activeTab,
      title: 'outro.md',
      filePath: 'C:/tmp/outro.md',
      markdown: '# Outro arquivo',
    };
    rerender(contentAreaElement(replacementTab, { isPanelActive: true }));
    expect(renderedDocument).toHaveAttribute('role', 'group');
    expect(screen.getByTestId('markdown-renderer')).toHaveAttribute(
      'data-tab-navigation',
      'disabled',
    );

    renderedDocument?.focus();
    fireEvent.keyDown(renderedDocument!, { key: 'Enter' });
    expect(renderedDocument).toHaveAttribute('role', 'document');

    const outside = document.createElement('button');
    document.body.append(outside);
    outside.focus();
    rerender(contentAreaElement(replacementTab, { isPanelActive: false }));
    expect(renderedDocument).toHaveAttribute('role', 'group');

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(outside).toHaveFocus();
    outside.remove();
  });
});
