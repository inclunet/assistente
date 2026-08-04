package controllers

import (
	"strings"
	"testing"

	"assistente/internal/updater"
)

func TestOConviteParaAtualizarVaiTraduzivelParaATela(t *testing.T) {
	payload := updatePromptPayload(&updater.UpdateInfo{
		CurrentVersion: "1.2.0",
		LatestVersion:  "1.3.0",
	})

	exigirContratoDoDialogo(t, "convite para atualizar", payload, dialogoEsperado{
		traduzido: map[string]string{
			"title":          "Atualização Disponível",
			"description":    "Versão atual: 1.2.0\nNova versão: 1.3.0",
			"submitLabel":    "Atualizar",
			"cancelLabel":    "Mais Tarde",
			"prompt:confirm": "Deseja atualizar agora?",
		},
	})
	exigirParametrosSemNomesReservados(t, "convite para atualizar", payload)
}

// A descrição muda com o que a release traz, e as quatro formas dividem um campo
// só: é a chave que diz qual delas está na tela. Com uma chave só, quem traduz
// deixaria de fora as notas da versão ou o tamanho do download — e é pelo tamanho
// que alguém em conexão limitada decide esperar.
func TestADescricaoDaAtualizacaoDistingueNotasETamanho(t *testing.T) {
	casos := map[string]struct {
		info      updater.UpdateInfo
		noTexto   []string
		foraDele  []string
		params    []string
		semParams []string
	}{
		"sem notas nem tamanho": {
			info:      updater.UpdateInfo{CurrentVersion: "1.2.0", LatestVersion: "1.3.0"},
			noTexto:   []string{"Versão atual: 1.2.0", "Nova versão: 1.3.0"},
			foraDele:  []string{"Notas da versão", "Tamanho do download"},
			params:    []string{"current", "latest"},
			semParams: []string{"notes", "size"},
		},
		"com notas": {
			info:      updater.UpdateInfo{CurrentVersion: "1.2.0", LatestVersion: "1.3.0", ReleaseNotes: "Corrige o foco do diálogo"},
			noTexto:   []string{"Notas da versão:\nCorrige o foco do diálogo"},
			foraDele:  []string{"Tamanho do download"},
			params:    []string{"notes"},
			semParams: []string{"size"},
		},
		"com tamanho": {
			info:      updater.UpdateInfo{CurrentVersion: "1.2.0", LatestVersion: "1.3.0", DownloadSize: 3 * 1024 * 1024},
			noTexto:   []string{"Tamanho do download: 3.00 MB"},
			foraDele:  []string{"Notas da versão"},
			params:    []string{"size"},
			semParams: []string{"notes"},
		},
		"com notas e tamanho": {
			info: updater.UpdateInfo{
				CurrentVersion: "1.2.0", LatestVersion: "1.3.0",
				ReleaseNotes: "Corrige o foco do diálogo", DownloadSize: 3 * 1024 * 1024,
			},
			noTexto: []string{"Notas da versão:\nCorrige o foco do diálogo", "Tamanho do download: 3.00 MB"},
			params:  []string{"notes", "size"},
		},
	}

	porChave := make(map[string]string, len(casos))
	for nome, caso := range casos {
		info := caso.info
		descricao := updatePromptPayload(&info).Description

		if descricao.Key == "" {
			t.Errorf("descrição %s = %+v, quer chave de tradução", nome, descricao)
			continue
		}
		if anterior, repetida := porChave[descricao.Key]; repetida {
			t.Errorf("as descrições %q e %q dividem a chave %q: uma delas seria dita pela outra",
				anterior, nome, descricao.Key)
		}
		porChave[descricao.Key] = nome

		for _, trecho := range caso.noTexto {
			if !strings.Contains(descricao.Fallback, trecho) {
				t.Errorf("descrição %s = %q, quer %q já no lugar", nome, descricao.Fallback, trecho)
			}
		}
		for _, trecho := range caso.foraDele {
			if strings.Contains(descricao.Fallback, trecho) {
				t.Errorf("descrição %s = %q, anuncia %q que a release não traz", nome, descricao.Fallback, trecho)
			}
		}
		for _, param := range caso.params {
			if _, presente := descricao.Params[param]; !presente {
				t.Errorf("descrição %s: params = %v, quer %q para a tradução repetir a interpolação",
					nome, descricao.Params, param)
			}
		}
		for _, param := range caso.semParams {
			if _, presente := descricao.Params[param]; presente {
				t.Errorf("descrição %s: params = %v, traz %q que a release não tem", nome, descricao.Params, param)
			}
		}
	}
}
