package acpinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"assistente/internal/acp"
	"assistente/internal/logging"
)

// knownFileName é a memória do que já foi baixado sem digest publicado (D4).
//
// Ela mora na raiz das instalações, e não dentro do diretório do agente, porque
// remover o agente não é dizer que o artefato dele passou a ser outro: remover
// libera disco, e esquecer junto faria a próxima instalação ser de novo uma
// estreia. É a mesma escolha do `known_hosts` do SSH, que sobrevive a
// desconectar do servidor.
const knownFileName = "known-artifacts.json"

// knownSchema versiona o arquivo. Esquema desconhecido é tratado como memória
// que não existe: é de uma versão do app que sabe algo que esta não sabe, e
// adivinhar o formato seria pior do que perguntar de novo.
const knownSchema = 1

// maxKnownBytes é o teto do arquivo. Ele é dado externo, como todo o resto que
// mora no disco de quem usa o app, e é folgado o bastante para nunca alcançar
// uma memória de verdade — são 64 caracteres por versão instalada.
const maxKnownBytes = 1 << 20

// ErrArtifactChanged é a mesma versão do mesmo agente que voltou com outro
// arquivo (D4).
//
// O digest observado não protege a primeira instalação — nada protege, é essa a
// natureza do problema. O que ele faz é proteger as seguintes: um artefato
// diferente sob a mesma versão é sinal de que algo mudou onde não deveria.
var ErrArtifactChanged = errors.New("esta versão já foi baixada antes e o arquivo agora é outro")

// knownArtifacts é o conteúdo do arquivo.
type knownArtifacts struct {
	Schema int `json:"schema"`

	// Agents mapeia identificador do agente para versão e digest observado.
	// A forma aninhada é para o arquivo poder ser lido por quem o abrir: é ele
	// que sustenta uma recusa, e uma recusa que ninguém consegue conferir é
	// difícil de distinguir de defeito.
	Agents map[string]map[string]string `json:"agents"`
}

// knownDigest é o digest observado da última vez que esta versão foi instalada.
//
// Arquivo ilegível vira memória vazia, e não recusa: quem consegue corromper o
// arquivo consegue trocar o executável instalado ao lado dele, então travar
// toda instalação por causa disso custaria caro sem fechar porta nenhuma. O
// aviso fica no log, que é onde ele é útil.
func (i *Installer) knownDigest(agentID, version string) string {
	known, err := i.readKnown()
	if err != nil {
		logging.Warnf(context.Background(), component,
			"não foi possível ler a memória de artefatos em %s: %v", i.knownPath(), err)
		return ""
	}
	return known.Agents[agentID][version]
}

// rememberArtifact guarda o digest observado desta versão.
//
// Falhar aqui não desfaz a instalação: o agente está no disco e funciona, e o
// que se perde é a proteção da próxima vez. Desfazer por causa disso trocaria
// uma proteção futura por um agente que a pessoa não tem agora.
func (i *Installer) rememberArtifact(ctx context.Context, agentID, version, digest string) {
	if i.root == "" || agentID == "" || version == "" || digest == "" {
		return
	}
	i.knownMu.Lock()
	defer i.knownMu.Unlock()

	known, err := i.readKnown()
	if err != nil {
		known = knownArtifacts{}
	}
	if known.Agents == nil {
		known.Agents = make(map[string]map[string]string)
	}
	if known.Agents[agentID] == nil {
		known.Agents[agentID] = make(map[string]string)
	}
	known.Agents[agentID][version] = digest
	known.Schema = knownSchema

	if err := i.writeKnown(known); err != nil {
		logging.Warnf(ctx, component,
			"não foi possível guardar o digest observado de %s %s: %v", agentID, version, err)
	}
}

func (i *Installer) knownPath() string {
	if i.root == "" {
		return ""
	}
	return filepath.Join(i.root, knownFileName)
}

func (i *Installer) readKnown() (knownArtifacts, error) {
	path := i.knownPath()
	if path == "" {
		return knownArtifacts{}, nil
	}
	data, err := readAtMost(path, maxKnownBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return knownArtifacts{}, nil
		}
		return knownArtifacts{}, err
	}
	var known knownArtifacts
	if err := json.Unmarshal(data, &known); err != nil {
		return knownArtifacts{}, err
	}
	if known.Schema != knownSchema {
		return knownArtifacts{}, fmt.Errorf("esquema %d desconhecido", known.Schema)
	}
	return known, nil
}

// writeKnown grava no temporário e renomeia, pelo mesmo motivo do registro da
// instalação: o app pode cair no meio da escrita, e um JSON pela metade
// apagaria a memória inteira em vez de uma entrada.
func (i *Installer) writeKnown(known knownArtifacts) error {
	path := i.knownPath()
	if path == "" {
		return errors.New("sem diretório de dados do app")
	}
	if err := os.MkdirAll(i.root, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(known, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(i.root, ".known-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := replaceFile(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// checkKnownArtifact recusa a versão que já foi baixada antes e voltou outra.
//
// A mensagem não lê os dois digests: são 64 caracteres cada, e ouvi-los em
// sequência não ajuda ninguém a decidir. Ela nomeia o agente e a versão, e diz
// o caminho que continua aberto — baixar do site do fornecedor e apontar o
// comando aqui. Os digests vão para o log, que é onde conferi-los é possível.
func (i *Installer) checkKnownArtifact(ctx context.Context, agentID, name, version, digest string) error {
	remembered := i.knownDigest(agentID, version)
	if remembered == "" || remembered == digest {
		return nil
	}
	logging.Warnf(ctx, component,
		"o artefato de %s %s mudou: esperava %s e chegou %s", agentID, version, remembered, digest)
	return failf(StepDownload, "%w: %s %s. Se a mudança for esperada, baixe o agente pelo site do fornecedor e aponte o comando à mão",
		ErrArtifactChanged, acp.SanitizeLabel(name), acp.SanitizeLabel(version))
}
