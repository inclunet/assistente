import { createNodeFromContent } from '@tiptap/core';
import type { Schema } from '@tiptap/pm/model';

/**
 * Tipos estruturais mínimos de ProseMirror. Usar tipos próprios (em vez dos
 * reais de prosemirror-model/state) permite que os testes usem editores fake
 * simples e evita acoplar este módulo a detalhes internos do TipTap.
 * A sintaxe de método (bivariante) mantém os tipos reais compatíveis.
 */
type FragmentLike = {
  size: number;
  findDiffStart(other: FragmentLike): number | null;
  findDiffEnd(other: FragmentLike): { a: number; b: number } | null;
};

export type DocLike = {
  content: FragmentLike;
  eq(other: DocLike): boolean;
  slice(from: number, to: number): unknown;
};

type TransactionLike = {
  replace(from: number, to: number, slice?: unknown): unknown;
  setMeta(key: string, value: unknown): unknown;
};

export type IncrementalEditorLike = {
  storage?: unknown;
  schema?: unknown;
  state?: {
    doc: DocLike;
    tr: TransactionLike;
  };
  view?: {
    dispatch?(tr: TransactionLike): void;
  };
};

/**
 * Parseia markdown para um doc ProseMirror usando o MESMO pipeline do
 * `setContent` do tiptap-markdown: `storage.markdown.parser.parse()` converte
 * markdown em HTML (markdown-it + specs das extensões) e o
 * `createNodeFromContent` do @tiptap/core (com `slice: false`, como o
 * `createDocument` interno) converte o HTML em doc via DOMParser do schema.
 *
 * Retorna `null` quando o editor não expõe parser/schema (ex.: fakes de teste)
 * ou quando o parse não produz um doc válido — o chamador faz fallback.
 */
export function parseExternalMarkdownToDoc(
  editor: IncrementalEditorLike,
  markdown: string
): DocLike | null {
  const storage = editor.storage as Record<string, unknown> | undefined;
  const markdownStorage = storage?.markdown as
    | { parser?: { parse?: (content: string) => unknown } }
    | undefined;
  const parser = markdownStorage?.parser;
  const schema = editor.schema as Schema | undefined;
  if (!parser || typeof parser.parse !== 'function' || !schema) return null;

  const parsedHtml = parser.parse(markdown);
  if (typeof parsedHtml !== 'string') return null;

  const doc = createNodeFromContent(parsedHtml, schema, {
    slice: false,
    parseOptions: {},
  }) as unknown as DocLike | null;

  if (!doc || !doc.content || typeof doc.eq !== 'function' || typeof doc.slice !== 'function') {
    return null;
  }
  return doc;
}

/**
 * Aplica `nextMarkdown` ao editor substituindo apenas o range mínimo alterado
 * (Fragment.findDiffStart/findDiffEnd), em UMA transação. A seleção, o cursor
 * e o scroll são remapeados automaticamente pelo mapping da transação — ao
 * contrário do `setContent` total, que os destrói.
 *
 * A transação leva `addToHistory: false` (aplicações externas não entram no
 * undo) e `preventUpdate: true` (não dispara `onUpdate`; o guard
 * `isApplyingExternalMarkdownRef` continua ativo como segunda barreira).
 *
 * Retorna `true` quando o conteúdo foi aplicado (ou já era idêntico — nenhuma
 * transação necessária) e `false` quando o chamador deve fazer fallback para o
 * `setContent` total.
 */
export function applyExternalMarkdownIncrementally(
  editor: IncrementalEditorLike,
  nextMarkdown: string
): boolean {
  try {
    const state = editor.state;
    // dispatch é chamado via view (view.dispatch(tr)) para preservar o binding
    // de `this` — no ProseMirror clássico, dispatch solto quebraria.
    const view = editor.view;
    if (!state?.doc?.content || typeof view?.dispatch !== 'function') return false;

    const nextDoc = parseExternalMarkdownToDoc(editor, nextMarkdown);
    if (!nextDoc) return false;

    const currentDoc = state.doc;
    if (currentDoc.eq(nextDoc)) return true;

    const start = currentDoc.content.findDiffStart(nextDoc.content);
    if (start === null || start === undefined) return true;

    const diffEnd = currentDoc.content.findDiffEnd(nextDoc.content);
    if (!diffEnd) return false;

    let endCurrent = diffEnd.a;
    let endNext = diffEnd.b;
    // Docs com trechos repetidos podem ter diffEnd antes do diffStart (o range
    // "compartilhado" se sobrepõe). Clampa como o ProseMirror faz internamente.
    const overlap = start - Math.min(endCurrent, endNext);
    if (overlap > 0) {
      endCurrent += overlap;
      endNext += overlap;
    }

    const tr = state.tr;
    tr.replace(start, endCurrent, nextDoc.slice(start, endNext));
    tr.setMeta('addToHistory', false);
    tr.setMeta('preventUpdate', true);
    view.dispatch(tr);
    return true;
  } catch {
    return false;
  }
}
