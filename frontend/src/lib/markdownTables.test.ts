import { describe, expect, it } from 'vitest';
import { looksLikeGfmPipeTable } from './markdownTables';

describe('looksLikeGfmPipeTable', () => {
  it('detecta uma tabela pipe simples', () => {
    const md = ['| A | B |', '| --- | --- |', '| 1 | 2 |'].join('\n');
    expect(looksLikeGfmPipeTable(md)).toBe(true);
  });

  it('aceita alinhamento', () => {
    const md = ['| A | B |', '| :--- | ---: |', '| 1 | 2 |'].join('\n');
    expect(looksLikeGfmPipeTable(md)).toBe(true);
  });

  it('não marca texto comum com pipes como tabela', () => {
    expect(looksLikeGfmPipeTable('isso | não | é | tabela')).toBe(false);
    expect(looksLikeGfmPipeTable('| só uma linha |')).toBe(false);
  });
});
