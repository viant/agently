package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/viant/agently-core/sdk"
	agentsvc "github.com/viant/agently-core/service/agent"
)

func main() {
	baseURL := flag.String("baseURL", "http://localhost:8080", "Agently base URL")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := sdk.NewHTTP(*baseURL)
	if err != nil {
		log.Fatalf("new http client failed: %v", err)
	}
	out, err := client.Query(ctx, &agentsvc.QueryInput{
		AgentID: "chatter",
		Query:   "hello from e2e",
		UserId:  "e2e-sdk",
	})
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}
	fmt.Printf("conversation: %s\n", out.ConversationID)
	fmt.Printf("message: %s\n", out.MessageID)
}
