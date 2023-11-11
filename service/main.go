package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"oxide-search/meta"
	"oxide-search/search"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/opensearch-project/opensearch-go"
	"github.com/sashabaranov/go-openai"
)

type QueryPayload struct {
	UserQuery string
}

type QueryResponse struct {
	UserQuery    string
	ChatResponse string
	Sources      []string
	Embeddings   []string
}

type server struct {
	searchClient *opensearch.Client
	openaiClient *openai.Client
	logger       *slog.Logger
}

func main() {
	searchClient, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Addresses: []string{"https://localhost:9200"},
		Username:  "admin", // For testing only. Don't store credentials in code.
		Password:  "admin",
	})
	if err != nil {
		panic(err)
	}

	s := &server{
		searchClient: searchClient,
		openaiClient: openai.NewClient(os.Getenv("OPENAI_API_KEY")),
		logger:       slog.Default(),
	}

	router := gin.Default()
	router.Use(cors.Default()) // Allow all origins

	router.POST("/chatQuery", s.queryHandler)

	err = router.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("Unexpected error in http server:", err)
	}
}

func (s *server) queryHandler(ctx *gin.Context) {
	var query QueryPayload
	if err := ctx.Bind(&query); err != nil {
		s.logger.ErrorContext(ctx, "failed to bind request to expected object", slog.Any("error", err))
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	queryEmbeddingResponse, err := s.openaiClient.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Input: []string{query.UserQuery},
		Model: openai.AdaEmbeddingV2,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong talking to openai"})
		s.logger.ErrorContext(ctx, "failed to generate embedding from user query", slog.Any("error", err))
		return
	}

	nearbyEmbeddings, err := search.QueryEmbedding(ctx, s.searchClient, queryEmbeddingResponse.Data[0].Embedding, 10, 2)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong talking to the search index"})
		s.logger.ErrorContext(ctx, "failed to locate nearby embeddings from user query", slog.Any("error", err))
		return
	}

	conversationContext := meta.CreateConversation(meta.GetPrompt(), nearbyEmbeddings, query.UserQuery)
	chatResponse, err := s.openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       openai.GPT4TurboPreview,
		Messages:    conversationContext,
		MaxTokens:   300,
		Temperature: 0.6,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong talking to openai"})
		s.logger.ErrorContext(ctx, "failed to generate a chat completion", slog.Any("error", err))
		return
	}

	sources := make([]string, len(nearbyEmbeddings))
	for i := range nearbyEmbeddings {
		sources[i] = fmt.Sprintf("%s - %s", nearbyEmbeddings[i].EpisodeData.Title, nearbyEmbeddings[i].EpisodeData.Link)
	}

	response := &QueryResponse{
		UserQuery:    query.UserQuery,
		ChatResponse: chatResponse.Choices[0].Message.Content,
		Sources:      sources,
		Embeddings:   []string{"Embedding id 31", "Embedding id 64"},
	}

	ctx.JSON(http.StatusOK, &response)
}
