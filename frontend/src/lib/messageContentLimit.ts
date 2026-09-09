/**
 * Limite de texto em bytes UTF-8. Deve permanecer espelhado com
 * chat.MaxMessageContentSize em internal/chat/interactor.go.
 *
 * Mídia é validada separadamente e nunca entra nesta contagem.
 */
export const MAX_MESSAGE_CONTENT_BYTES = 512 * 1024;
export const MAX_MESSAGE_CONTENT_KIB = MAX_MESSAGE_CONTENT_BYTES / 1024;

const utf8Encoder = new TextEncoder();

export function getUtf8ByteLength(content: string): number {
  return utf8Encoder.encode(content).byteLength;
}
