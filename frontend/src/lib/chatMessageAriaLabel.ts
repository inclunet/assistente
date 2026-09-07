import { stripMarkdown } from './stripMarkdown';

export type ChatMessageAriaLabelArgs = {
  roleLabel: string;
  role: string;
  displayContent: string;
  isStreaming: boolean;

  timePrefix: string;
  relativeTime: string;

  isReasoningExpanded: boolean;
  reasoning?: string | null;
  streamingReasoning?: string | null;

  toolCallsRaw?: string | null;
  toolCallsHasTextEdit?: boolean;

  /** Rótulo i18n falado no lugar de blocos de código. */
  codeBlockLabel?: string;
};

const parseToolNames = (raw?: string | null): string[] => {
  const s = typeof raw === 'string' ? raw.trim() : '';
  if (!s) return [];

  try {
    const parsed = JSON.parse(s);
    const calls = Array.isArray(parsed) ? parsed : [parsed];
    return calls
      .map((c) => {
        const call = c as { function?: { name?: unknown }; name?: unknown };
        return call.function?.name || call.name;
      })
      .filter(Boolean)
      .map((n) => String(n));
  } catch {
    return [];
  }
};

export function buildChatMessageAriaLabel(args: ChatMessageAriaLabelArgs): string {
  const stripOptions = args.codeBlockLabel ? { codeBlockLabel: args.codeBlockLabel } : undefined;
  const preview = args.displayContent ? stripMarkdown(args.displayContent, stripOptions).trim() : '';

  let contentPreview = preview;
  if (!contentPreview) {
    if (args.isStreaming) {
      contentPreview = 'Respondendo...';
    } else {
      const toolNames = parseToolNames(args.toolCallsRaw);
      if (toolNames.length > 0) {
        contentPreview = `Executou ferramenta${toolNames.length > 1 ? 's' : ''}: ${toolNames.join(', ')}`;
      } else if (args.toolCallsHasTextEdit) {
        contentPreview = 'Aplicou uma alteração no texto via ferramenta.';
      } else {
        contentPreview = 'Sem conteúdo textual.';
      }
    }
  }

  const reasoningText = (args.reasoning || args.streamingReasoning || '').trim();
  const reasoningLabel = args.isReasoningExpanded && reasoningText
    ? ` Raciocínio: ${stripMarkdown(reasoningText, stripOptions)}.`
    : '';

  const playHint = args.role === 'assistant' && !args.isStreaming
    ? ' Pressione Espaço para reproduzir áudio.'
    : '';

  return `${args.roleLabel}: ${contentPreview}.${reasoningLabel} ${args.timePrefix} ${args.relativeTime}.${playHint}`;
}
