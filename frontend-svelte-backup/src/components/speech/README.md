# Speech Components

Componentes de UI para configuração de voz (TTS/STT).

## Componentes

| Componente | Descrição |
|------------|-----------|
| `VoiceSettingsPanel` | Painel de configurações de síntese de voz |

## VoiceSettingsPanel

Painel com controles de volume, velocidade e opções de TTS.

### Uso Básico

```svelte
<script>
  import { VoiceSettingsPanel } from './components/speech';
  import { ttsService } from './lib/speech';
  
  let volume = 100;
  let rate = 0;
  let autoSpeak = true;
</script>

<VoiceSettingsPanel
  bind:volume
  bind:rate
  bind:autoSpeak
  selectedVoice={selectedVoice}
  voiceSource={voiceSource}
  on:volumeChange={({ detail }) => console.log('Volume:', detail.volume)}
  on:rateChange={({ detail }) => console.log('Rate:', detail.rate)}
  on:autoSpeakChange={({ detail }) => console.log('Auto:', detail.autoSpeak)}
/>
```

### Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `volume` | `number` | `100` | Volume (0-100) |
| `rate` | `number` | `0` | Velocidade (-10 a 10) |
| `autoSpeak` | `boolean` | `true` | Falar respostas automaticamente |
| `selectedVoice` | `string` | `''` | Nome da voz selecionada |
| `voiceSource` | `string` | `'disabled'` | Fonte: 'disabled', 'webspeech', 'sapi5', 'openai' |
| `testText` | `string` | `'Olá!...'` | Texto para teste de voz |
| `showAutoSpeak` | `boolean` | `true` | Mostrar toggle de auto-speak |
| `showVoiceInfo` | `boolean` | `true` | Mostrar informações da voz |
| `showTestButton` | `boolean` | `true` | Mostrar botão de testar |

### Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `volumeChange` | `{ volume }` | Quando volume muda |
| `rateChange` | `{ rate }` | Quando velocidade muda |
| `autoSpeakChange` | `{ autoSpeak }` | Quando auto-speak muda |

### Uso em Modal

```svelte
<script>
  import { Modal } from './components/modal';
  import { VoiceSettingsPanel } from './components/speech';
  
  let showSettings = false;
  let settingsPanel;
</script>

<button on:click={() => showSettings = true}>
  Configurações de Voz
</button>

<Modal 
  title="Configurações de Voz" 
  open={showSettings} 
  on:close={() => showSettings = false}
  autoFocus={false}
>
  <VoiceSettingsPanel
    bind:this={settingsPanel}
    bind:volume
    bind:rate
    bind:autoSpeak
    {selectedVoice}
    {voiceSource}
  />
</Modal>
```

### Integração com TTSService

O componente usa automaticamente o `ttsService` para aplicar configurações:

```svelte
<script>
  import { ttsService, TTS_PROVIDERS } from './lib/speech';
  
  // Configurar o serviço
  ttsService.setProvider(TTS_PROVIDERS.WEBSPEECH, { 
    voice: 'Microsoft Maria - Portuguese (Brazil)'
  });
</script>

<!-- O painel aplica volume/rate no serviço automaticamente -->
<VoiceSettingsPanel bind:volume bind:rate />
```

## Arquitetura

```
components/speech/
└── VoiceSettingsPanel.svelte    # UI de configurações

lib/speech/
├── tts-service.js               # Serviço de TTS (WebSpeech, SAPI5, OpenAI)
├── stt-service.js               # Serviço de STT (WebSpeech, Whisper)
├── tts-webspeech.js             # Manager WebSpeech TTS
├── stt-webspeech.js             # Manager WebSpeech STT
├── audio-recorder.js            # Gravador de áudio
└── vad.js                       # Voice Activity Detector
```

## Acessibilidade

- Todos os controles têm labels acessíveis
- Sliders têm `aria-valuemin`, `aria-valuemax`, `aria-valuenow`
- Descrições conectadas via `aria-describedby`
- Toggle usa checkbox nativo com label clicável
- Focus management quando usado em modal




