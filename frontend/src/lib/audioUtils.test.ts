import { describe, it, expect } from 'vitest';
import { base64ToBlob, base64ToBytes, calcTTSTimeoutMs } from './audioUtils';

describe('base64ToBlob', () => {
  it('converte base64 em Blob com mimeType correto', () => {
    // "SGVsbG8=" é "Hello" em base64
    const blob = base64ToBlob('SGVsbG8=', 'audio/mpeg');
    expect(blob).toBeInstanceOf(Blob);
    expect(blob.type).toBe('audio/mpeg');
    expect(blob.size).toBe(5);
  });

  it('converte base64 vazio em Blob vazio', () => {
    const blob = base64ToBlob('', 'audio/mpeg');
    expect(blob.size).toBe(0);
  });

  it('preserva bytes binários corretamente', () => {
    // Verificar que bytes arbitrários passam pelo pipeline
    // Usamos base64ToBytes que é a mesma lógica interna
    const bytes = base64ToBytes('AP+A');
    expect(bytes[0]).toBe(0x00);
    expect(bytes[1]).toBe(0xFF);
    expect(bytes[2]).toBe(0x80);

    // E que base64ToBlob produz Blob do tamanho correto
    const blob = base64ToBlob('AP+A', 'application/octet-stream');
    expect(blob.size).toBe(3);
  });
});

describe('base64ToBytes', () => {
  it('converte base64 em Uint8Array', () => {
    const bytes = base64ToBytes('SGVsbG8=');
    expect(bytes).toBeInstanceOf(Uint8Array);
    expect(bytes.length).toBe(5);
    // "Hello" = [72, 101, 108, 108, 111]
    expect(bytes[0]).toBe(72);
    expect(bytes[1]).toBe(101);
    expect(bytes[4]).toBe(111);
  });

  it('retorna array vazio para base64 vazio', () => {
    const bytes = base64ToBytes('');
    expect(bytes.length).toBe(0);
  });
});

describe('calcTTSTimeoutMs', () => {
  it('retorna timeout base para textos curtos (< 4000 chars)', () => {
    expect(calcTTSTimeoutMs(0)).toBe(60_000);
    expect(calcTTSTimeoutMs(100)).toBe(60_000);
    expect(calcTTSTimeoutMs(3999)).toBe(60_000);
  });

  it('adiciona 30s por chunk de 4000 chars', () => {
    expect(calcTTSTimeoutMs(4000)).toBe(90_000);   // 60s + 30s * 1
    expect(calcTTSTimeoutMs(8000)).toBe(120_000);  // 60s + 30s * 2
    expect(calcTTSTimeoutMs(12000)).toBe(150_000); // 60s + 30s * 3
  });

  it('usa floor para chunks parciais', () => {
    expect(calcTTSTimeoutMs(5999)).toBe(90_000);  // floor(5999/4000) = 1
    expect(calcTTSTimeoutMs(7999)).toBe(90_000);  // floor(7999/4000) = 1
  });
});
