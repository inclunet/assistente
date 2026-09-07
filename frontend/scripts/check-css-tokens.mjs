#!/usr/bin/env node
// @ts-check
/**
 * Validação de tokens CSS (AEP / CLAUDE.md — design tokens em theme.css).
 *
 * Garante que toda referência `var(--token)` SEM fallback aponte para um token
 * realmente declarado:
 *   1. em `src/theme.css` (tokens canônicos de design), ou
 *   2. como custom property local em algum CSS (hooks de override de componente), ou
 *   3. injetada via estilo inline em arquivos .ts/.tsx (style={{ '--x': ... }}).
 *
 * Referências como `var(--x, var(--bg-elevated))` são consideradas seguras porque
 * possuem fallback declarado. O objetivo é pegar referências "penduradas" a tokens
 * inexistentes (ex.: `--color-error`, `--color-accent`) que o Stylelint não detecta.
 *
 * Uso: node scripts/check-css-tokens.mjs
 * Sai com código 1 se encontrar referências inválidas.
 */

import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = fileURLToPath(new URL('.', import.meta.url));
const ROOT = join(__dirname, '..');
const SRC = join(ROOT, 'src');
const THEME = join(SRC, 'theme.css');

/** Percorre recursivamente `dir` retornando arquivos que casam com `exts`. */
function walk(dir, exts) {
  /** @type {string[]} */
  const out = [];
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules') continue;
    const full = join(dir, entry);
    const st = statSync(full);
    if (st.isDirectory()) {
      out.push(...walk(full, exts));
    } else if (exts.some((e) => entry.endsWith(e))) {
      out.push(full);
    }
  }
  return out;
}

/**
 * Extrai nomes de custom properties declaradas (`--x: ...`) de um texto CSS.
 * Uma declaração é um nome `--token` seguido de `:`. Usos via `var(--token)` não
 * casam, pois ali o token é seguido de `)` ou `,` (nunca `:`).
 */
function collectDeclaredTokens(css, into) {
  const re = /(--[A-Za-z0-9-]+)\s*:/g;
  let m;
  while ((m = re.exec(css)) !== null) {
    into.add(m[1]);
  }
}

/** Extrai custom properties definidas inline em TS/TSX (`'--x': ...` ou `"--x": ...`). */
function collectInlineTokens(code, into) {
  const re = /['"](--[A-Za-z0-9-]+)['"]\s*:/g;
  let m;
  while ((m = re.exec(code)) !== null) {
    into.add(m[1]);
  }
}

/**
 * Percorre as referências `var(...)` de um CSS.
 * Para cada uma, identifica o token e se há fallback (vírgula no nível do próprio var()).
 * @returns {{token: string, hasFallback: boolean, index: number}[]}
 */
function parseVarRefs(css) {
  /** @type {{token: string, hasFallback: boolean, index: number}[]} */
  const refs = [];
  const re = /var\(\s*(--[A-Za-z0-9-]+)/g;
  let m;
  while ((m = re.exec(css)) !== null) {
    const token = m[1];
    // Procura, a partir do fim do token, se existe vírgula antes do `)` que fecha
    // ESTE var(), respeitando aninhamento de parênteses.
    let depth = 1; // já estamos dentro do `var(`
    let hasFallback = false;
    for (let i = m.index + m[0].length; i < css.length; i++) {
      const ch = css[i];
      if (ch === '(') depth++;
      else if (ch === ')') {
        depth--;
        if (depth === 0) break;
      } else if (ch === ',' && depth === 1) {
        hasFallback = true;
        break;
      }
    }
    refs.push({ token, hasFallback, index: m.index });
  }
  return refs;
}

/** Converte um índice de caractere em "linha:coluna" (1-based). */
function lineCol(text, index) {
  let line = 1;
  let col = 1;
  for (let i = 0; i < index; i++) {
    if (text[i] === '\n') {
      line++;
      col = 1;
    } else {
      col++;
    }
  }
  return { line, col };
}

function main() {
  const themeCss = readFileSync(THEME, 'utf8');

  /** Tokens declarados (theme + locais + inline). */
  const declared = new Set();
  collectDeclaredTokens(themeCss, declared);

  const cssFiles = walk(SRC, ['.css']);
  const codeFiles = walk(SRC, ['.ts', '.tsx']);

  for (const f of cssFiles) collectDeclaredTokens(readFileSync(f, 'utf8'), declared);
  for (const f of codeFiles) collectInlineTokens(readFileSync(f, 'utf8'), declared);

  /** @type {{file: string, line: number, col: number, token: string}[]} */
  const violations = [];

  for (const f of cssFiles) {
    const css = readFileSync(f, 'utf8');
    for (const ref of parseVarRefs(css)) {
      if (ref.hasFallback) continue; // fallback torna a referência segura
      if (declared.has(ref.token)) continue;
      const { line, col } = lineCol(css, ref.index);
      violations.push({ file: relative(ROOT, f).split(sep).join('/'), line, col, token: ref.token });
    }
  }

  if (violations.length === 0) {
    console.log(`✔ check-css-tokens: ${cssFiles.length} arquivos CSS, nenhum token inexistente referenciado.`);
    process.exit(0);
  }

  console.error(`✖ check-css-tokens: ${violations.length} referência(s) a token inexistente (sem fallback):\n`);
  for (const v of violations) {
    console.error(`  ${v.file}:${v.line}:${v.col}  var(${v.token}) não está declarado em theme.css nem localmente`);
  }
  console.error(
    '\nCorrija usando um token existente de src/theme.css (ex.: --accent, --bg-hover, --color-danger),\n' +
      'declarando o token, ou adicionando um fallback explícito var(--x, <token-existente>).'
  );
  process.exit(1);
}

main();
