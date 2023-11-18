package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/sashabaranov/go-openai"

	"oxide-search/meta"
	"oxide-search/search"
	"oxide-search/service/query"
)

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
	var queryBody query.RequestPayload
	if err := ctx.Bind(&queryBody); err != nil {
		s.logger.ErrorContext(ctx, "failed to bind request to expected object", slog.Any("error", err))
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	queryEmbeddingResponse, err := s.openaiClient.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Input: []string{queryBody.UserQuery},
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

	additionalEmbeddings, err := search.AddNearbySegments(ctx, s.searchClient, nearbyEmbeddings, 20)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong talking to the search index"})
		s.logger.ErrorContext(ctx, "failed to add additional embeddings from user query", slog.Any("error", err))
		return
	}

	contextEmbeddings := append(nearbyEmbeddings, additionalEmbeddings...)
	conversationContext := meta.CreateConversation(meta.GetPrompt(), contextEmbeddings, queryBody.UserQuery)
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

	response := &query.ResponseBody{
		UserQuery:    queryBody.UserQuery,
		ChatResponse: chatResponse.Choices[0].Message.Content,
		Sources:      sources,
		Embeddings:   []string{"Embedding id 31", "Embedding id 64"},
	}

	ctx.JSON(http.StatusOK, &response)
}
