# Heap da timeline na WebView2

Medição local opt-in do AEP-0116, no Windows com WebView2 instalado:

```powershell
./frontend/performance/timeline-heap/run.ps1
```

O host usa Wails e WebView2 reais, com perfil exclusivo e janela oculta. A
fixture compila `ChatSessionProvider`, seu hook de lifecycle, `MessageList`,
`MessageNode`, `ChatTimelinePresentation`, `ChatTimelineClient` e o adaptador
Wails de produção. Somente as respostas RPC de abrir/navegar/fechar são
substituídas por dados determinísticos, sem acessar o banco ou provedores do
usuário e sem executar ACP. O bundle do aplicativo não contém o harness.

O loader Go do Wails apaga `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`. Por isso o
script usa uma cópia do Wails e `go build -modfile` para acrescentar somente a
porta CDP ao host de medição. A cópia instrumentada fica em `build/bin`; nenhum arquivo do módulo instalado,
registro do Windows ou configuração do aplicativo é alterado. Mudança no ponto
de instrumentação do Wails interrompe o script para revisão.

Para históricos de 100, 500 e 1.000 mensagens, abre e fecha uma janela limitada
a 240 itens. Cada mensagem contém aproximadamente 4 KiB de texto. As conversas
têm identidades distintas para exercitar a evicção da LRU da sessão. Cada
cenário tem cinco ciclos de aquecimento e trinta ciclos medidos, com amostras a
cada cinco ciclos e após logout. A mesma sessão conserva sua LRU entre ciclos;
logout muda a sessão no store de autenticação, exercitando o descarte real do
cliente. A fixture verifica também a liberação das sessões visuais do store.
Os casos de 500 e 1.000 têm os mesmos 240 itens materializados: exercitam
históricos maiores com janela limitada, não cargas de 500/1.000 itens na UI.

`HeapProfiler.collectGarbage` precede `Runtime.getHeapUsage`. O JSON em
`build/bin/agent-build/results/timeline-webview-heap.json` registra a versão do
runtime e o heap JavaScript/V8 efetivo, em vez de inferi-lo pelo tamanho
serializado do cache. Não é o consumo nativo total do processo WebView2.
O script falha se a lista não montar, uma janela ficar aberta ou ocorrer erro
JavaScript. A inspeção por protótipo conta instâncias ainda vivas e itens da
LRU depois do GC, exigindo até 240 itens após fechar e zero após logout. As
referências criadas pela própria inspeção são liberadas antes da próxima coleta.
A curva de heap precisa ser inspecionada: não há um teto inventado
para transformar qualquer resultado em aprovação automática.

Esta prova cobre retenção de estado e componentes na WebView. Não mede o heap
Go, latência SQL/IPC, TTS, áudio, execução de ferramentas ou NVDA. Os perfis da
WebView e artefatos permanecem em `build/bin/agent-build/` para inspeção; o
processo do harness é encerrado ao terminar, inclusive em caso de erro.
