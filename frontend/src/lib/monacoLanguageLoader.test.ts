import { describe, expect, it } from 'vitest';
import { loadMonacoLanguage } from './monacoLanguageLoader';

describe('monacoLanguageLoader', () => {
  it('ignora linguagem desconhecida sem erro', async () => {
    await expect(loadMonacoLanguage('unknown-lang')).resolves.toBeUndefined();
  });

  it('ignora plaintext', async () => {
    await expect(loadMonacoLanguage('plaintext')).resolves.toBeUndefined();
  });
});
