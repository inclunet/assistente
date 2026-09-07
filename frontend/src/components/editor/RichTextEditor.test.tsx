import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { RichTextEditor } from './RichTextEditor';

const openLinkDialogSpy = vi.fn();

// Editor TipTap falso com o mínimo necessário para o RichTextEditor e o
// useRichMarkdownSync REAL: setContent dispara onUpdate (como o TipTap faz)
// para exercitar o guard isApplyingExternalMarkdownRef.
const tiptapMocks = vi.hoisted(() => {
  const state = {
    currentMarkdown: '',
    options: null as Record<string, unknown> | null,
  };
  const editor = {
    setEditable: vi.fn(),
    commands: {
      setContent: vi.fn((markdown: string) => {
        state.currentMarkdown = markdown;
        (state.options?.onUpdate as ((ctx: { editor: unknown }) => void) | undefined)?.({ editor });
      }),
      focus: vi.fn(),
    },
    storage: {
      markdown: {
        getMarkdown: () => state.currentMarkdown,
      },
    },
  };
  return { state, editor };
});

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@tiptap/react', () => ({
  EditorContent: () => <div data-testid="editor-content" />,
  useEditor: (options: Record<string, unknown>) => {
    tiptapMocks.state.options = options;
    return tiptapMocks.editor;
  },
}));

vi.mock('./useRichLinkDialog', () => ({
  useRichLinkDialog: () => openLinkDialogSpy,
}));

vi.mock('./buildRichTextExtensions', () => ({
  buildRichTextExtensions: () => [],
}));

describe('RichTextEditor', () => {
  beforeEach(() => {
    openLinkDialogSpy.mockReset();
    tiptapMocks.editor.setEditable.mockReset();
    tiptapMocks.editor.commands.setContent.mockClear();
    tiptapMocks.editor.commands.focus.mockReset();
    tiptapMocks.state.currentMarkdown = '';
    tiptapMocks.state.options = null;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renderiza editor e dispara Ctrl+K', () => {
    render(
      <RichTextEditor
        markdown=""
        onMarkdownChange={() => {}}
      />
    );

    const region = screen.getByRole('region', { name: 'editor.richText.label' });
    fireEvent.keyDown(region, { key: 'k', ctrlKey: true });

    expect(openLinkDialogSpy).toHaveBeenCalled();
  });

  it('aplica markdown externo (troca de slide) sem emitir onMarkdownChange espúrio', () => {
    vi.useFakeTimers();
    tiptapMocks.state.currentMarkdown = '# Slide 1';
    const onMarkdownChange = vi.fn();

    const { rerender } = render(
      <RichTextEditor markdown="# Slide 1" onMarkdownChange={onMarkdownChange} />
    );

    // Montagem inicial: conteúdo já está em sincronia, nada a aplicar.
    expect(tiptapMocks.editor.commands.setContent).not.toHaveBeenCalled();

    // Troca de slide: a prop markdown muda e o conteúdo é aplicado via setContent.
    rerender(<RichTextEditor markdown="## Slide 2" onMarkdownChange={onMarkdownChange} />);

    expect(tiptapMocks.editor.commands.setContent).toHaveBeenCalledTimes(1);
    expect(tiptapMocks.editor.commands.setContent).toHaveBeenCalledWith('## Slide 2');

    // O onUpdate disparado pelo setContent é coberto pelo guard: mesmo após
    // o debounce e a liberação do guard, nenhuma emissão espúria acontece.
    vi.advanceTimersByTime(1000);
    expect(onMarkdownChange).not.toHaveBeenCalled();
  });

  it('edições do usuário continuam emitindo onMarkdownChange após sync externo', () => {
    vi.useFakeTimers();
    tiptapMocks.state.currentMarkdown = '# Slide 1';
    const onMarkdownChange = vi.fn();

    const { rerender } = render(
      <RichTextEditor markdown="# Slide 1" onMarkdownChange={onMarkdownChange} />
    );
    rerender(<RichTextEditor markdown="## Slide 2" onMarkdownChange={onMarkdownChange} />);
    vi.advanceTimersByTime(1000);
    expect(onMarkdownChange).not.toHaveBeenCalled();

    // Simula digitação do usuário no novo slide.
    tiptapMocks.state.currentMarkdown = '## Slide 2 editado';
    (tiptapMocks.state.options?.onUpdate as (ctx: { editor: unknown }) => void)({
      editor: tiptapMocks.editor,
    });

    vi.advanceTimersByTime(300);
    expect(onMarkdownChange).toHaveBeenCalledTimes(1);
    expect(onMarkdownChange).toHaveBeenCalledWith('## Slide 2 editado');
  });
});
