package signal

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// registerRequest é o payload de POST /v1/register/{number}.
type registerRequest struct {
	UseVoice bool   `json:"use_voice"`
	Captcha  string `json:"captcha,omitempty"`
}

// Register inicia o registro de uma conta Signal via signal-cli-rest-api.
// mode: "sms" (padrão) ou "voice" para receber o código por ligação.
// captcha: token do Signal (signalcaptcha://...), exigido pela plataforma.
func Register(apiURL, number, mode, captcha string) error {
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
	log.Printf("[Signal] Register: POST %s (mode=%s, use_voice=%v, has_captcha=%v)",
		reqURL, mode, payload.UseVoice, captcha != "")

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[Signal] Register: erro de rede: %v", err)
		return fmt.Errorf("erro ao registrar número %s: %w", number, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Signal] Register: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 500))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("erro %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[Signal] Register: sucesso para %s", number)
	return nil
}

// Verify verifica o código recebido via SMS ou ligação.
func Verify(apiURL, number, code string) error {
	apiURL = strings.TrimRight(apiURL, "/")

	reqURL := fmt.Sprintf("%s/v1/register/%s/verify/%s",
		apiURL, url.PathEscape(number), url.PathEscape(code))
	log.Printf("[Signal] Verify: POST %s", reqURL)

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[Signal] Verify: erro de rede: %v", err)
		return fmt.Errorf("erro ao verificar número %s: %w", number, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Signal] Verify: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 500))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("erro %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("[Signal] Verify: número %s verificado com sucesso", number)
	return nil
}

// Unregister remove uma conta da signal-cli-rest-api.
// deleteLocalData: se true, também apaga os dados locais da conta.
func Unregister(apiURL, number string, deleteLocalData bool) error {
	apiURL = strings.TrimRight(apiURL, "/")

	// POST /v1/unregister/{number}
	reqURL := fmt.Sprintf("%s/v1/unregister/%s", apiURL, url.PathEscape(number))
	log.Printf("[Signal] Unregister: POST %s (deleteLocalData=%v)", reqURL, deleteLocalData)

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[Signal] Unregister: erro de rede: %v", err)
		return fmt.Errorf("erro ao descadastrar %s: %w", number, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Signal] Unregister: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 500))

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
		log.Printf("[Signal] Unregister: DELETE %s", delURL)

		delReq, err := http.NewRequest("DELETE", delURL, nil)
		if err != nil {
			return fmt.Errorf("descadastrado, mas erro ao limpar dados locais: %w", err)
		}

		delResp, err := httpClient.Do(delReq)
		if err != nil {
			return fmt.Errorf("descadastrado, mas erro ao limpar dados locais: %w", err)
		}
		defer delResp.Body.Close()

		delBody, _ := io.ReadAll(delResp.Body)
		log.Printf("[Signal] Unregister: delete local-data status=%d, body=%s", delResp.StatusCode, truncateStr(string(delBody), 300))
	}

	log.Printf("[Signal] Unregister: %s removido com sucesso", number)
	return nil
}

// GetLinkQRCode gera um QR code para vincular como dispositivo secundário.
// Retorna a imagem PNG em base64 (data URI).
// O signal-cli inicia o provisioning em background no servidor.
func GetLinkQRCode(apiURL, deviceName string) (string, error) {
	apiURL = strings.TrimRight(apiURL, "/")

	reqURL := fmt.Sprintf("%s/v1/qrcodelink?device_name=%s",
		apiURL, url.QueryEscape(deviceName))
	log.Printf("[Signal] GetLinkQRCode: GET %s", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("erro ao criar request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[Signal] GetLinkQRCode: erro: %v", err)
		return "", fmt.Errorf("erro ao gerar QR code: %w", err)
	}
	defer resp.Body.Close()

	imgBytes, _ := io.ReadAll(resp.Body)
	log.Printf("[Signal] GetLinkQRCode: status=%d, content-type=%s, body_len=%d",
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
func GetLinkRawURI(apiURL, deviceName string) (string, error) {
	apiURL = strings.TrimRight(apiURL, "/")

	reqURL := fmt.Sprintf("%s/v1/qrcodelink/raw?device_name=%s",
		apiURL, url.QueryEscape(deviceName))
	log.Printf("[Signal] GetLinkRawURI: GET %s", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("erro ao criar request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro ao gerar URI de vinculação: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Signal] GetLinkRawURI: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 300))

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
func ListAccounts(apiURL string) ([]string, error) {
	apiURL = strings.TrimRight(apiURL, "/")

	reqURL := apiURL + "/v1/accounts"
	log.Printf("[Signal] ListAccounts: GET %s", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar contas: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Signal] ListAccounts: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 300))

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
func CheckAPI(apiURL string) (map[string]interface{}, error) {
	apiURL = strings.TrimRight(apiURL, "/")

	// Tenta primeiro /v1/about (signal-cli-rest-api padrão)
	reqURL := apiURL + "/v1/about"
	log.Printf("[Signal] CheckAPI: tentando GET %s", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// Se /v1/about falhar, tenta /api/v1/about (caso haja um prefixo)
		log.Printf("[Signal] CheckAPI: GET %s falhou: %v", reqURL, err)

		// Tenta também a raiz para diagnóstico
		rootReq, rootErr := http.NewRequest("GET", apiURL, nil)
		if rootErr == nil {
			rootResp, rootRespErr := httpClient.Do(rootReq)
			if rootRespErr == nil {
				rootBody, _ := io.ReadAll(rootResp.Body)
				rootResp.Body.Close()
				log.Printf("[Signal] CheckAPI: GET %s retornou status %d, body=%s",
					apiURL, rootResp.StatusCode, truncateStr(string(rootBody), 500))
			} else {
				log.Printf("[Signal] CheckAPI: GET %s também falhou: %v", apiURL, rootRespErr)
			}
		}

		return nil, fmt.Errorf("signal-cli-rest-api não acessível em %s: %w", reqURL, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[Signal] CheckAPI: status=%d, body=%s", resp.StatusCode, truncateStr(string(respBody), 500))

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
