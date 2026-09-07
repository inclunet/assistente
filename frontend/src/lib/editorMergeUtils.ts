import { createTwoFilesPatch } from 'diff';
import type { TFunction } from 'i18next';
import type { apidto } from '@wailsjs/go/models';

/** Metadados de disco usados para detectar mudanças externas em um arquivo. */
export type DiskInfo = { exists: boolean; isDir: boolean; size: number; modTimeMs: number };

/** Recorte de preview de texto com indicação de truncamento. */
export type TextPreview = { preview: string; truncated: boolean; total: number };

/** Normaliza o retorno do backend (`EditorFileInfo`) para o formato `DiskInfo`. */
export function normalizeDiskInfo(info: apidto.EditorFileInfo | null | undefined): DiskInfo {
  return {
    exists: !!info?.exists,
    isDir: !!info?.isDir,
    size: Number(info?.size ?? 0),
    modTimeMs: Number(info?.modTimeMs ?? 0),
  };
}

/** Compara dois `DiskInfo` campo a campo (null/undefined nunca são iguais). */
export function diskInfoEquals(a?: DiskInfo | null, b?: DiskInfo | null): boolean {
  if (!a || !b) return false;
  return a.exists === b.exists && a.isDir === b.isDir && a.size === b.size && a.modTimeMs === b.modTimeMs;
}

/** Hash FNV-1a de 32 bits, estável e barato, usado para detectar mudança de conteúdo. */
export function hashStringFNV1a32(text: string): number {
  const s = String(text ?? '');
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = (h + ((h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24))) >>> 0;
  }
  return h >>> 0;
}

/** Detecta marcadores de conflito no estilo Git (`<<<<<<<`, `=======`, `>>>>>>>`). */
export function hasConflictMarkers(text: string): boolean {
  const s = String(text ?? '');
  return /^<{7} /m.test(s) || /^={7}$/m.test(s) || /^>{7} /m.test(s);
}

/** Monta um texto de conflito no estilo Git a partir do conteúdo de disco e local. */
export function makeGitStyleConflictText(
  diskContent: string,
  localContent: string,
  labels?: { disk?: string; local?: string }
): string {
  const diskLabel = String(labels?.disk || 'disco');
  const localLabel = String(labels?.local || 'minha');
  return [
    `<<<<<<< ${diskLabel}`,
    String(diskContent ?? ''),
    `=======`,
    String(localContent ?? ''),
    `>>>>>>> ${localLabel}`,
    '',
  ].join('\n');
}

/** Sanitiza um identificador para uso como parte de um draftId de merge. */
export function safeDraftIdPart(raw: string): string {
  return String(raw || '')
    .trim()
    .slice(0, 60)
    .replace(/[^a-zA-Z0-9_-]+/g, '_')
    .replace(/^_+/, '')
    .replace(/_+$/, '') || 'tab';
}

/** Gera um diff unificado entre o conteúdo de disco e o local (best-effort). */
export function buildUnifiedDiff(diskContent: string, localContent: string): string {
  try {
    return createTwoFilesPatch('disco', 'minha-versao', String(diskContent ?? ''), String(localContent ?? ''), '', '', {
      context: 3,
    });
  } catch {
    return '';
  }
}

/**
 * Trunca um texto para preview retornando apenas os dados (preview + flags).
 *
 * Não compõe nenhum texto user-facing: quando `truncated` é `true`, cabe ao
 * caller anexar um sufixo traduzido via i18n (ex.: `editor.preview.truncatedSuffix`).
 */
export function truncatePreview(text: string, limit = 20000): TextPreview {
  const s = String(text ?? '');
  if (s.length <= limit) return { preview: s, truncated: false, total: s.length };
  return {
    preview: s.slice(0, Math.max(0, limit)),
    truncated: true,
    total: s.length,
  };
}

/**
 * Compõe o texto user-facing de um preview: trunca via {@link truncatePreview}
 * e, quando truncado, anexa o sufixo traduzido `editor.preview.truncatedSuffix`.
 *
 * Recebe a função `t` (TFunction do i18next) como parâmetro, seguindo o padrão
 * dos demais utilitários deste módulo (data-only) e evitando importar o i18n
 * global. Centraliza a lógica antes duplicada entre `useEditorMerge` e
 * `EditorPage`.
 */
export function composePreviewText(text: string, t: TFunction, limit?: number): string {
  const p = limit === undefined ? truncatePreview(text) : truncatePreview(text, limit);
  return p.truncated ? p.preview + t('editor.preview.truncatedSuffix', { total: p.total }) : p.preview;
}
