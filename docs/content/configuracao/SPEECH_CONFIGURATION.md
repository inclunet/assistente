---
title: "Voz (TTS e STT)"
weight: 2
---

# Configuração de Voz (TTS e STT)

O Assistente possui um sistema completo de voz com Text-to-Speech (TTS) e Speech-to-Text (STT), permitindo interação por voz bidirecional com o assistente de IA.

## Text-to-Speech (TTS)

O TTS permite que o assistente leia respostas em voz alta. Há três engines disponíveis:

### Engines Disponíveis

| Engine | Plataformas | Qualidade | Custo | Notas |
|---|---|---|---|---|
| **Web Speech API** | Todas | Média | Gratuito | Usa vozes do navegador/sistema |
| **SAPI5** | Windows | Boa | Gratuito | Integra com NVDA, JAWS e Narrator |
| **OpenAI TTS** | Todas | Excelente | Pago | Requer API key OpenAI configurada |

### Configurações TTS

Acesse **Configurações** (`Alt + 2`) → **Voz**:

- **Ativar TTS**: Liga/desliga a saída de voz
- **Leitura automática**: Lê respostas do assistente automaticamente ao receber
- **Engine**: Escolha entre Web Speech, SAPI5 ou OpenAI
- **Voz**: Selecione entre as vozes disponíveis na engine escolhida
- **Velocidade**: Ajuste de -10 a 10
- **Volume**: Ajuste de 0 a 100

### Escolhendo a Engine Certa

- **Usuários de leitores de tela (NVDA/JAWS)**: Use **SAPI5** para integração natural
- **Uso geral**: **Web Speech API** funciona em todas as plataformas sem custo
- **Máxima qualidade**: **OpenAI TTS** oferece vozes naturais de alta qualidade (requer créditos OpenAI)

## Speech-to-Text (STT)

O STT permite ditar mensagens por voz ao invés de digitar.

### Providers Disponíveis

| Provider | Qualidade | Custo | Notas |
|---|---|---|---|
| **Web Speech** | Boa | Gratuito | Usa reconhecimento do navegador |
| **Whisper API** | Excelente | Pago | OpenAI Whisper — suporta 100+ idiomas |

### Modos de Gravação

| Modo | Descrição |
|---|---|
| **PTT (Push-to-Talk)** | Segure o botão para gravar, solte para enviar |
| **Toggle** | Clique para iniciar, clique novamente para parar |
| **VAD (Silêncio)** | Gravação para automaticamente ao detectar silêncio |
| **VAD (Atividade)** | Grava enquanto detecta atividade de voz |
| **Gravar Áudio** | Grava áudio bruto para envio |

### Configurações STT

- **Provider**: Web Speech ou Whisper API
- **Idioma**: pt-BR, en, es, etc.
- **Duração de silêncio**: Tempo de silêncio para parar gravação (modo VAD)
- **Limiar de silêncio**: Sensibilidade para detectar silêncio
- **Limiar de atividade**: Sensibilidade para detectar fala

## Wake Word

O sistema de Wake Word permite ativar o assistente por voz sem precisar interagir com a janela do aplicativo.

### Como Funciona

1. O reconhecimento fica ativo em segundo plano usando Web Speech API
2. Quando a palavra-chave configurada é detectada, o STT é ativado automaticamente
3. Você pode ditar seu comando normalmente
4. A resposta é lida via TTS se ativado

### Configuração

- **Palavra-chave**: Defina a palavra que ativa o assistente (ex: "assistente", "hey")
- **Idioma**: Idioma do reconhecimento contínuo

### Dicas

- Escolha uma palavra-chave que não apareça naturalmente em conversas
- O Wake Word funciona mesmo com a janela minimizada
- Combine com TTS automático para interação totalmente hands-free

## Perfis de Voz

É possível configurar parâmetros de voz diferentes por perfil de interação. Cada perfil pode ter:

- Engine TTS diferente
- Voz diferente
- Velocidade e volume individuais

Configure em **Configurações** → **Perfis** → edite o perfil desejado.
