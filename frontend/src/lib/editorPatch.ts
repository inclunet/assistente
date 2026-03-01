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

  let parsed: any;
  try {
    parsed = JSON.parse(jsonText);
  } catch (e: any) {
    return { ok: false, error: `JSON inválido no patch: ${e?.message || String(e)}` };
  }

  if (parsed?.v !== 1) return { ok: false, error: 'Patch inválido: campo v deve ser 1' };
  if (parsed?.op !== 'replace_selection') return { ok: false, error: 'Patch inválido: op deve ser replace_selection' };
  if (parsed?.format !== 'markdown' && parsed?.format !== 'plain') {
    return { ok: false, error: 'Patch inválido: format deve ser markdown ou plain' };
  }
  if (typeof parsed?.replacement !== 'string') {
    return { ok: false, error: 'Patch inválido: replacement deve ser string' };
  }

  return { ok: true, patch: parsed as EditorPatchV1 };
}

export function buildEditorPatchPrompt(params: {
  instruction: string;
  selectedText: string;
  format?: 'markdown' | 'plain';
  selectionIsEmpty?: boolean;
  cursorContext?: string;
}): string {
  const { instruction, selectedText, format = 'markdown', selectionIsEmpty, cursorContext } = params;

  const selectedFence = format === 'markdown' ? 'markdown' : 'text';

  if (selectionIsEmpty) {
    return [
      instruction.trim(),
      '',
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
    'Trecho selecionado:',
    '```' + selectedFence,
    selectedText.trimEnd(),
    '```',
  ].join('\n');
}
