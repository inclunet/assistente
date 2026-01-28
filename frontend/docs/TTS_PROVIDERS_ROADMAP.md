# TTS Providers - Roadmap de Implementação

## Status Atual (React Migration)

Atualmente, apenas o **WebSpeech API** está implementado no frontend React. Este provider usa as vozes nativas do navegador.

### Implementação Atual
- **Arquivo**: `frontend/src/services/tts/index.ts`
- **Provider**: WebSpeech API apenas
- **Vozes**: Obtidas via `speechSynthesis.getVoices()`
- **Limitações**: 
  - Apenas vozes instaladas no sistema operacional
  - Qualidade varia por sistema
  - Sem vozes premium/cloud

## Arquitetura Original (Svelte)

A versão Svelte suportava **3 providers** de TTS:

### 1. WebSpeech API (Navegador)
- **Tipo**: Client-side, nativo do navegador
- **Vozes**: Sistema operacional
- **Função Backend**: Nenhuma
- **Custo**: Gratuito
- **Qualidade**: Básica/Boa (depende do SO)

### 2. SAPI5 (Windows TTS)
- **Tipo**: Via backend Wails
- **Vozes**: Windows SAPI5 voices (Microsoft David, Zira, etc)
- **Funções Backend**:
  - `SpeakSAPI5(text, voice)` - Fala texto
  - `StopSAPI5()` - Para a fala
  - `SetSAPI5Volume(volume)` - Define volume (0-100)
  - `SetSAPI5Rate(rate)` - Define velocidade (-10 a 10)
  - `GetSAPI5Voices()` - Lista vozes disponíveis
- **Custo**: Gratuito (usa vozes do Windows)
- **Qualidade**: Boa
- **Plataforma**: Apenas Windows

### 3. OpenAI TTS
- **Tipo**: Cloud API via backend
- **Vozes**: OpenAI voices (alloy, echo, fable, onyx, nova, shimmer)
- **Funções Backend**:
  - `SynthesizeOpenAIWithVoice(text, voice, speed)` - Gera áudio
  - `SetOpenAITTSSpeed(speed)` - Define velocidade
- **Custo**: Pago (via créditos OpenAI)
- **Qualidade**: Excelente (vozes neurais premium)
- **Plataforma**: Multiplataforma

## Arquitetura de Providers

### Estrutura de Dados

```typescript
// Provider types
export enum TTSProvider {
  DISABLED = 'disabled',
  WEBSPEECH = 'webspeech',
  SAPI5 = 'sapi5',
  OPENAI = 'openai'
}

// Voice metadata
export interface TTSVoice {
  id: string;           // Identificador único
  name: string;         // Nome para exibição
  language: string;     // Código do idioma (pt-BR, en-US, etc)
  provider: TTSProvider;// Provider que fornece a voz
  gender?: 'male' | 'female' | 'neutral';
  premium?: boolean;    // Se é uma voz premium (custo adicional)
  localService?: boolean; // Se é processada localmente
}
```

### Interface de Provider

Cada provider deve implementar:

```typescript
interface ITTSProvider {
  // Identificação
  readonly name: TTSProvider;
  readonly isAvailable: boolean;
  
  // Inicialização
  initialize(): Promise<void>;
  
  // Vozes
  getVoices(): Promise<TTSVoice[]>;
  setVoice(voiceId: string): void;
  
  // Controles
  speak(text: string): Promise<void>;
  stop(): void;
  pause(): void;
  resume(): void;
  
  // Parâmetros
  setVolume(volume: number): void;  // 0-100
  setRate(rate: number): void;      // -10 a 10
  setPitch(pitch: number): void;    // 0-2 (apenas WebSpeech)
  
  // Estado
  isSpeaking(): boolean;
  
  // Eventos
  on(event: string, callback: Function): void;
  off(event: string, callback: Function): void;
}
```

## Roadmap de Implementação

### Fase 1: Refatoração da Arquitetura ✅ (Parcial)
- [x] Criar serviço base `TTSService`
- [x] Implementar `WebSpeechProvider`
- [ ] Abstrair interface de provider
- [ ] Criar factory de providers
- [ ] Sistema de fallback entre providers

### Fase 2: Backend - SAPI5
1. **Backend Go**:
   - Criar funções Wails para SAPI5
   - `GetSAPI5Voices() []Voice`
   - `SpeakSAPI5(text, voice string) error`
   - `StopSAPI5() error`
   - `SetSAPI5Volume(volume int) error`
   - `SetSAPI5Rate(rate int) error`

2. **Frontend**:
   - Criar `SAPI5Provider` implementando `ITTSProvider`
   - Integrar com `TTSService`
   - Atualizar `VoicePicker` para listar vozes SAPI5

### Fase 3: Backend - OpenAI TTS
1. **Backend Go**:
   - Integrar OpenAI TTS API
   - `GetOpenAIVoices() []Voice`
   - `SynthesizeOpenAIWithVoice(text, voice, speed string) ([]byte, error)`
   - `SetOpenAITTSSpeed(speed int) error`

2. **Frontend**:
   - Criar `OpenAIProvider` implementando `ITTSProvider`
   - Gerenciar reprodução de áudio (Audio API)
   - Integrar com `TTSService`
   - Atualizar `VoicePicker` para listar vozes OpenAI

### Fase 4: UI/UX
1. **VoicePicker Melhorado**:
   - Agrupar vozes por provider
   - Indicar vozes premium (💎)
   - Indicar vozes locais (🏠)
   - Filtrar por idioma
   - Preview de voz

2. **Settings TTS**:
   - Seletor de provider padrão
   - Configurações específicas por provider
   - Fallback automático
   - Indicador de custo (para OpenAI)

## Estrutura de Arquivos Proposta

```
frontend/src/services/tts/
├── index.ts                 # TTSService (orquestrador)
├── types.ts                 # Interfaces e tipos
├── providers/
│   ├── base.ts             # Interface ITTSProvider
│   ├── webSpeech.ts        # WebSpeechProvider
│   ├── sapi5.ts            # SAPI5Provider
│   └── openai.ts           # OpenAIProvider
└── factory.ts              # Factory de providers
```

## Exemplo de Uso (Futuro)

```typescript
import { ttsService, TTSProvider } from '@/services/tts';

// Listar providers disponíveis
const providers = await ttsService.getAvailableProviders();
// ['webspeech', 'sapi5', 'openai']

// Selecionar provider
await ttsService.setProvider(TTSProvider.OPENAI);

// Listar vozes do provider atual
const voices = await ttsService.getVoices();
// [
//   { id: 'alloy', name: 'Alloy', language: 'pt-BR', provider: 'openai', premium: true },
//   { id: 'echo', name: 'Echo', language: 'pt-BR', provider: 'openai', premium: true },
//   ...
// ]

// Selecionar voz
ttsService.setVoice('alloy');

// Falar (usa provider configurado)
await ttsService.speak('Olá, mundo!');

// Fallback automático se provider falhar
// openai → sapi5 → webspeech
```

## Configuração de Fallback

```typescript
// Ordem de preferência de providers
const fallbackOrder = [
  TTSProvider.OPENAI,   // Tenta primeiro (melhor qualidade)
  TTSProvider.SAPI5,    // Fallback 1 (se Windows)
  TTSProvider.WEBSPEECH // Fallback 2 (sempre disponível)
];
```

## Considerações de Implementação

### Performance
- **WebSpeech**: Instantâneo, local
- **SAPI5**: Instantâneo, local (só Windows)
- **OpenAI**: Network latency (~500ms-2s), requer streaming para responsividade

### Custos
- **WebSpeech**: Gratuito
- **SAPI5**: Gratuito
- **OpenAI**: ~$15 por 1 milhão de caracteres

### Qualidade
- **WebSpeech**: Básica (varia por SO)
- **SAPI5**: Boa (vozes Microsoft)
- **OpenAI**: Excelente (vozes neurais premium)

### Disponibilidade
- **WebSpeech**: ✅ Todos os navegadores modernos
- **SAPI5**: ✅ Windows apenas
- **OpenAI**: ✅ Qualquer plataforma (requer internet e API key)

## Próximos Passos Imediatos

1. ✅ Corrigir `VoicePicker` para mostrar apenas vozes reais (WebSpeech)
2. ⏳ Criar interface abstrata `ITTSProvider`
3. ⏳ Refatorar `webSpeechTTS.ts` para implementar `ITTSProvider`
4. ⏳ Criar factory de providers
5. ⏳ Adicionar seletor de provider no settings
6. ⏳ Implementar SAPI5 (backend + frontend)
7. ⏳ Implementar OpenAI TTS (backend + frontend)

## Referências

- Código original Svelte: `frontend-svelte-backup/src/lib/speech/tts-service.js`
- OpenAI TTS API: https://platform.openai.com/docs/guides/text-to-speech
- Web Speech API: https://developer.mozilla.org/en-US/docs/Web/API/Web_Speech_API
- Windows SAPI5: https://docs.microsoft.com/en-us/previous-versions/windows/desktop/ms723627(v=vs.85)
