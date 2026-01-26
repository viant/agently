package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/viant/agently/client/sdk"
)

func main() {
	baseURL := flag.String("baseURL", "http://localhost:8080", "Agently base URL")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client := sdk.New(*baseURL)

	conv, err := client.CreateConversation(ctx, &sdk.CreateConversationRequest{Title: "e2e sdk"})
	if err != nil {
		log.Fatalf("create conversation failed: %v", err)
	}
	fmt.Printf("conversation: %s\n", conv.ID)

	msg, err := client.PostMessage(ctx, conv.ID, &sdk.PostMessageRequest{Content: "hello from e2e"})
	if err != nil {
		log.Fatalf("post message failed: %v", err)
	}
	fmt.Printf("message: %s\n", msg.ID)

	// Long-poll once to ensure /events is reachable.
	if _, err := client.PollEvents(ctx, conv.ID, "", []string{"text", "tool_op", "control"}, 2*time.Second); err != nil {
		log.Fatalf("poll events failed: %v", err)
	}
}
