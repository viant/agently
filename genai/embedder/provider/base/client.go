package base

import "context"

type Embedder interface {
	Embed(ctx context.Context, data []string) (vector [][]float32, totalTokens int, err error)
}
type Client struct {
	Embedder
	UsageListener
}

func (c *Client) CreateEmbedding(ctx context.Context, data []string) ([][]float32, error) {
	vector, token, err := c.Embed(ctx, data)
	if err != nil {
		return nil, err
	}
	if c.UsageListener != nil {
		c.UsageListener(data, token)
	}
	return vector, nil
}
