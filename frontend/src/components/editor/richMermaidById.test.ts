import { describe, expect, it, vi } from 'vitest';
import { applyMermaidById, removeMermaidById, findMermaidNodeById } from './richMermaidById';

describe('richMermaidById', () => {
  it('encontra bloco mermaid por id', () => {
    const editor = {
      state: {
        doc: {
          descendants: (fn: (node: unknown, pos: number) => boolean) => {
            fn({ attrs: { language: 'mermaid', mermaidBlockId: 'm1' }, nodeSize: 5 }, 10);
          },
        },
      },
    };

    const hit = findMermaidNodeById(editor, 'm1');
    expect(hit?.pos).toBe(10);
  });

  it('aplica novo codigo quando encontra', () => {
    const replaceSpy = vi.fn();
    const editor = {
      state: {
        doc: {
          descendants: (fn: (node: unknown, pos: number) => boolean) => {
            fn({ attrs: { language: 'mermaid', mermaidBlockId: 'm1' }, nodeSize: 5 }, 10);
          },
        },
      },
      commands: {
        command: (cb: (ctx: { tr: { replaceWith: typeof replaceSpy }; state: { schema: { text: (text: string) => string } } }) => boolean) => {
          cb({ tr: { replaceWith: replaceSpy }, state: { schema: { text: (text: string) => text } } });
        },
      },
    };

    const ok = applyMermaidById(editor, 'm1', 'novo');
    expect(ok).toBe(true);
    expect(replaceSpy).toHaveBeenCalled();
  });

  it('remove bloco quando encontra', () => {
    const deleteSpy = vi.fn();
    const editor = {
      state: {
        doc: {
          descendants: (fn: (node: unknown, pos: number) => boolean) => {
            fn({ attrs: { language: 'mermaid', mermaidBlockId: 'm1' }, nodeSize: 5 }, 10);
          },
        },
      },
      commands: {
        command: (cb: (ctx: { tr: { delete: typeof deleteSpy } }) => boolean) => {
          cb({ tr: { delete: deleteSpy } });
        },
      },
    };

    const ok = removeMermaidById(editor, 'm1');
    expect(ok).toBe(true);
    expect(deleteSpy).toHaveBeenCalled();
  });
});
