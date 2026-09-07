---
title: "Voz — TTS e STT"
weight: 14
---

# Voz

- **TTS**: 3 engines — OpenAI (alta qualidade), SAPI5 (integrado ao leitor de tela) e Web Speech (gratuito), com arbitragem global (uma voz por vez).
- **STT**: Whisper (API) ou Web Speech, com modos push-to-talk, toggle e detecção de silêncio; wake word com janela minimizada.
- Configure por perfil em **Perfis → Editar perfil → Áudio**.

Ao pressionar Espaço sobre uma mensagem sem voz configurada, o Assistente
pergunta se você deseja configurá-la. A ação **Configurar voz** abre diretamente
a seção de voz do perfil usado pela conversa. Depois de salvar ou cancelar, você
volta à mesma aba e conversa, sem perder o contexto.

Links internos também podem abrir essa seção diretamente:
`assistente://profiles/edit/<slug>?tab=voice`.
