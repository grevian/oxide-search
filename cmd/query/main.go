package query

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/sashabaranov/go-openai"
	"github.com/urfave/cli/v2"
	"io"
	"net/http"
	"os"

	"oxide-search/meta"
)

type searchDocument struct {
	meta.EpisodeData
	Vectors []float32 `json:"vector_data"`
}

func Query(ctx *cli.Context) error {

	userQuery := "Tell me a story about fan power consumption in oxide racks"

	openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	embeddingResponse, err := openaiClient.CreateEmbeddings(ctx.Context, openai.EmbeddingRequestStrings{
		Input: []string{userQuery},
		Model: openai.AdaEmbeddingV2,
		User:  "josh-hayes-sheen",
	})
	if err != nil {
		return fmt.Errorf("failed to generate vectors for query: %w", err)
	}

	queryVector := embeddingResponse.Data[0].Embedding

	client, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Addresses: []string{"https://localhost:9200"},
		Username:  "admin", // For testing only. Don't store credentials in code.
		Password:  "admin",
	})

	s := struct {
		Size  int   `json:"size"`
		Query query `json:"query"`
	}{
		Size: 10,
		Query: query{
			Knn: knnSearch{
				vectorData: vectorData{
					Vector: queryVector,
					K:      2,
				},
			},
		},
	}
	queryBytes, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal vector query: %w", err)
	}

	searchReq := opensearchapi.SearchRequest{
		Index: []string{"oxide"},
		Body:  bytes.NewReader(queryBytes),
	}

	searchResponse, err := searchReq.Do(ctx.Context, client)
	if err != nil {
		return fmt.Errorf("failed to execute vector query: %w", err)
	}

	if searchResponse.StatusCode != 200 {
		return fmt.Errorf("unexpected response to vector query: %s", searchResponse.String())
	}

	bodyBytes, err := io.ReadAll(searchResponse.Body)
	if err != nil {
		return fmt.Errorf("failed to read search response body")
	}

	result := struct {
		Hits struct {
			Hits []struct {
				Id     string         `json:"_id"`
				Source searchDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}{}
	err = json.Unmarshal(bodyBytes, &result)
	if err != nil {
		return fmt.Errorf("failed to deserialize search results")
	}

	basePrompt := "A technical leader is discussing technical and social topics with friends and colleagues"
	contextMessages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: basePrompt,
		},
	}

	for _, snippet := range result.Hits.Hits {
		contextMessages = append(contextMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: snippet.Source.Transcript,
		})
	}

	contextMessages = append(contextMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userQuery,
	})

	chatResponse, err := openaiClient.CreateChatCompletion(ctx.Context, openai.ChatCompletionRequest{
		Model:       openai.GPT3Dot5Turbo,
		Messages:    contextMessages,
		Temperature: 0.6,
	})
	if err != nil {
		return fmt.Errorf("failed to generate chat completion: %w", err)
	}

	fmt.Println("ChatResponse: " + chatResponse.Choices[0].Message.Content)

	return nil
}

// Opensearch API is stupid :(
type query struct {
	Knn knnSearch `json:"knn"`
}

type knnSearch struct {
	vectorData `json:"vector_data"`
}

type vectorData struct {
	Vector []float32 `json:"vector"`
	K      int       `json:"k"`
}
