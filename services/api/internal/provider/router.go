package provider

import (
	"context"
	"fmt"
)

// Router selects the correct provider adapter based on provider name and credential.
type Router struct {
	providers map[string]Provider
}

func NewRouter() *Router {
	return &Router{providers: make(map[string]Provider)}
}

func (r *Router) Register(name string, p Provider) {
	r.providers[name] = p
}

func (r *Router) Get(providerName string) (Provider, error) {
	p, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", providerName)
	}
	return p, nil
}

func (r *Router) Embed(ctx context.Context, providerName, text string) ([]float32, error) {
	p, err := r.Get(providerName)
	if err != nil {
		return nil, err
	}
	return p.Embed(ctx, text)
}
