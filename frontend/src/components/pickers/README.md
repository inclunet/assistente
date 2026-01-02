# Pickers

Componentes de seleção especializados baseados no `Combobox`.

## Componentes

### ModelPicker

Seletor de modelos de LLM (GPT-4, Claude, etc.).

```svelte
<script>
  import { ModelPicker } from '../components/pickers';
  let model = 'gpt-4';
</script>

<ModelPicker bind:value={model} variant="toolbar" />
```

**Props:**
- `value` - Modelo selecionado
- `label` - Label do campo (default: 'Modelo')
- `icon` - Ícone (default: '🤖')
- `variant` - 'toolbar' (compacto) ou 'form' (com label)
- `disabled` - Desabilita o picker
- `maxWidth` - Largura máxima

---

### ImageModelPicker

Seletor de modelos com suporte a visão/imagens.

```svelte
<script>
  import { ImageModelPicker, VisionStatus } from '../components/pickers';
  let imageModel = '';
</script>

<ImageModelPicker 
  bind:value={imageModel} 
  chatModel="gpt-4"
  filterVisionOnly={false}
/>
```

**Props:**
- `value` - Modelo selecionado
- `chatModel` - Modelo de chat principal (para fallback)
- `filterVisionOnly` - Mostrar apenas modelos com visão confirmada
- `variant`, `label`, `icon`, etc.

**Exports:**
- `VisionStatus` - Enum: `UNKNOWN`, `TESTING`, `CONFIRMED`, `NOT_SUPPORTED`
- `getVisionStatus(modelName, cache)` - Verifica status de visão do modelo

---

### VoicePicker

Seletor de vozes TTS (SAPI5, OpenAI, Web Speech).

```svelte
<script>
  import { VoicePicker, VOICE_DISABLED } from '../components/pickers';
  let voice = VOICE_DISABLED;
</script>

<VoicePicker 
  bind:value={voice} 
  language="pt"
  allowDisabled={true}
/>
```

**Props:**
- `value` - ID da voz selecionada
- `language` - Filtro de idioma preferido (default: 'pt')
- `allowDisabled` - Permite opção "desativada"
- `variant`, `label`, `icon`, etc.

**Exports:**
- `VOICE_DISABLED` - Constante para voz desativada

**Métodos:**
- `getSelectedVoice()` - Retorna objeto com informações da voz
- `isSelectedVoiceSAPI5()` - Verifica se é voz SAPI5
- `isSelectedVoiceOpenAI()` - Verifica se é voz OpenAI
- `getOpenAIVoiceId()` - Retorna ID da voz OpenAI

---

### STTProviderPicker

Seletor de provedores de Speech-to-Text.

```svelte
<script>
  import { STTProviderPicker, STT_WEBSPEECH, STT_WHISPER } from '../components/pickers';
  let sttProvider = STT_WEBSPEECH;
</script>

<STTProviderPicker bind:value={sttProvider} />
```

**Props:**
- `value` - ID do provedor selecionado
- `variant`, `label`, `icon`, etc.

**Exports (constantes de provedores):**
- `STT_WEBSPEECH` - Web Speech API (navegador)
- `STT_WHISPER` - OpenAI Whisper
- `STT_AZURE` - Azure Speech (futuro)
- `STT_GOOGLE` - Google Speech (futuro)
- `STT_REALTIME` - GPT-4o Realtime (futuro)

---

## Variantes

Todos os pickers suportam duas variantes:

### `toolbar` (padrão)
Compacto, ideal para barras de ferramentas:
```svelte
<ModelPicker variant="toolbar" />
```

### `form`
Com label e texto de ajuda:
```svelte
<ModelPicker variant="form" label="Modelo Principal" helpText="Escolha o modelo de IA" />
```

---

## Importação

```javascript
import { 
  ModelPicker, 
  ImageModelPicker, 
  VoicePicker, 
  STTProviderPicker,
  VOICE_DISABLED,
  STT_WEBSPEECH,
  STT_WHISPER,
  VisionStatus,
  getVisionStatus
} from '../components/pickers';
```

