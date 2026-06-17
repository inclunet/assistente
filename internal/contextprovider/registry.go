package contextprovider

import (
	"context"
	"log"
	"sort"
	"strings"
)

type Registry struct {
	providers []Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{}
	for _, provider := range providers {
		r.Register(provider)
	}
	return r
}

func (r *Registry) Register(provider Provider) {
	if provider == nil {
		return
	}
	r.providers = append(r.providers, provider)
	sort.SliceStable(r.providers, func(i, j int) bool {
		return r.providers[i].Name() < r.providers[j].Name()
	})
}

func (r *Registry) Providers() []Provider {
	if r == nil || len(r.providers) == 0 {
		return nil
	}
	return append([]Provider(nil), r.providers...)
}

func (r *Registry) Build(ctx context.Context, req BuildRequest) ([]Block, error) {
	if r == nil {
		return nil, nil
	}
	var blocks []Block
	for _, provider := range r.providers {
		providerBlocks, err := provider.Build(ctx, req)
		blocks = appendProviderBlocks(blocks, provider.Name(), providerBlocks)
		if err != nil {
			log.Printf("[context/providers] provider %q ignorado após erro: %v", provider.Name(), err)
			continue
		}
	}
	sortBlocks(blocks)
	return blocks, nil
}

func appendProviderBlocks(blocks []Block, providerName string, providerBlocks []Block) []Block {
	for _, block := range providerBlocks {
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		if block.Provider == "" {
			block.Provider = providerName
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func RenderBlocks(blocks []Block) []string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if trimmed := strings.TrimSpace(block.Content); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func sortBlocks(blocks []Block) {
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Volatility != blocks[j].Volatility {
			return volatilityRank(blocks[i].Volatility) < volatilityRank(blocks[j].Volatility)
		}
		if blocks[i].Priority != blocks[j].Priority {
			return blocks[i].Priority < blocks[j].Priority
		}
		if blocks[i].Provider != blocks[j].Provider {
			return blocks[i].Provider < blocks[j].Provider
		}
		return blocks[i].Name < blocks[j].Name
	})
}

func volatilityRank(value Volatility) int {
	switch value {
	case VolatilityStable:
		return 0
	case VolatilitySlowDynamic:
		return 1
	case VolatilityFastDynamic:
		return 2
	default:
		return 9
	}
}
