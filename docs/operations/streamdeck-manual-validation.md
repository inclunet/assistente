# Validação manual do Stream Deck — AEP-0103 / I13.5

Este runbook valida a última parte que não pode ser provada por mock: HID físico.
Ele não habilita comandos de produto; exercita somente o driver real, o runtime
seguro e o recebimento de uma tecla.

## Pré-condições

- Go 1.25+ disponível (`go version`).
- Um Stream Deck suportado conectado por USB.
- Software oficial da Elgato fechado, ou qualquer processo que possa possuir o
  dispositivo encerrado.
- Worktree na branch `feat/aep-0103-comandos`.

## Comando

No diretório raiz do repo:

```powershell
$env:ASSISTENTE_STREAMDECK_MANUAL = "1"
go test ./internal/commanddeck -run TestManualStreamDeckPhysicalRoundTrip -count=1 -v
```

## Resultado esperado

1. O teste enumera e abre um Stream Deck.
2. A primeira tecla fica vermelha com o frame de teste.
3. O teste imprime `pressione a primeira tecla do Stream Deck em até 30s`.
4. Ao pressionar a primeira tecla física, o teste recebe um evento
   `streamdeck.key:<serial>` / `key:0`.
5. O shutdown envia frame seguro e fecha o handle.
6. O comando termina com `PASS`.

## Falhas úteis

- `nenhum Stream Deck enumerado`: Windows/USB/HID não expôs o dispositivo.
- `nenhum Stream Deck abriu`: outro processo pode estar segurando o dispositivo.
- timeout após renderizar: evento físico não chegou ao callback do driver.
- erro ao renderizar: modelo/geometria ou envio de imagem precisa ajuste.

## Evidência para fechar I13.5

Registre no acompanhamento, sem publicar número de série, token, identificador
pessoal ou outro dado real:

- modelo exibido pelo teste;
- se a tecla ficou vermelha;
- evento recebido (mascare `SourceInstance`/serial, se aparecer);
- saída final `PASS`;
- se houve disputa com outro processo.

## Execução registrada — 15/09/2026

- Worktree: `C:\Users\leonardo.gleison\dev\assistente-worktrees\aep-0103-comandos`.
- Comando: `ASSISTENTE_STREAMDECK_MANUAL=1 go test ./internal/commanddeck -run TestManualStreamDeckPhysicalRoundTrip -count=1 -v`.
- Resultado: `PASS`.
- Dispositivo: modelo `Stream Deck`, 15 teclas; serial omitido da documentação.
- Evento: tecla física `key:0` recebida; serial/`SourceInstance` omitido.
- Observação: o teste abriu o dispositivo, renderizou o frame manual e recebeu a primeira tecla física.
