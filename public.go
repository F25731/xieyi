package main

type CatalogAPI struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Group     string   `json:"group"`
	Method    string   `json:"method"`
	SampleURL string   `json:"sampleUrl"`
	Order     int      `json:"order"`
	Domains   []string `json:"domains"`
}

func publicAPIs(cfg Config) []CatalogAPI {
	apis := enabledAPIs(cfg)
	out := make([]CatalogAPI, 0, len(apis))
	for _, api := range apis {
		out = append(out, CatalogAPI{
			ID:        api.ID,
			Name:      api.Name,
			Group:     api.Group,
			Method:    api.Method,
			SampleURL: api.SampleURL,
			Order:     api.Order,
			Domains:   api.Domains,
		})
	}
	return out
}
