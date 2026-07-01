package signal

import (
	"assistente/internal/logging"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

// getHTTPClient cria um cliente HTTP centralizado para as funções de registro
func getHTTPClient() *httpclient.Client {
	return httpclient.New(&httpclient.Config{
		CredentialManager: credentials.NewManager(nil),
		Timeout:           30 * time.Second,
	}, map[string]string{})
}

// registerRequest é o payload de POST /v1/register/{number}.
type registerRequest struct {
	UseVoice bool   `json:"use_voice"`
	Captcha  string `json:"captcha,omitempty"`
}

// Register inicia o registro de uma conta Signal via signal-cli-rest-api.
// mode: "sms" (padrão) ou "voice" para receber o código por ligação.
// captcha: token do Signal (signalcaptcha://...), exigido pela plataforma.
func Register(apiURL, number, mode, captcha, apiToken string) error {
	ctx := context.Background()
	client := getHTTPClient()
	apiURL = strings.TrimRight(apiURL, "/")

	payload := registerRequest{
		UseVoice: mode == "voice",
		Captcha:  captcha,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("erro ao serializar request: %w", err)
	}

	reqURL := fmt.Sprintf("%s/v1/register/%s", apiURL, url.PathEscape(number))
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Register: POST %s (number=%s, mode=%s, use_voice=%v, has_captcha=%v)",
		reqURL, maskIdentifier(number), mode, payload.UseVoice, captcha != "")

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(ctx, req)
	if err != nil {
		logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Register: erro de rede: %v", err)
		return fmt.Errorf("erro ao registrar número %s: %w", number, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Register: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 500))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("erro %d: %s", resp.StatusCode, string(respBody))
	}

	logging.Infof(context.Background(), "messaging.signal.register", "[Signal] Register: sucesso para %s", maskIdentifier(number))
	return nil
}

// Verify verifica o código recebido via SMS ou ligação.
func Verify(apiURL, number, code, apiToken string) error {
	ctx := context.Background()
	client := getHTTPClient()
	apiURL = strings.TrimRight(apiURL, "/")

	reqURL := fmt.Sprintf("%s/v1/register/%s/verify/%s",
		apiURL, url.PathEscape(number), url.PathEscape(code))
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Verify: POST %s (number=%s)", reqURL, maskIdentifier(number))

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, nil)
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(ctx, req)
	if err != nil {
		logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Verify: erro de rede: %v", err)
		return fmt.Errorf("erro ao verificar número %s: %w", number, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Verify: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 500))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("erro %d: %s", resp.StatusCode, string(respBody))
	}

	logging.Infof(context.Background(), "messaging.signal.register", "[Signal] Verify: número %s verificado com sucesso", maskIdentifier(number))
	return nil
}

// Unregister remove uma conta da signal-cli-rest-api.
// deleteLocalData: se true, também apaga os dados locais da conta.
func Unregister(apiURL, number string, deleteLocalData bool, apiToken string) error {
	ctx := context.Background()
	client := getHTTPClient()
	apiURL = strings.TrimRight(apiURL, "/")

	// POST /v1/unregister/{number}
	reqURL := fmt.Sprintf("%s/v1/unregister/%s", apiURL, url.PathEscape(number))
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Unregister: POST %s (number=%s, deleteLocalData=%v)", reqURL, maskIdentifier(number), deleteLocalData)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, nil)
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(ctx, req)
	if err != nil {
		logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Unregister: erro de rede: %v", err)
		return fmt.Errorf("erro ao descadastrar %s: %w", number, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Unregister: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 500))

	if resp.StatusCode >= 300 {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("erro %d: %s", resp.StatusCode, string(respBody))
	}

	// Apaga dados locais se solicitado
	if deleteLocalData {
		delURL := fmt.Sprintf("%s/v1/devices/%s/local-data", apiURL, url.PathEscape(number))
		logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] Unregister: DELETE %s", delURL)

		delReq, err := http.NewRequestWithContext(ctx, "DELETE", delURL, nil)
		if err != nil {
			return fmt.Errorf("descadastrado, mas erro ao limpar dados locais: %w", err)
		}

		delResp, err := client.Do(ctx, delReq)
		if err != nil {
			return fmt.Errorf("descadastrado, mas erro ao limpar dados locais: %w", err)
		}
		defer func() { _ = delResp.Body.Close() }()

		delBody, _ := io.ReadAll(delResp.Body)
		logging.Infof(context.Background(), "messaging.signal.register", "[Signal] Unregister: delete local-data status=%d, body=%s", delResp.StatusCode, truncateStr(string(delBody), 300))
	}

	logging.Infof(context.Background(), "messaging.signal.register", "[Signal] Unregister: %s removido com sucesso", maskIdentifier(number))
	return nil
}

// GetLinkQRCode gera um QR code para vincular como dispositivo secundário.
// Retorna a imagem PNG em base64 (data URI).
// O signal-cli inicia o provisioning em background no servidor.
func GetLinkQRCode(apiURL, deviceName, apiToken string) (string, error) {
	ctx := context.Background()
	client := getHTTPClient()
	apiURL = strings.TrimRight(apiURL, "/")

	reqURL := fmt.Sprintf("%s/v1/qrcodelink?device_name=%s",
		apiURL, url.QueryEscape(deviceName))
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] GetLinkQRCode: GET %s", reqURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("erro ao criar request: %w", err)
	}

	resp, err := client.Do(ctx, req)
	if err != nil {
		logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] GetLinkQRCode: erro: %v", err)
		return "", fmt.Errorf("erro ao gerar QR code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	imgBytes, _ := io.ReadAll(resp.Body)
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] GetLinkQRCode: status=%d, content-type=%s, body_len=%d",
		resp.StatusCode, resp.Header.Get("Content-Type"), len(imgBytes))

	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if json.Unmarshal(imgBytes, &apiErr) == nil && apiErr.Error != "" {
			return "", fmt.Errorf("%s", apiErr.Error)
		}
		return "", fmt.Errorf("erro %d: %s", resp.StatusCode, string(imgBytes))
	}

	if len(imgBytes) == 0 {
		return "", fmt.Errorf("resposta vazia ao gerar QR code")
	}

	b64 := base64.StdEncoding.EncodeToString(imgBytes)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/png"
	}

	return fmt.Sprintf("data:%s;base64,%s", contentType, b64), nil
}

// GetLinkRawURI gera a URI de vinculação como dispositivo secundário (sem QR code).
// Retorna a URI texto que pode ser usada para vincular o dispositivo.
func GetLinkRawURI(apiURL, deviceName, apiToken string) (string, error) {
	ctx := context.Background()
	client := getHTTPClient()
	apiURL = strings.TrimRight(apiURL, "/")

	reqURL := fmt.Sprintf("%s/v1/qrcodelink/raw?device_name=%s",
		apiURL, url.QueryEscape(deviceName))
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] GetLinkRawURI: GET %s", reqURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("erro ao criar request: %w", err)
	}

	resp, err := client.Do(ctx, req)
	if err != nil {
		return "", fmt.Errorf("erro ao gerar URI de vinculação: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] GetLinkRawURI: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 300))

	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return "", fmt.Errorf("%s", apiErr.Error)
		}
		return "", fmt.Errorf("erro %d: %s", resp.StatusCode, string(respBody))
	}

	// A resposta é JSON com o campo "device_link_uri"
	var result map[string]string
	if json.Unmarshal(respBody, &result) == nil {
		if uri, ok := result["device_link_uri"]; ok {
			return uri, nil
		}
	}

	// Fallback: tenta como texto puro
	return strings.TrimSpace(string(respBody)), nil
}

// ListAccounts retorna as contas já registradas/vinculadas na signal-cli-rest-api.
func ListAccounts(apiURL, apiToken string) ([]string, error) {
	ctx := context.Background()
	client := getHTTPClient()
	apiURL = strings.TrimRight(apiURL, "/")

	reqURL := apiURL + "/v1/accounts"
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] ListAccounts: GET %s", reqURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	resp, err := client.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar contas: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] ListAccounts: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 300))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro %d: %s", resp.StatusCode, string(respBody))
	}

	var accounts []string
	if err := json.Unmarshal(respBody, &accounts); err != nil {
		return nil, fmt.Errorf("erro ao parsear contas: %w", err)
	}

	return accounts, nil
}

// CheckAPI verifica se a signal-cli-rest-api está acessível e retorna informações.
func CheckAPI(apiURL, apiToken string) (map[string]interface{}, error) {
	ctx := context.Background()
	client := getHTTPClient()
	apiURL = strings.TrimRight(apiURL, "/")

	// Tenta primeiro /v1/about (signal-cli-rest-api padrão)
	reqURL := apiURL + "/v1/about"
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] CheckAPI: tentando GET %s", reqURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	resp, err := client.Do(ctx, req)
	if err != nil {
		// Se /v1/about falhar, tenta /api/v1/about (caso haja um prefixo)
		logging.Infof(context.Background(), "messaging.signal.register", "[Signal] CheckAPI: GET %s falhou: %v", reqURL, err)

		// Tenta também a raiz para diagnóstico
		rootReq, rootErr := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if rootErr == nil {
			rootResp, rootRespErr := client.Do(ctx, rootReq)
			if rootRespErr == nil {
				rootBody, _ := io.ReadAll(rootResp.Body)
				_ = rootResp.Body.Close()
				logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] CheckAPI: GET %s retornou status %d, body=%s",
					apiURL, rootResp.StatusCode, truncateStr(string(rootBody), 500))
			} else {
				logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] CheckAPI: GET %s também falhou: %v", apiURL, rootRespErr)
			}
		}

		return nil, fmt.Errorf("signal-cli-rest-api não acessível em %s: %w", reqURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	logging.Errorf(context.Background(), "messaging.signal.register", "[Signal] CheckAPI: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 500))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signal-cli-rest-api retornou status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	return result, nil
}

// truncateStr encurta uma string para uso em logs.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
