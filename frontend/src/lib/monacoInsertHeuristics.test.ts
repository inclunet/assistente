import { describe, expect, it } from 'vitest';
import { computeMonacoInsertText } from './monacoInsertHeuristics';

describe('computeMonacoInsertText', () => {
  it('sem foco + seleção vazia em (1,1) => insere no fim com separador', () => {
    const r = computeMonacoInsertText({
      hasFocus: false,
      selectionIsEmpty: true,
      selectionStart: { lineNumber: 1, column: 1 },
      currentText: 'abc\n',
      content: 'novo',
    });

    expect(r.useSelection).toBe(false);
    expect(r.separator).toBe('\n\n');
    expect(r.textToInsert).toBe('\n\nnovo');
  });

  it('não duplica separador quando já termina com \n\n', () => {
    const r = computeMonacoInsertText({
      hasFocus: false,
      selectionIsEmpty: true,
      selectionStart: { lineNumber: 1, column: 1 },
      currentText: 'abc\n\n',
      content: 'novo',
    });

    expect(r.useSelection).toBe(false);
    expect(r.separator).toBe('');
    expect(r.textToInsert).toBe('novo');
  });

  it('não adiciona separador quando o documento está vazio/só espaços', () => {
    const r = computeMonacoInsertText({
      hasFocus: false,
      selectionIsEmpty: true,
      selectionStart: { lineNumber: 1, column: 1 },
      currentText: '   \n',
      content: 'novo',
    });

    expect(r.useSelection).toBe(false);
    expect(r.separator).toBe('');
    expect(r.textToInsert).toBe('novo');
  });

  it('com foco => usa seleção (sem separador)', () => {
    const r = computeMonacoInsertText({
      hasFocus: true,
      selectionIsEmpty: true,
      selectionStart: { lineNumber: 1, column: 1 },
      currentText: 'abc',
      content: 'novo',
    });

    expect(r.useSelection).toBe(true);
    expect(r.separator).toBe('');
  });

  it('seleção não vazia => usa seleção (sem separador)', () => {
    const r = computeMonacoInsertText({
      hasFocus: false,
      selectionIsEmpty: false,
      selectionStart: { lineNumber: 1, column: 1 },
      currentText: 'abc',
      content: 'novo',
    });

    expect(r.useSelection).toBe(true);
    expect(r.separator).toBe('');
  });

  it('cursor fora de (1,1) mesmo sem foco => usa seleção', () => {
    const r = computeMonacoInsertText({
      hasFocus: false,
      selectionIsEmpty: true,
      selectionStart: { lineNumber: 3, column: 5 },
      currentText: 'abc',
      content: 'novo',
    });

    expect(r.useSelection).toBe(true);
  });
});
