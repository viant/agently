package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/viant/agently-core/sdk"
	agentsvc "github.com/viant/agently-core/service/agent"
)

func main() {
	baseURL := flag.String("baseURL", "http://localhost:8080", "Agently base URL")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := sdk.NewHTTP(*baseURL)
	if err != nil {
		log.Fatalf("new http client failed: %v", err)
	}

	conversationID := ""
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		out, err := client.Query(ctx, &agentsvc.QueryInput{
			AgentID:        "chatter",
			ConversationID: conversationID,
			Query:          line,
			UserId:         "e2e-sdk-cli",
		})
		if err != nil {
			log.Fatalf("query failed: %v", err)
		}
		conversationID = out.ConversationID
		if conversationID != "" {
			fmt.Printf("conversation:%s\n", conversationID)
		}
		fmt.Printf("sent:%s\n", out.MessageID)
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("read stdin failed: %v", err)
	}
}
