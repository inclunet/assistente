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

  /** Rótulos já localizados pela mesma camada de apresentação dos cards. */
  toolLabels?: string[];
  toolCallsHasTextEdit?: boolean;

  /** Rótulo i18n falado no lugar de blocos de código. */
  codeBlockLabel?: string;
};

export function buildChatMessageAriaLabel(args: ChatMessageAriaLabelArgs): string {
  const stripOptions = args.codeBlockLabel ? { codeBlockLabel: args.codeBlockLabel } : undefined;
  const preview = args.displayContent ? stripMarkdown(args.displayContent, stripOptions).trim() : '';

  let contentPreview = preview;
  if (!contentPreview) {
    if (args.isStreaming) {
      contentPreview = 'Respondendo...';
    } else {
      if (args.toolCallsHasTextEdit) {
        contentPreview = 'Aplicou uma alteração no texto via ferramenta.';
      } else {
        const toolLabels = args.toolLabels ?? [];
        if (toolLabels.length > 0) {
          contentPreview = toolLabels.join('. ');
        } else {
          contentPreview = 'Sem conteúdo textual.';
        }
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
