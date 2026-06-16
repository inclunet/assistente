import { describe, it, expect, vi } from 'vitest';
import type { TFunction } from 'i18next';
import {
  normalizeDiskInfo,
  diskInfoEquals,
  hashStringFNV1a32,
  hasConflictMarkers,
  makeGitStyleConflictText,
  safeDraftIdPart,
  buildUnifiedDiff,
  truncatePreview,
  composePreviewText,
} from './editorMergeUtils';

describe('editorMergeUtils', () => {
  describe('normalizeDiskInfo', () => {
    it('normaliza valores ausentes para defaults seguros', () => {
      expect(normalizeDiskInfo(null)).toEqual({ exists: false, isDir: false, size: 0, modTimeMs: 0 });
    });

    it('preserva os campos fornecidos pelo backend', () => {
      const info = { path: 'a.md', exists: true, isDir: false, size: 12, modTimeMs: 99 } as never;
      expect(normalizeDiskInfo(info)).toEqual({ exists: true, isDir: false, size: 12, modTimeMs: 99 });
    });
  });

  describe('diskInfoEquals', () => {
    const base = { exists: true, isDir: false, size: 10, modTimeMs: 5 };

    it('retorna false quando algum lado é nulo', () => {
      expect(diskInfoEquals(null, base)).toBe(false);
      expect(diskInfoEquals(base, null)).toBe(false);
    });

    it('compara campo a campo', () => {
      expect(diskInfoEquals(base, { ...base })).toBe(true);
      expect(diskInfoEquals(base, { ...base, size: 11 })).toBe(false);
      expect(diskInfoEquals(base, { ...base, modTimeMs: 6 })).toBe(false);
    });
  });

  describe('hashStringFNV1a32', () => {
    it('é determinístico e sensível a mudanças', () => {
      expect(hashStringFNV1a32('abc')).toBe(hashStringFNV1a32('abc'));
      expect(hashStringFNV1a32('abc')).not.toBe(hashStringFNV1a32('abd'));
    });

    it('trata entradas vazias/nulas sem lançar', () => {
      expect(typeof hashStringFNV1a32('')).toBe('number');
    });
  });

  describe('hasConflictMarkers', () => {
    it('detecta marcadores de conflito no estilo Git', () => {
      const text = makeGitStyleConflictText('disco', 'minha');
      expect(hasConflictMarkers(text)).toBe(true);
    });

    it('retorna false para texto comum', () => {
      expect(hasConflictMarkers('texto normal\nsem conflito')).toBe(false);
    });
  });

  describe('makeGitStyleConflictText', () => {
    it('usa labels padrão quando não informados', () => {
      const text = makeGitStyleConflictText('A', 'B');
      expect(text).toContain('<<<<<<< disco');
      expect(text).toContain('=======');
      expect(text).toContain('>>>>>>> minha');
      expect(text).toContain('A');
      expect(text).toContain('B');
    });

    it('respeita labels customizados', () => {
      const text = makeGitStyleConflictText('A', 'B', { disk: 'remoto', local: 'local' });
      expect(text).toContain('<<<<<<< remoto');
      expect(text).toContain('>>>>>>> local');
    });
  });

  describe('safeDraftIdPart', () => {
    it('substitui caracteres inválidos e limita o tamanho', () => {
      expect(safeDraftIdPart('a/b c.md')).toBe('a_b_c_md');
    });

    it('retorna fallback quando vazio', () => {
      expect(safeDraftIdPart('')).toBe('tab');
      expect(safeDraftIdPart('   ')).toBe('tab');
    });
  });

  describe('buildUnifiedDiff', () => {
    it('gera diff entre dois conteúdos diferentes', () => {
      const diff = buildUnifiedDiff('linha 1\n', 'linha 2\n');
      expect(diff).toContain('disco');
      expect(diff).toContain('minha-versao');
    });
  });

  describe('truncatePreview', () => {
    it('mantém textos curtos intactos', () => {
      const res = truncatePreview('curto', 100);
      expect(res).toEqual({ preview: 'curto', truncated: false, total: 5 });
    });

    it('trunca textos longos e marca truncated sem compor sufixo user-facing', () => {
      const long = 'x'.repeat(50);
      const res = truncatePreview(long, 10);
      expect(res.truncated).toBe(true);
      expect(res.total).toBe(50);
      expect(res.preview).toBe('xxxxxxxxxx');
      expect(res.preview).not.toContain('truncado');
    });
  });

  describe('composePreviewText', () => {
    // Mock de TFunction: ecoa a chave + os args interpolados.
    const t = ((key: string, opts?: Record<string, unknown>) =>
      opts ? `${key}|${JSON.stringify(opts)}` : key) as unknown as TFunction;

    it('retorna o texto intacto, sem sufixo, quando não trunca', () => {
      expect(composePreviewText('curto', t, 100)).toBe('curto');
    });

    it('anexa o sufixo traduzido com o total quando trunca', () => {
      const long = 'x'.repeat(50);
      const res = composePreviewText(long, t, 10);
      expect(res).toBe('xxxxxxxxxx' + 'editor.preview.truncatedSuffix|{"total":50}');
    });

    it('usa o limite padrão quando nenhum limite é informado', () => {
      const res = composePreviewText('texto pequeno', t);
      expect(res).toBe('texto pequeno');
    });

    it('só chama t quando o texto é truncado', () => {
      const spy = vi.fn((key: string) => key) as unknown as TFunction;
      composePreviewText('curto', spy, 100);
      expect(spy).not.toHaveBeenCalled();

      composePreviewText('x'.repeat(20), spy, 5);
      expect(spy).toHaveBeenCalledWith('editor.preview.truncatedSuffix', { total: 20 });
    });
  });
});
