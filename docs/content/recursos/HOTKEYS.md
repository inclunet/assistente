---
title: "Hotkeys Globais"
weight: 6
---

# Hotkeys Globais

O Assistente suporta atalhos de teclado globais que funcionam mesmo quando a janela não está em foco — útil para ativar o assistente rapidamente ou controlar funcionalidades como STT.

## Como Funciona

Hotkeys globais são registrados no nível do sistema operacional usando APIs nativas:

| Plataforma | Tecnologia |
|---|---|
| **Windows** | Windows API (RegisterHotKey) |
| **macOS** | Carbon Events |
| **Linux** | X11 |

## Modificadores Suportados

| Modificador | Tecla |
|---|---|
| `Ctrl` | Control |
| `Shift` | Shift |
| `Alt` | Alt |
| `Win/Cmd` | Windows / Command |

Combinações são suportadas (ex: `Ctrl + Shift + A`).

## Registro

Hotkeys são registrados programaticamente pelo app durante a inicialização. A configuração é feita em **Configurações** → **Atalhos Globais** (quando disponível).

## Casos de Uso

- **Ativar STT**: Pressione um atalho global para começar a ditar, mesmo com o app minimizado
- **Trazer app ao foco**: Atalho para trazer a janela do assistente à frente
- **Controle de TTS**: Pausar/retomar leitura por voz

## Notas

- Hotkeys globais podem conflitar com atalhos de outros aplicativos
- No Linux, requer X11 (Wayland pode ter limitações)
- Cada hotkey roda em sua própria goroutine para não bloquear o app
