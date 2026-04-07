/**
 * Utilitários de áudio compartilhados.
 */

/**
 * Converte uma string base64 em Blob binário.
 * Usado por providers TTS, stream player e serviço de áudio de mensagens.
 */
export function base64ToBlob(base64: string, mimeType: string): Blob {
  const binaryString = atob(base64);
  const bytes = new Uint8Array(binaryString.length);
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }
  return new Blob([bytes], { type: mimeType });
}

/**
 * Converte uma string base64 em Uint8Array.
 * Útil quando o Blob não é necessário (ex: append a SourceBuffer).
 */
export function base64ToBytes(base64: string): Uint8Array {
  const binaryString = atob(base64);
  const bytes = new Uint8Array(binaryString.length);
  for (let i = 0; i < binaryString.length; i++) {
    bytes[i] = binaryString.charCodeAt(i);
  }
  return bytes;
}

/**
 * Calcula o timeout para operações TTS proporcionalmente ao tamanho do texto.
 * Equivalente ao CalcTTSTimeout do backend Go.
 */
export function calcTTSTimeoutMs(textLength: number): number {
  const TTS_MAX_CHUNK_SIZE = 4000;
  const BASE_TIMEOUT_MS = 60_000;
  const PER_CHUNK_TIMEOUT_MS = 30_000;
  const chunks = Math.floor(textLength / TTS_MAX_CHUNK_SIZE);
  return BASE_TIMEOUT_MS + chunks * PER_CHUNK_TIMEOUT_MS;
}
