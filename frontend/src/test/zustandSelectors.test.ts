import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * O zustand 5 entrega o seletor direto como `getSnapshot` do
 * `useSyncExternalStore`, sem memoizar o resultado. Um seletor que fabrica
 * valor novo a cada chamada (`?? []`, `.map(...)`, objeto literal) faz o React
 * enxergar snapshot diferente em todo commit e reagendar render até estourar
 * "Maximum update depth exceeded" — o app simplesmente não abre.
 *
 * Testes de componente costumam mockar o store e trocar o hook por uma chamada
 * síncrona, então não veem o defeito. Esta varredura vê.
 *
 * Para corrigir uma ocorrência apontada aqui: devolva uma referência estável
 * (constante de módulo para o caso vazio) ou embrulhe o seletor em
 * `useShallow`.
 */

// Roda tanto com o vitest lançado de `frontend/` quanto da raiz do repositório.
const SRC = existsSync(join(process.cwd(), 'src'))
  ? join(process.cwd(), 'src')
  : join(process.cwd(), 'frontend', 'src');

const PADROES_INSTAVEIS: Array<[RegExp, string]> = [
  [/\?\?\s*\[\]/, 'lista literal como padrão'],
  [/\?\?\s*\{\}/, 'objeto literal como padrão'],
  [/\|\|\s*\[\]/, 'lista literal como padrão'],
  [/\|\|\s*\{\}/, 'objeto literal como padrão'],
  [/\.(map|filter|slice|concat|sort|reverse)\(/, 'lista derivada'],
  [/Object\.(keys|values|entries|assign)\(/, 'coleção derivada'],
  [/new (Set|Map|Date)\(/, 'instância nova'],
  [/=>\s*\[/, 'lista literal'],
  [/=>\s*\(\{/, 'objeto literal'],
  [/return\s*[[{]/, 'valor literal'],
];

function arquivosFonte(dir: string, acc: string[] = []): string[] {
  for (const entrada of readdirSync(dir, { withFileTypes: true })) {
    const caminho = join(dir, entrada.name);
    if (entrada.isDirectory()) arquivosFonte(caminho, acc);
    else if (/\.tsx?$/.test(entrada.name) && !/\.test\.tsx?$/.test(entrada.name)) acc.push(caminho);
  }
  return acc;
}

/** Texto dos argumentos da chamada que começa no parêntese em `abre`. */
function argumentosDaChamada(texto: string, abre: number): string | null {
  let nivel = 0;
  for (let i = abre; i < texto.length; i++) {
    if (texto[i] === '(') nivel++;
    else if (texto[i] === ')') {
      nivel--;
      if (nivel === 0) return texto.slice(abre + 1, i);
    }
  }
  return null;
}

describe('seletores de store', () => {
  it('não fabricam valor novo a cada chamada', () => {
    const chamada = /\buse[A-Z][A-Za-z]*Store\s*\(/g;
    const achados: string[] = [];

    for (const arquivo of arquivosFonte(SRC)) {
      const texto = readFileSync(arquivo, 'utf8');
      chamada.lastIndex = 0;

      let encontro: RegExpExecArray | null;
      while ((encontro = chamada.exec(texto)) !== null) {
        const seletor = argumentosDaChamada(texto, encontro.index + encontro[0].length - 1)?.trim();
        if (!seletor || seletor.startsWith('useShallow')) continue;

        const motivo = PADROES_INSTAVEIS.find(([padrao]) => padrao.test(seletor))?.[1];
        if (!motivo) continue;

        const linha = texto.slice(0, encontro.index).split('\n').length;
        achados.push(`${relative(SRC, arquivo)}:${linha} (${motivo}) → ${seletor.replace(/\s+/g, ' ').slice(0, 120)}`);
      }
    }

    expect(achados).toEqual([]);
  });
});
