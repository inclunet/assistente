# Validação física de entrada — AEP-0103 / I13.6

Este runbook consolida o aceite de ambiente para teclado/foco/janela e
dispositivo físico. Ele não popula comandos do produto.

## Pré-condições

- Windows com uma sessão interativa desbloqueada.
- Worktree `feat/aep-0103-comandos`.
- Stream Deck já validado pelo runbook `streamdeck-manual-validation.md`.

## Comando

```powershell
$env:ASSISTENTE_PHYSICAL_MANUAL = "1"
$env:ASSISTENTE_STREAMDECK_MODEL = "Stream Deck"
$env:ASSISTENTE_STREAMDECK_SERIAL = "AL28K2C54852"
go test ./internal/commandphysical -run TestManualPhysicalEnvironment -count=1 -v
```

## Resultado esperado

- Foreground nativo captura a janela atual antes de qualquer bring-to-front.
- Hotkey global é reportado como suportado nesta plataforma.
- Sessão interativa atual é tratada como conhecida/desbloqueada para o teste.
- Stream Deck físico validado é incluído no relatório.
- O teste termina com `PASS`.

## Observação do ambiente Codex

Quando executado pelo processo em segundo plano do agente, o teste pode falhar
com `GetForegroundWindow: identidade da janela em primeiro plano desconhecida`.
Isso indica ausência de janela foreground interativa no processo de teste, não
falha do contrato. Execute pelo PowerShell visível do usuário para produzir a
evidência manual.

## Limites

- Este teste não registra uma hotkey real para evitar conflito com o sistema.
- Lock/unlock contínuo permanece coberto por `internal/ossession`; validar
  bloqueio real da estação pode ser feito depois em uma janela de teste dedicada.
- Mapas reais de comandos do produto continuam em P04/P01.
