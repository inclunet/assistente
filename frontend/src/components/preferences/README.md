# Preferences Components

Componentes de preferências reutilizáveis para configurações do chat.

## Componentes

### ChatPreferences

Componente de preferências do chat com guias (Chat, Voz, Transcrição).
Usado tanto no modal de preferências por conversa quanto na página de configurações globais.

```svelte
<script>
  import { ChatPreferences } from './components/preferences';
  
  let model = 'gpt-4o';
  let temperature = 0.7;
  let maxTokens = 4096;
  let voice = 'disabled';
  
  function handleChange(event) {
    console.log('Campo alterado:', event.detail.field);
    console.log('Novo valor:', event.detail.value);
    console.log('Todas as preferências:', event.detail.preferences);
  }
</script>

<ChatPreferences
  bind:model
  bind:temperature
  bind:maxTokens
  bind:topP
  bind:useTools
  bind:showInternalMessages
  bind:voice
  bind:autoSpeak
  bind:voiceVolume
  bind:voiceRate
  bind:sttProvider
  bind:recordingMode
  on:change={handleChange}
/>
```

#### Props

| Prop | Tipo | Padrão | Descrição |
|------|------|--------|-----------|
| model | string | '' | Modelo LLM selecionado |
| temperature | number | 0.7 | Temperatura (0.0 - 2.0) |
| maxTokens | number | 4096 | Máximo de tokens |
| topP | number | 1.0 | Top P (0.0 - 1.0) |
| useTools | boolean | true | Se usa ferramentas/agentes |
| showInternalMessages | boolean | false | Se mostra mensagens internas |
| voice | string | 'disabled' | Voz TTS (ID ou 'disabled') |
| autoSpeak | boolean | true | Falar respostas automaticamente |
| voiceVolume | number | 100 | Volume (0-100) |
| voiceRate | number | 0 | Velocidade (-10 a 10) |
| sttProvider | string | 'webspeech' | Provedor STT |
| recordingMode | string | 'ptt' | Modo de gravação |
| showAdvanced | boolean | true | Mostra parâmetros avançados |
| disabled | boolean | false | Desabilita todos os controles |
| compact | boolean | false | Layout compacto |
| initialTab | string | 'chat' | Aba inicial ativa |

#### Eventos

- `change` - Disparado quando qualquer preferência muda
  - `detail.field` - Nome do campo alterado
  - `detail.value` - Novo valor
  - `detail.preferences` - Objeto com todas as preferências atuais

#### Métodos Públicos

- `getPreferences()` - Retorna objeto com todas as preferências
- `setPreferences(prefs)` - Define todas as preferências de uma vez

## Uso com ConfigModal

O ChatPreferences é projetado para funcionar com o ConfigModal:

```svelte
<script>
  import { ConfigModal } from './components/modal';
  import { ChatPreferences } from './components/preferences';
  
  let showModal = false;
  let prefsComponent;
  let hasChanges = false;
  
  function handleChange() {
    hasChanges = true;
  }
  
  async function handleApply() {
    const prefs = prefsComponent.getPreferences();
    await savePreferences(prefs);
  }
  
  async function handleSave() {
    await handleApply();
    showModal = false;
  }
  
  function handleCancel() {
    showModal = false;
  }
</script>

<ConfigModal
  title="Preferências"
  open={showModal}
  {hasChanges}
  on:apply={handleApply}
  on:save={handleSave}
  on:cancel={handleCancel}
>
  <ChatPreferences
    bind:this={prefsComponent}
    on:change={handleChange}
  />
</ConfigModal>
```

## Preferências por Conversa

As preferências podem ser salvas por conversa usando a API:

```javascript
import { UpdateConversationPreferences } from '../wailsjs/go/main/App.js';

// Salvar preferências na conversa atual
await UpdateConversationPreferences(conversationId, {
  model: 'gpt-4o',
  temperature: 0.7,
  max_tokens: 4096,
  top_p: 1.0,
  use_tools: true,
  show_internal_messages: false,
  voice: 'disabled',
  auto_speak: true,
  voice_volume: 100,
  voice_rate: 0,
  stt_provider: 'webspeech',
  recording_mode: 'ptt'
});
```

## Preferências Globais

As preferências globais (padrão para novas conversas) são configuradas na página de Configurações e salvas em `config.json`.

