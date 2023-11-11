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
	"time"

	"oxide-search/meta"
)

type searchDocument struct {
	meta.EpisodeData
	Vectors []float32 `json:"vector_data"`
}

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

	// The opensearch API in go is... very awkward
	queryBytes, err := json.Marshal(struct {
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
	})
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

	// Now combine our base prompt, the context from our index, and the users query, to create a
	// chat completion request, which we can then submit to OpenAI to generate a response
	basePrompt := "You are a technical leader in the open source community, maybe affiliated with Oxide computers, and are discussing technical and social topics with friends and colleagues and answering questions from the audience"
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

	fmt.Printf("Included Embeddings: %d, took %s seconds to generate a response \n", len(result.Hits.Hits), time.Since(queryStart))
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
