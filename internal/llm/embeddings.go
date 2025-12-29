package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// EmbeddingsService gerencia a geração e comparação de embeddings
type EmbeddingsService struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// EmbeddingsConfig configuração do serviço
type EmbeddingsConfig struct {
	APIKey  string
	BaseURL string // Opcional, padrão OpenAI
	Model   string // Opcional, padrão text-embedding-3-small
}

// NewEmbeddingsService cria um novo serviço de embeddings
func NewEmbeddingsService(config EmbeddingsConfig) *EmbeddingsService {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	model := config.Model
	if model == "" {
		model = "text-embedding-3-small" // 1536 dimensões, $0.02/1M tokens
	}

	return &EmbeddingsService{
		apiKey:     config.APIKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// embeddingRequest representa a requisição para a API
type embeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

// embeddingResponse representa a resposta da API
type embeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Generate gera o embedding para um texto
func (s *EmbeddingsService) Generate(text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("texto vazio")
	}

	reqBody := embeddingRequest{
		Input: text,
		Model: s.model,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar requisição: %w", err)
	}

	req, err := http.NewRequest("POST", s.baseURL+"/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erro HTTP %d: %s", resp.StatusCode, string(body))
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(body, &embResp); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("resposta sem embeddings")
	}

	return embResp.Data[0].Embedding, nil
}

// CosineSimilarity calcula a similaridade de cosseno entre dois vetores
// Retorna valor entre -1 e 1, onde 1 = idênticos, 0 = ortogonais, -1 = opostos
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// SimilarityResult representa um resultado de busca por similaridade
type SimilarityResult struct {
	ID         uint    `json:"id"`
	Similarity float32 `json:"similarity"`
}

// FindMostSimilar encontra os IDs mais similares a um embedding de query
// embeddings é um map de ID para embedding
// Retorna os topK resultados ordenados por similaridade decrescente
func FindMostSimilar(queryEmbedding []float32, embeddings map[uint][]float32, topK int) []SimilarityResult {
	if len(embeddings) == 0 || len(queryEmbedding) == 0 {
		return nil
	}

	results := make([]SimilarityResult, 0, len(embeddings))

	for id, emb := range embeddings {
		similarity := CosineSimilarity(queryEmbedding, emb)
		results = append(results, SimilarityResult{
			ID:         id,
			Similarity: similarity,
		})
	}

	// Ordena por similaridade decrescente
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Similarity > results[i].Similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if topK > len(results) {
		topK = len(results)
	}

	return results[:topK]
}

// EmbeddingWithID associa um ID a um embedding
type EmbeddingWithID struct {
	ID        uint
	Embedding []float32
}

// FindMostSimilarSlice versão que recebe slice em vez de map
func FindMostSimilarSlice(queryEmbedding []float32, items []EmbeddingWithID, topK int, minSimilarity float32) []SimilarityResult {
	if len(items) == 0 || len(queryEmbedding) == 0 {
		return nil
	}

	results := make([]SimilarityResult, 0, len(items))

	for _, item := range items {
		if len(item.Embedding) == 0 {
			continue
		}
		similarity := CosineSimilarity(queryEmbedding, item.Embedding)
		if similarity >= minSimilarity {
			results = append(results, SimilarityResult{
				ID:         item.ID,
				Similarity: similarity,
			})
		}
	}

	// Ordena por similaridade decrescente
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Similarity > results[i].Similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if topK > len(results) {
		topK = len(results)
	}

	return results[:topK]
}

