package acpinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clienteFalso responde o download sem rede.
type clienteFalso struct {
	status int
	corpo  []byte
	err    error
	pedido *http.Request
}

func (c *clienteFalso) Do(_ context.Context, req *http.Request) (*http.Response, error) {
	c.pedido = req
	if c.err != nil {
		return nil, c.err
	}
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(c.corpo)),
		Header:     make(http.Header),
	}, nil
}

func digestDe(data []byte) string {
	soma := sha256.Sum256(data)
	return hex.EncodeToString(soma[:])
}

func TestOAlvoDaPlataformaSegueONomeDoRegistro(t *testing.T) {
	casos := []struct {
		goos, goarch, esperado string
	}{
		{"windows", "amd64", "windows-x86_64"},
		{"windows", "arm64", "windows-aarch64"},
		{"darwin", "arm64", "darwin-aarch64"},
		{"linux", "amd64", "linux-x86_64"},
		{"linux", "386", ""},
		{"freebsd", "amd64", ""},
	}
	for _, caso := range casos {
		if got := platformTargetFor(caso.goos, caso.goarch); got != caso.esperado {
			t.Errorf("%s/%s: esperava %q, veio %q", caso.goos, caso.goarch, caso.esperado, got)
		}
	}
}

func TestOFormatoVemDoNomeDoArquivoNaURL(t *testing.T) {
	casos := []struct {
		url      string
		esperado archiveFormat
	}{
		{"https://exemplo.test/agente.zip", formatZip},
		{"https://exemplo.test/agente.tar.gz", formatTarGz},
		{"https://exemplo.test/agente.tgz", formatTarGz},
		{"https://exemplo.test/agente.tar.bz2", formatTarBz2},
		{"https://exemplo.test/agente.tbz2", formatTarBz2},
		{"https://exemplo.test/opencode", formatRaw},
		{"https://exemplo.test/agente.exe", formatRaw},
		{"https://exemplo.test/agente.zip?v=2#tag", formatZip},
		{"https://exemplo.test/v1.2.3/AGENTE.TAR.GZ", formatTarGz},
	}
	for _, caso := range casos {
		got, err := formatOf(caso.url)
		if err != nil {
			t.Fatalf("%s: %v", caso.url, err)
		}
		if got != caso.esperado {
			t.Errorf("%s: esperava %q, veio %q", caso.url, caso.esperado, got)
		}
	}
}

func TestInstaladorDoSistemaNaoEArtefatoInstalavel(t *testing.T) {
	// O registro já recusa instalador, mas o índice é dado externo e a recusa é
	// refeita aqui: rodar um `.msi` é outra coisa, e pede outra conversa.
	for _, url := range []string{
		"https://exemplo.test/agente.msi",
		"https://exemplo.test/agente.dmg",
		"https://exemplo.test/agente.deb",
		"https://exemplo.test/agente.AppImage",
		"https://exemplo.test/agente.tar",
		"https://exemplo.test/agente.7z",
	} {
		if _, err := formatOf(url); !errors.Is(err, ErrUnsupportedArchive) {
			t.Errorf("%s: esperava recusa de formato, veio %v", url, err)
		}
	}
}

func TestOArtefatoQueBateComODigestFicaNoDisco(t *testing.T) {
	dest := t.TempDir()
	corpo := []byte("conteúdo do agente")
	cliente := &clienteFalso{corpo: corpo}

	art, err := fetchArtifact(context.Background(), cliente, "https://exemplo.test/agente.zip", digestDe(corpo), dest)
	if err != nil {
		t.Fatalf("esperava sucesso, veio %v", err)
	}
	if art.SHA256 != digestDe(corpo) {
		t.Errorf("digest esperado %s, veio %s", digestDe(corpo), art.SHA256)
	}
	if art.Format != formatZip {
		t.Errorf("esperava formato zip, veio %q", art.Format)
	}
	if art.Bytes != int64(len(corpo)) {
		t.Errorf("esperava %d bytes, veio %d", len(corpo), art.Bytes)
	}
	if !acpWithin(dest, art.Path) {
		t.Errorf("o download foi parar em %s, fora de %s", art.Path, dest)
	}
	if got, err := os.ReadFile(art.Path); err != nil || !bytes.Equal(got, corpo) {
		t.Errorf("o arquivo gravado não é o que chegou: %v", err)
	}
	if cliente.pedido.Header.Get("User-Agent") == "" {
		t.Error("o download saiu sem User-Agent")
	}
}

func TestOArtefatoQueNaoBateComODigestNaoFicaNoDisco(t *testing.T) {
	dest := t.TempDir()
	cliente := &clienteFalso{corpo: []byte("outro conteúdo")}

	_, err := fetchArtifact(context.Background(), cliente, "https://exemplo.test/agente.zip", digestDe([]byte("o esperado")), dest)
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("esperava divergência de digest, veio %v", err)
	}
	// Um artefato que não bate não pode ficar esperando alguém executá-lo por
	// engano — é a razão de o digest existir.
	restou, _ := os.ReadDir(dest)
	if len(restou) != 0 {
		t.Errorf("o download divergente ficou no disco: %v", nomesDe(restou))
	}
}

func TestSemDigestPublicadoOObservadoEODoQueChegou(t *testing.T) {
	dest := t.TempDir()
	corpo := []byte("agente sem digest publicado")
	cliente := &clienteFalso{corpo: corpo}

	art, err := fetchArtifact(context.Background(), cliente, "https://exemplo.test/agente.tar.gz", "", dest)
	if err != nil {
		t.Fatalf("esperava sucesso, veio %v", err)
	}
	if art.SHA256 != digestDe(corpo) {
		t.Errorf("esperava o digest observado %s, veio %s", digestDe(corpo), art.SHA256)
	}
}

func TestRespostaQueNaoE200NaoViraArtefato(t *testing.T) {
	dest := t.TempDir()
	cliente := &clienteFalso{status: http.StatusNotFound, corpo: []byte("<html>não achei</html>")}

	_, err := fetchArtifact(context.Background(), cliente, "https://exemplo.test/agente.zip", "", dest)
	if !errors.Is(err, ErrDownload) {
		t.Fatalf("esperava falha de download, veio %v", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("a mensagem devia dizer o código: %v", err)
	}
	// E o texto que o servidor escreveu não entra na mensagem: ela acaba em
	// tela e em anúncio.
	if strings.Contains(err.Error(), "não achei") {
		t.Errorf("a mensagem carregou texto do servidor: %v", err)
	}
	restou, _ := os.ReadDir(dest)
	if len(restou) != 0 {
		t.Errorf("a resposta de erro deixou arquivo em %s: %v", dest, nomesDe(restou))
	}
}

func TestDownloadQueNaoParaDeChegarERecusadoPeloTeto(t *testing.T) {
	dest := t.TempDir()

	// Um servidor que nunca fecha a resposta é o caso contra o qual o teto
	// existe. O limite entra por parâmetro para o teste provar a recusa sem
	// transferir meio gigabyte.
	_, err := storeArtifact(semFim{}, dest, formatZip, 1024)
	if !errors.Is(err, ErrArtifactTooLarge) {
		t.Fatalf("esperava recusa por tamanho, veio %v", err)
	}
	restou, _ := os.ReadDir(dest)
	if len(restou) != 0 {
		t.Errorf("o download recusado ficou no disco: %v", nomesDe(restou))
	}
}

type semFim struct{}

func (semFim) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

func nomesDe(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name())
	}
	return out
}

func acpWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && !strings.HasPrefix(rel, "..")
}
