package vertexai

import (
	"fmt"
	"strconv"
	"strings"
)

// Request represents the request structure for VertexAI embeddings API
type Request struct {
	Instances []Instance `json:"instances"`
}

// Instance represents a single instance in the VertexAI embeddings API request
type Instance struct {
	Content string `json:"content"`
}

// Response represents the response structure from VertexAI embeddings API
type Response struct {
	Predictions []Prediction `json:"predictions"`
}

// Prediction represents a single prediction in the VertexAI embeddings API response
type Prediction struct {
	Embeddings Embeddings `json:"embeddings"`
}

// Embeddings represents the embeddings data in the VertexAI embeddings API response
type Embeddings struct {
	Values     []float32  `json:"values"`
	Statistics Statistics `json:"statistics"`
}

// Statistics represents the statistics data in the VertexAI embeddings API response
type Statistics struct {
	Truncated  bool `json:"truncated"`
	TokenCount int  `json:"token_count"`
}

type Project struct {
	ProjectNumber string `json:"projectNumber"`
	ProjectId     string `json:"projectId"` //project name
	Name          string `json:"name"`
	DisplayName   string `json:"displayName"`
}

func (p *Project) Id() int {
	parts := strings.Split(p.Name, "/")
	if len(parts) < 2 {
		fmt.Println("Invalid project name format")
		return 0
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		fmt.Println("Error parsing project ID:", err)
		return 0
	}
	return id
}
