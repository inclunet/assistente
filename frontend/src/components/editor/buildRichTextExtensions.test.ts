import { describe, expect, it } from 'vitest';
import { Editor } from '@tiptap/core';
import { buildRichTextExtensions } from './buildRichTextExtensions';
import type { TipTapMarkdownStorage } from '../../pages/editorTypes';

describe('buildRichTextExtensions', () => {
  it('retorna lista de extensoes', () => {
    const result = buildRichTextExtensions({
      placeholder: 'x',
      imageFallbackLabel: 'Imagem sem descrição',
      imageLabelPrefix: 'Imagem',
    });
    expect(Array.isArray(result)).toBe(true);
    expect(result.length).toBeGreaterThan(0);
  });

  it('renderiza imagens Markdown com nome acessivel no texto rico', () => {
    const editor = new Editor({
      extensions: buildRichTextExtensions({
        placeholder: 'x',
        imageFallbackLabel: 'Imagem sem descrição',
        imageLabelPrefix: 'Imagem',
      }),
      content: '![Gato laranja](assets/gato.png)',
    });

    const imageNode = editor.view.dom.querySelector('[data-rich-image-node="true"]');
    const img = editor.view.dom.querySelector('img');

    expect(imageNode).not.toBeNull();
    expect(imageNode).toHaveAttribute('role', 'group');
    expect(imageNode).toHaveAttribute('aria-label', 'Imagem: Gato laranja');
    expect(imageNode).toHaveAttribute('tabindex', '0');
    expect(imageNode).toHaveTextContent('Imagem: Gato laranja');
    expect(img).not.toBeNull();
    expect(img).toHaveAttribute('src', 'assets/gato.png');
    expect(img).toHaveAttribute('alt', 'Gato laranja');
    expect(img).toHaveAttribute('title', 'Gato laranja');

    editor.destroy();
  });

  it('remove src inseguro de imagens no texto rico', () => {
    const editor = new Editor({
      extensions: buildRichTextExtensions({
        placeholder: 'x',
        imageFallbackLabel: 'Imagem sem descrição',
        imageLabelPrefix: 'Imagem',
      }),
      content: '',
    });
    const commands = editor.commands as {
      setImage?: (attrs: { src: string; alt?: string }) => boolean;
    };
    commands.setImage?.({ src: 'javascript:alert(1)', alt: 'Imagem perigosa' });

    const img = editor.view.dom.querySelector('img');

    expect(img).not.toBeNull();
    expect(img).not.toHaveAttribute('src');
    expect(editor.getHTML()).not.toContain('javascript:');
    expect(editor.getHTML()).not.toContain('src=""');
    const markdownStorage = (editor.storage as { markdown?: TipTapMarkdownStorage }).markdown;
    expect(markdownStorage?.getMarkdown).toBeTypeOf('function');
    expect(markdownStorage?.getMarkdown?.()).not.toContain('javascript:');

    editor.destroy();
  });
});
