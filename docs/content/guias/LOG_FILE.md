---
title: "Logs do aplicativo em arquivo"
weight: 7
---

# Gravar os logs do aplicativo gráfico

Em **Configurações → Logs**, ative a gravação, informe o caminho completo do
arquivo que ainda não exista e escolha o tamanho máximo e quantos arquivos anteriores serão mantidos.
Por padrão, o caminho é `~/.assistente/assistente.log`, o limite é 50 MiB,
mantêm-se cinco arquivos anteriores e o intervalo de rotação é de 24 horas.
A gravação começa desativada. O intervalo pode ser alterado ou desativado com 0.
Salvar essa configuração global requer uma conta administradora do Assistente.
Ao atingir o limite ou quando chegar uma gravação após o intervalo, o aplicativo passa a gravar em um novo arquivo e preserva
os anteriores em uma pasta própria com sufixo `.assistente-archives` junto ao
arquivo. Os arquivos mais
antigos são removidos quando excedem a retenção escolhida. Um registro individual
maior que o limite não é dividido. A configuração é aplicada imediatamente e
mantida entre inicializações.

Caso o destino configurado fique indisponível, o aplicativo continuará abrindo.
A aba Logs mostrará o erro para que você possa corrigir ou desativar a gravação.
Se o processo for encerrado durante a rotação, o aplicativo detecta a operação
pendente e pede outro caminho para retomar a gravação. Os arquivos anteriores e
os temporários permanecem na pasta `.assistente-archives` para inspeção e
recuperação manual. A detecção depende do sistema de arquivos ter persistido o
marcador da operação; uma queda de energia pode interromper essa persistência.
Se a criação inicial for interrompida antes de terminar, o destino também pode
exigir um novo caminho; os arquivos parciais ficam disponíveis para inspeção.

Também é possível iniciar o executável gráfico com `--log-file` e informar o caminho:

```powershell
.\assistente.exe --log-file "$env:TEMP\assistente.log"
```

A forma com sinal de igual também é aceita:

```powershell
.\assistente.exe --log-file="$env:TEMP\assistente.log"
```

O arquivo é criado quando não existe. Se já existir, os novos registros são
anexados ao final. A opção não aumenta o nível de detalhe: ela grava os mesmos
logs que a aplicação já produz, incluindo logs estruturados, logs padrão do Go
e avisos do banco de dados.

Quando `--log-file` é informado, seu destino prevalece durante aquela execução
e não usa a rotação configurada na interface. A configuração salva volta a
valer na próxima inicialização sem a opção.

Em desenvolvimento, a saída continua aparecendo no terminal e também é
gravada no arquivo. O arquivo é fechado quando o aplicativo encerra.

Se o caminho estiver ausente ou o arquivo não puder ser aberto, o aplicativo
informa o erro no início e não prossegue com a inicialização. Quando o
executável gráfico é aberto sem terminal, o erro também é apresentado em um
diálogo do sistema.

> Os mecanismos existentes de sanitização continuam valendo, mas o arquivo
> ainda pode conter informações de diagnóstico. Guarde-o em local protegido e
> revise o conteúdo antes de compartilhá-lo.
