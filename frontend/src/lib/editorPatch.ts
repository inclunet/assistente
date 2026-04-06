export type EditorPatchV1 = {
  v: 1;
  op: 'replace_selection';
  format: 'markdown' | 'plain';
  replacement: string;
  notes?: string;
};

export type ExtractEditorPatchResult =
  | { ok: true; patch: EditorPatchV1 }
  | { ok: false; error: string };

const MAX_EDITOR_PATCH_CHARS = 200 * 1024; // ~200KB (aprox em chars)
const MAX_EDITOR_REPLACEMENT_CHARS = 200 * 1024; // mantém UI responsiva

export function extractEditorPatch(text: string): ExtractEditorPatchResult {
  // Preferir formato Markdown (bloco de código) por ser mais compatível.
  // Aceita: ```editor_patch\n{...}\n```
  const fenceMatch = text.match(/```\s*(editor_patch|editor-patch|assistente_editor_patch)\s*\r?\n([\s\S]*?)\r?\n```/i);

  const jsonText = (fenceMatch?.[2] ?? '').trim();
  if (!jsonText) {
    if (!fenceMatch) {
      return { ok: false, error: 'Resposta não contém patch (bloco ```editor_patch```)' };
    }
    return { ok: false, error: 'Patch vazio' };
  }

  if (jsonText.length > MAX_EDITOR_PATCH_CHARS) {
    return {
      ok: false,
      error: `Patch muito grande para aplicar com segurança (limite: ${MAX_EDITOR_PATCH_CHARS} caracteres).`,
    };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(jsonText);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    return { ok: false, error: `JSON inválido no patch: ${message}` };
  }

  const patch = typeof parsed === 'object' && parsed !== null ? (parsed as Partial<EditorPatchV1>) : {};

  if (patch.v !== 1) return { ok: false, error: 'Patch inválido: campo v deve ser 1' };
  if (patch.op !== 'replace_selection') return { ok: false, error: 'Patch inválido: op deve ser replace_selection' };
  if (patch.format !== 'markdown' && patch.format !== 'plain') {
    return { ok: false, error: 'Patch inválido: format deve ser markdown ou plain' };
  }
  if (typeof patch.replacement !== 'string') {
    return { ok: false, error: 'Patch inválido: replacement deve ser string' };
  }

  if (String(patch.replacement).length > MAX_EDITOR_REPLACEMENT_CHARS) {
    return {
      ok: false,
      error: `replacement muito grande para aplicar com segurança (limite: ${MAX_EDITOR_REPLACEMENT_CHARS} caracteres).`,
    };
  }

  return { ok: true, patch: patch as EditorPatchV1 };
}

export function buildEditorPatchPrompt(params: {
  instruction: string;
  selectedText: string;
  format?: 'markdown' | 'plain';
  selectionIsEmpty?: boolean;
  cursorContext?: string;
  filePath?: string;
}): string {
  const { instruction, selectedText, format = 'markdown', selectionIsEmpty, cursorContext, filePath } = params;

  const selectedFence = format === 'markdown' ? 'markdown' : 'text';
  const fileHeader = filePath ? [`Arquivo ativo: \`${filePath}\``, ''] : [];

  if (selectionIsEmpty) {
    return [
      instruction.trim(),
      '',
      ...fileHeader,
      'IMPORTANTE: você NÃO recebeu um trecho selecionado.',
      'Faça uma INSERÇÃO no cursor atual (não peça para o usuário copiar/colar nada).',
      'Contexto ao redor do cursor (⟂ = cursor):',
      '```' + selectedFence,
      String(cursorContext || '').trimEnd(),
      '```',
    ].join('\n');
  }

  return [
    instruction.trim(),
    '',
    ...fileHeader,
    'Trecho selecionado:',
    '```' + selectedFence,
    selectedText.trimEnd(),
    '```',
  ].join('\n');
}
