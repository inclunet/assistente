import { describe, expect, it, vi } from 'vitest';
import { GetOpenAITTSVoices } from '@wailsjs/go/wailsapi/Speech';
import { OpenAIProvider } from './openai';

vi.mock('@wailsjs/go/wailsapi/Speech', () => ({ GetOpenAITTSVoices: vi.fn(async () => []) }));

describe('transporte HTTP TTS', () => {
  it('permanece disponível sem vozes catalogadas e não sonda na inicialização', async () => {
    const provider = new OpenAIProvider();
    await provider.initialize();
    expect(GetOpenAITTSVoices).not.toHaveBeenCalled();
    expect(provider.isAvailable).toBe(true);
    expect(await provider.getVoices()).toEqual([]);
    expect(provider.isAvailable).toBe(true);
  });
});
