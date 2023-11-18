package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/opensearch-project/opensearch-go/v2"
	requestsigner "github.com/opensearch-project/opensearch-go/v2/signer/awsv2"
	"github.com/sashabaranov/go-openai"

	"oxide-search/meta"
	"oxide-search/search"
	"oxide-search/service/query"
)

type lambdaHandler struct {
	openaiClient *openai.Client
	searchClient *opensearch.Client
	logger       *slog.Logger
}

func (l *lambdaHandler) handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	l.logger.InfoContext(ctx, "Handler started", slog.Any("request", request), slog.String("body", request.Body))

	var queryBody query.ResponseBody
	err := json.Unmarshal([]byte(request.Body), &queryBody)
	if err != nil {
		l.logger.ErrorContext(ctx, "failed to parse request body", slog.Any("error", err))
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       err.Error(),
		}, err
	}

	queryEmbeddingResponse, err := l.openaiClient.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Input: []string{queryBody.UserQuery},
		Model: openai.AdaEmbeddingV2,
	})
	if err != nil {
		l.logger.ErrorContext(ctx, "failed to generate embedding from user query", slog.Any("error", err))
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "something went wrong talking to openai",
		}, err
	}

	nearbyEmbeddings, err := search.QueryEmbedding(ctx, l.searchClient, queryEmbeddingResponse.Data[0].Embedding, 10, 2)
	if err != nil {
		l.logger.ErrorContext(ctx, "failed to locate nearby embeddings from user query", slog.Any("error", err))
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "something went wrong talking to the search index",
		}, err
	}

	additionalEmbeddings, err := search.AddNearbySegments(ctx, l.searchClient, nearbyEmbeddings, 20)
	if err != nil {
		l.logger.ErrorContext(ctx, "failed to add additional embeddings from user query", slog.Any("error", err))
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "something went wrong talking to the search index",
		}, err
	}

	contextEmbeddings := append(nearbyEmbeddings, additionalEmbeddings...)
	conversationContext := meta.CreateConversation(meta.GetPrompt(), contextEmbeddings, queryBody.UserQuery)
	chatResponse, err := l.openaiClient.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       openai.GPT4TurboPreview,
		Messages:    conversationContext,
		MaxTokens:   300,
		Temperature: 0.6,
	})
	if err != nil {
		l.logger.ErrorContext(ctx, "failed to generate a chat completion", slog.Any("error", err))
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "something went wrong talking to openai",
		}, err
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

	responseBytes, err := json.MarshalIndent(response, "", " ")
	if err != nil {
		l.logger.ErrorContext(ctx, "serialize response", slog.Any("error", err))
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       "something went wrong building the response",
		}, err
	}
	return events.APIGatewayProxyResponse{
		Body:       string(responseBytes),
		StatusCode: 200,
	}, nil
}

func getOpenAIClient(apiKey string) *openai.Client {
	return openai.NewClient(apiKey)
}

func getOpensearchClient(awsCfg aws.Config, username string, password string) *opensearch.Client {

	signer, err := requestsigner.NewSignerWithService(awsCfg, "es")
	if err != nil {
		panic(err)
	}

	searchClient, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
		Addresses: []string{"https://" + os.Getenv("OPENSEARCH_HOST")},
		Username:  username,
		Password:  password,
		Signer:    signer,
	})
	if err != nil {
		panic(err)
	}
	return searchClient
}

func main() {
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}

	smClient := secretsmanager.NewFromConfig(awsCfg)

	openaiSecret, err := smClient.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String("openai-api-key"),
	})
	if err != nil {
		panic(err)
	}

	opensearchUsername, err := smClient.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String("os-username"),
	})
	if err != nil {
		panic(err)
	}

	opensearchPassword, err := smClient.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String("os-password"),
	})
	if err != nil {
		panic(err)
	}

	handler := lambdaHandler{
		openaiClient: getOpenAIClient(*openaiSecret.SecretString),
		searchClient: getOpensearchClient(awsCfg, *opensearchUsername.SecretString, *opensearchPassword.SecretString),
		logger:       slog.Default(),
	}

	lambda.Start(handler.handler)
}
