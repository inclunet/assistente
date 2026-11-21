---
title: "Plugins"
weight: 24
---

# Plugins

Abra **Plugins** pelo menu principal para consultar os pacotes instalados na sua conta. A lista mostra nome, versão informada pelo autor e pasta de origem.

## Instalar uma pasta local

1. Escolha **Escolher pasta de plugin** e selecione a pasta que contém o `plugin.json` do formato Agent Plugins 1.0.0.
2. Confira a origem e os arquivos incluídos na prévia. O identificador SHA-256 corresponde aos nomes e ao conteúdo dos arquivos capturados. Ele permite conferir a revisão instalada.
3. Escolha **Instalar revisão** para guardar uma cópia na conta atual ou **Cancelar** para fechar a prévia.

Se os arquivos mudarem depois da prévia, abra a pasta novamente para examinar a nova revisão. Para atualizar um pacote, selecione a mesma pasta. A prévia mostra o digest instalado e o novo digest. Escolha **Atualizar revisão** para confirmar. Se ambos forem iguais, a tela informa que essa revisão já está instalada. A nova revisão começa com os componentes inativos.

## Ativar uma skill

Na grade de plugins instalados, selecione o pacote e pressione Enter. Os detalhes listam as skills e os servidores MCP encontrados. Para uma skill de instruções compatível, escolha **Ativar**. O identificador exibido após a ativação pode ser usado no chat como comando de barra, desde que o perfil permita a skill. Use **Desativar** para removê-la das próximas seleções. Ao atualizar a revisão, confira e ative os componentes desejados novamente.

A interface indica quando arquivos auxiliares ou campos de configuração exigem suporte adicional. O reconhecimento de um servidor MCP na prévia ainda não inicia uma conexão.

## Registrar um servidor MCP

Nos detalhes do plugin, confira o endereço e o transporte. Para servidores SSE ou Streamable HTTP compatíveis, escolha **Adicionar aos servidores MCP**. O servidor aparece na configuração de MCP da conta, onde você pode conectá-lo. O registro deixa a conexão automática desativada.

Nos detalhes do plugin, você pode remover o servidor ou mantê-lo como servidor independente. Ao atualizar o plugin, servidores vinculados cuja configuração continua igual são removidos. Se você editou um deles, mantenha-o como independente antes de repetir a atualização para preservar sua configuração.
