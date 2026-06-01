import { describe, expect, it } from 'vitest';
import {
  assertContrastTokenPairAllowed,
  isContrastTokenPairAllowed,
} from './a11yHelpers';

/*
 * Demonstração do helper de PAR DE TOKENS DE CONTRASTE.
 *
 * axe-core/jsdom não resolve `var(--token)`, então pares texto/fundo errados
 * (mas com cor não-hardcoded) passam batido. Este helper consulta o contrato
 * `texto → fundos permitidos`. Sem scanning de CSS: apenas o helper + casos.
 */
describe('contraste por token — isContrastTokenPairAllowed', () => {
  it('--text-inverse sobre --bg-overlay é INVÁLIDO (regressão real do PR #141/#143)', () => {
    expect(isContrastTokenPairAllowed('--text-inverse', '--bg-overlay')).toBe(false);
  });

  it('--text-inverse sobre --accent é válido', () => {
    expect(isContrastTokenPairAllowed('--text-inverse', '--accent')).toBe(true);
    expect(isContrastTokenPairAllowed('--text-inverse', '--accent-hover')).toBe(true);
  });

  it('--text-primary/secondary/muted sobre superfícies é válido', () => {
    expect(isContrastTokenPairAllowed('--text-primary', '--bg-surface')).toBe(true);
    expect(isContrastTokenPairAllowed('--text-secondary', '--bg-elevated')).toBe(true);
    expect(isContrastTokenPairAllowed('--text-muted', '--bg-base')).toBe(true);
  });

  it('--text-primary sobre --accent é INVÁLIDO (texto de superfície não vai sobre acento sólido)', () => {
    expect(isContrastTokenPairAllowed('--text-primary', '--accent')).toBe(false);
  });

  it('token de texto desconhecido retorna false', () => {
    expect(isContrastTokenPairAllowed('--text-inexistente', '--bg-surface')).toBe(false);
  });

  it('aceita tokens com ou sem o prefixo "--"', () => {
    expect(isContrastTokenPairAllowed('text-inverse', 'accent')).toBe(true);
  });
});

describe('contraste por token — assertContrastTokenPairAllowed', () => {
  it('não lança para par permitido', () => {
    expect(() => assertContrastTokenPairAllowed('--text-inverse', '--accent')).not.toThrow();
  });

  it('lança para par não permitido', () => {
    expect(() => assertContrastTokenPairAllowed('--text-inverse', '--bg-overlay')).toThrow();
  });
});
