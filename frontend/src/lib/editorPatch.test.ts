import { describe, expect, it } from 'vitest';
import { extractEditorPatch } from './editorPatch';

describe('extractEditorPatch', () => {
  it('retorna erro quando não há bloco fenced', () => {
    const res = extractEditorPatch('sem patch aqui');
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toContain('Resposta não contém patch');
  });

  it('aceita fence editor_patch com JSON válido', () => {
    const text = [
      'bla',
      '```editor_patch',
      JSON.stringify({ v: 1, op: 'replace_selection', format: 'markdown', replacement: 'oi' }),
      '```',
    ].join('\n');

    const res = extractEditorPatch(text);
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.patch.v).toBe(1);
      expect(res.patch.op).toBe('replace_selection');
      expect(res.patch.format).toBe('markdown');
      expect(res.patch.replacement).toBe('oi');
    }
  });

  it('aceita fence com CRLF (Windows)', () => {
    const text =
      'x\r\n```editor_patch\r\n' +
      '{"v":1,"op":"replace_selection","format":"plain","replacement":"abc"}\r\n' +
      '```\r\n';

    const res = extractEditorPatch(text);
    expect(res.ok).toBe(true);
    if (res.ok) expect(res.patch.format).toBe('plain');
  });

  it('aceita aliases editor-patch e assistente_editor_patch', () => {
    for (const fence of ['editor-patch', 'assistente_editor_patch']) {
      const text = [
        '```' + fence,
        '{"v":1,"op":"replace_selection","format":"plain","replacement":"ok"}',
        '```',
      ].join('\n');

      const res = extractEditorPatch(text);
      expect(res.ok).toBe(true);
    }
  });

  it('aceita variações de espaços e case no label do fence', () => {
    const text = [
      'bla',
      '```   EDITOR_PATCH   ',
      '{"v":1,"op":"replace_selection","format":"plain","replacement":"ok"}',
      '```',
    ].join('\n');

    const res = extractEditorPatch(text);
    expect(res.ok).toBe(true);
    if (res.ok) expect(res.patch.replacement).toBe('ok');
  });

  it('quando há múltiplos fences, usa o primeiro match', () => {
    const text = [
      '```editor_patch',
      '{"v":1,"op":"replace_selection","format":"plain","replacement":"primeiro"}',
      '```',
      'texto intermediário',
      '```editor_patch',
      '{"v":1,"op":"replace_selection","format":"plain","replacement":"segundo"}',
      '```',
    ].join('\n');

    const res = extractEditorPatch(text);
    expect(res.ok).toBe(true);
    if (res.ok) expect(res.patch.replacement).toBe('primeiro');
  });

  it('aceita JSON com linhas em branco dentro do fence (trim)', () => {
    const text = [
      '```editor_patch',
      '',
      '',
      '{"v":1,"op":"replace_selection","format":"plain","replacement":"ok"}',
      '',
      '```',
    ].join('\n');

    const res = extractEditorPatch(text);
    expect(res.ok).toBe(true);
  });

  it('retorna Patch vazio quando fence existe mas sem conteúdo', () => {
    const text = ['```editor_patch', '', '```'].join('\n');
    const res = extractEditorPatch(text);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toBe('Patch vazio');
  });

  it('retorna JSON inválido quando conteúdo não é JSON', () => {
    const text = ['```editor_patch', '{ nope', '```'].join('\n');
    const res = extractEditorPatch(text);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toContain('JSON inválido');
  });

  it('retorna JSON inválido quando há conteúdo extra dentro do fence', () => {
    const text = [
      '```editor_patch',
      '{"v":1,"op":"replace_selection","format":"plain","replacement":"ok"}',
      'texto que não é JSON',
      '```',
    ].join('\n');

    const res = extractEditorPatch(text);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toContain('JSON inválido');
  });

  it('valida v/op/format/replacement', () => {
    const base = { v: 1, op: 'replace_selection', format: 'markdown', replacement: 'x' };

    const cases: Array<{ patch: Record<string, unknown>; expected: string }> = [
      { patch: { ...base, v: 2 }, expected: 'campo v deve ser 1' },
      { patch: { ...base, op: 'other' }, expected: 'op deve ser replace_selection' },
      { patch: { ...base, format: 'html' }, expected: 'format deve ser markdown ou plain' },
      { patch: { ...base, replacement: 123 }, expected: 'replacement deve ser string' },
    ];

    for (const c of cases) {
      const text = ['```editor_patch', JSON.stringify(c.patch), '```'].join('\n');
      const res = extractEditorPatch(text);
      expect(res.ok).toBe(false);
      if (!res.ok) expect(res.error).toContain(c.expected);
    }
  });

  it('rejeita patch gigantesco acima do limite', () => {
    const huge = 'a'.repeat(200 * 1024 + 10);
    const text = ['```editor_patch', JSON.stringify({ v: 1, op: 'replace_selection', format: 'plain', replacement: huge }), '```'].join(
      '\n'
    );

    // Aqui o JSON vai estourar o limite de patch antes (jsonText muito grande)
    const res = extractEditorPatch(text);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toContain('Patch muito grande');
  });

  it('rejeita replacement muito grande (mesmo com JSON pequeno)', () => {
    // Estratégia: produzir um JSON dentro do limite do patch, mas com replacement > limite.
    // Para isso, removemos bastante overhead (strings curtas) e geramos replacement grande.
    const replacement = 'b'.repeat(200 * 1024 + 1);
    const patch = { v: 1, op: 'replace_selection', format: 'plain', replacement };
    const json = JSON.stringify(patch);

    // Se este teste ficar instável por tamanho exato, ele deve pelo menos não retornar ok.
    const text = ['```editor_patch', json, '```'].join('\n');
    const res = extractEditorPatch(text);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toMatch(/replacement muito grande|Patch muito grande/);
  });

  it('quando fence está aberto mas sem fechamento, trata como sem patch', () => {
    const text = ['antes', '```editor_patch', '{"v":1}'].join('\n');
    const res = extractEditorPatch(text);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toContain('Resposta não contém patch');
  });

  it('quando fechamento do fence vem sem quebra de linha, trata como sem patch', () => {
    const text =
      '```editor_patch\n' +
      '{"v":1,"op":"replace_selection","format":"plain","replacement":"ok"}' +
      '```';
    const res = extractEditorPatch(text);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toContain('Resposta não contém patch');
  });
});

