import { afterEach, describe, expect, it } from 'vitest';
import { Editor } from '@tiptap/core';
import { buildRichTextExtensions } from './buildRichTextExtensions';
import { findMermaidNodeById } from './richMermaidById';

const editors: Editor[] = [];
afterEach(() => editors.splice(0).forEach(editor => editor.destroy()));
function fixture(ids = ['m1', 'm2']) {
  const editor = new Editor({ extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: '', imageLabelPrefix: '' }),
    content: { type: 'doc', content: ids.map(id => ({ type: 'codeBlock', attrs: { language: 'mermaid', mermaidBlockId: id }, content: [{ type: 'text', text: 'graph TD' }] })) } });
  editors.push(editor); return editor;
}
describe('richMermaidById — lookup real; mutações migradas para useMermaidSession', () => {
  it('encontra bloco mermaid por id no schema de produção', () => {
    const editor = fixture(); const hit = findMermaidNodeById(editor, 'm2');
    expect(hit?.pos).toBe(editor.state.doc.firstChild!.nodeSize); expect(hit?.node.textContent).toBe('graph TD');
  });
  it('recusa id ausente sem fornecer alvo de aplicação', () => {
    expect(findMermaidNodeById(fixture(), 'missing')).toBeNull();
  });
  it('recusa id duplicado sem escolher alvo para remoção', () => {
    expect(findMermaidNodeById(fixture(['m1', 'm1']), 'm1')).toBeNull();
  });
});
