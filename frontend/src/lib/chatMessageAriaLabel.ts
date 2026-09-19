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
  localized: {
    responding: string;
    reasoning: string;
    textEditApplied: string;
    noTextContent: string;
    playAudioHint: string;
  };
};

export function buildChatMessageAriaLabel(args: ChatMessageAriaLabelArgs): string {
  const stripOptions = args.codeBlockLabel ? { codeBlockLabel: args.codeBlockLabel } : undefined;
  const preview = args.displayContent ? stripMarkdown(args.displayContent, stripOptions).trim() : '';

  let contentPreview = preview;
  if (!contentPreview) {
    if (args.isStreaming) {
      contentPreview = args.localized.responding;
    } else {
      if (args.toolCallsHasTextEdit) {
        contentPreview = args.localized.textEditApplied;
      } else {
        const toolLabels = args.toolLabels ?? [];
        if (toolLabels.length > 0) {
          contentPreview = toolLabels.join('. ');
        } else {
          contentPreview = args.localized.noTextContent;
        }
      }
    }
  }

  const reasoningText = (args.reasoning || args.streamingReasoning || '').trim();
  const reasoningLabel = args.isReasoningExpanded && reasoningText
    ? ` ${args.localized.reasoning}: ${stripMarkdown(reasoningText, stripOptions)}.`
    : '';

  const playHint = args.role === 'assistant' && !args.isStreaming
    ? ` ${args.localized.playAudioHint}`
    : '';

  return `${args.roleLabel}: ${contentPreview}.${reasoningLabel} ${args.timePrefix} ${args.relativeTime}.${playHint}`;
}
