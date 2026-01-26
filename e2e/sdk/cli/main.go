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

	"github.com/viant/agently/client/sdk"
)

func main() {
	baseURL := flag.String("baseURL", "http://localhost:8080", "Agently base URL")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := sdk.New(*baseURL)
	conv, err := client.CreateConversation(ctx, &sdk.CreateConversationRequest{Title: "e2e sdk cli"})
	if err != nil {
		log.Fatalf("create conversation failed: %v", err)
	}
	fmt.Printf("conversation:%s\n", conv.ID)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		msg, err := client.PostMessage(ctx, conv.ID, &sdk.PostMessageRequest{Content: line})
		if err != nil {
			log.Fatalf("post message failed: %v", err)
		}
		fmt.Printf("sent:%s\n", msg.ID)
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("read stdin failed: %v", err)
	}
}
