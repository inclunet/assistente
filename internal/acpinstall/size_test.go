package acpinstall

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/acpregistry"
)

// clienteTamanho responde HEAD/GET com tamanhos controlados para o plano.
type clienteTamanho struct {
	headStatus int
	headLen    int64
	headErr    error
	getStatus  int
	getLen     int64
	getRange   string
	getErr     error
	metodos    []string
}

func (c *clienteTamanho) Do(_ context.Context, req *http.Request) (*http.Response, error) {
	c.metodos = append(c.metodos, req.Method)
	switch req.Method {
	case http.MethodHead:
		if c.headErr != nil {
			return nil, c.headErr
		}
		status := c.headStatus
		if status == 0 {
			status = http.StatusOK
		}
		return &http.Response{
			StatusCode:    status,
			Body:          io.NopCloser(bytes.NewReader(nil)),
			Header:        make(http.Header),
			ContentLength: c.headLen,
		}, nil
	case http.MethodGet:
		if c.getErr != nil {
			return nil, c.getErr
		}
		status := c.getStatus
		if status == 0 {
			status = http.StatusOK
		}
		header := make(http.Header)
		if c.getRange != "" {
			header.Set("Content-Range", c.getRange)
		}
		return &http.Response{
			StatusCode:    status,
			Body:          io.NopCloser(bytes.NewReader([]byte{0})),
			Header:        header,
			ContentLength: c.getLen,
		}, nil
	default:
		return nil, http.ErrNotSupported
	}
}

func TestOPlanoBinarioPreencheBytesComContentLengthDoHEAD(t *testing.T) {
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	cliente := &clienteTamanho{headLen: 42_000}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode, http: cliente})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if !plano.CanInstall {
		t.Fatalf("CanInstall deveria continuar verdadeiro: %+v", plano)
	}
	if plano.Bytes != 42_000 {
		t.Errorf("bytes = %d, queria 42000", plano.Bytes)
	}
	if len(cliente.metodos) == 0 || cliente.metodos[0] != http.MethodHead {
		t.Errorf("métodos = %v, queria começar por HEAD", cliente.metodos)
	}
}

func TestSemContentLengthOPlanoOmiteBytesEMantemCanInstall(t *testing.T) {
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	cliente := &clienteTamanho{headLen: -1, getStatus: http.StatusOK, getLen: -1}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode, http: cliente})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if !plano.CanInstall {
		t.Fatalf("falha de tamanho não pode derrubar CanInstall: %+v", plano)
	}
	if plano.Bytes != 0 {
		t.Errorf("bytes = %d, queria 0 quando o servidor não informa", plano.Bytes)
	}
}

func TestErroDeHEADNaoBloqueiaCanInstall(t *testing.T) {
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	cliente := &clienteTamanho{headErr: context.DeadlineExceeded, getErr: context.DeadlineExceeded}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode, http: cliente})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if !plano.CanInstall {
		t.Fatalf("erro de HEAD não pode bloquear CanInstall: %+v", plano)
	}
	if plano.Bytes != 0 {
		t.Errorf("bytes = %d, queria 0 após falha de sonda", plano.Bytes)
	}
}

func TestRangePreencheBytesQuandoHEADFalha(t *testing.T) {
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	cliente := &clienteTamanho{
		headStatus: http.StatusMethodNotAllowed,
		getStatus:  http.StatusPartialContent,
		getRange:   "bytes 0-0/98765",
		getLen:     1,
	}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode, http: cliente})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Bytes != 98765 {
		t.Errorf("bytes = %d, queria 98765 do Content-Range", plano.Bytes)
	}
}

func TestRangeSemContentRangeOmiteBytes(t *testing.T) {
	// 206 com Content-Length do trecho e sem Content-Range: chutar o
	// Content-Length subestimaria o artefato. Melhor omitir.
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	cliente := &clienteTamanho{
		headStatus: http.StatusMethodNotAllowed,
		getStatus:  http.StatusPartialContent,
		getLen:     1,
	}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode, http: cliente})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Bytes != 0 {
		t.Errorf("bytes = %d, queria 0 sem total em Content-Range", plano.Bytes)
	}
}

func TestDiskBytesContaArquivosDoDiretorioInstalado(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "bin")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	conteudo := []byte("abcdefghij") // 10 bytes
	if err := os.WriteFile(filepath.Join(sub, "agente.exe"), conteudo, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := dirDiskBytes(dir)
	want := int64(len(conteudo) + len("hi"))
	if got != want {
		t.Errorf("disk bytes = %d, queria %d", got, want)
	}
}

func TestDiskBytesIgnoraSymlinkParaFora(t *testing.T) {
	dir := t.TempDir()
	dentro := []byte("abc")
	if err := os.WriteFile(filepath.Join(dir, "local.bin"), dentro, 0o644); err != nil {
		t.Fatal(err)
	}
	fora := filepath.Join(t.TempDir(), "gordo.bin")
	gordo := bytes.Repeat([]byte("x"), 10_000)
	if err := os.WriteFile(fora, gordo, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "atalho.bin")
	if err := os.Symlink(fora, link); err != nil {
		t.Skipf("não deu para criar o link neste sistema: %v", err)
	}

	got := dirDiskBytes(dir)
	if got != int64(len(dentro)) {
		t.Errorf("disk bytes = %d, queria só o arquivo local (%d), sem seguir o symlink", got, len(dentro))
	}
}

func TestInstallationAtPreencheDiskBytes(t *testing.T) {
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agente},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})

	if _, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{
		Distribution: DistributionBinary,
		Origin:       opencodeURL,
		SHA256:       digestDe(pacote),
	}); err != nil {
		t.Fatalf("instalação: %v", err)
	}
	lida, ok := c.instalador.Installed(opencodeID)
	if !ok {
		t.Fatal("instalação sumiu")
	}
	if lida.DiskBytes == 0 {
		t.Fatal("DiskBytes deveria contar os arquivos do diretório instalado")
	}
}
