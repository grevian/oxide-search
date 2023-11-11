package query

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"oxide-search/meta"
	"time"

	"github.com/opensearch-project/opensearch-go"
	"github.com/sashabaranov/go-openai"
	"github.com/urfave/cli/v2"

	"oxide-search/search"
)

func Query(ctx *cli.Context) error {
	userQuery := "Tell me about fan power consumption in oxide racks"

	// Get an embedding of the users input query so we can find local context for its content
	openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	queryEmbeddingResponse, err := openaiClient.CreateEmbeddings(ctx.Context, openai.EmbeddingRequestStrings{
		Input: []string{userQuery},
		Model: openai.AdaEmbeddingV2,
	})
	if err != nil {
		return fmt.Errorf("failed to generate vectors for query: %w", err)
	}
	queryVector := queryEmbeddingResponse.Data[0].Embedding

	// Now search for neighbors of the embedding in our index to build context for the response
	client, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Addresses: []string{"https://localhost:9200"},
		Username:  "admin", // For testing only. Don't store credentials in code.
		Password:  "admin",
	})

	searchResults, err := search.QueryEmbedding(ctx.Context, client, queryVector, 10, 2)
	if err != nil {
		return fmt.Errorf("failed to query nearby vectors: %w", err)
	}

	contextMessages := meta.CreateConversation(meta.GetPrompt(), searchResults, userQuery)

	queryStart := time.Now()
	chatResponse, err := openaiClient.CreateChatCompletion(ctx.Context, openai.ChatCompletionRequest{
		Model:       openai.GPT4TurboPreview,
		Messages:    contextMessages,
		Temperature: 0.6,
		MaxTokens:   300,
	})
	if err != nil {
		return fmt.Errorf("failed to generate chat completion: %w", err)
	}

	fmt.Printf("Included Embeddings: %d, took %s seconds to generate a response \n", len(searchResults), time.Since(queryStart))
	fmt.Println("ChatResponse: " + chatResponse.Choices[0].Message.Content)

	return nil
}
