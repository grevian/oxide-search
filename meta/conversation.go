package meta

import (
	"github.com/sashabaranov/go-openai"

	"oxide-search/search"
)

const (
	defaultBasePrompt = "You are a technical leader in the open source community, maybe affiliated with Oxide computers, and are discussing technical and social topics with friends and colleagues and answering questions from the audience"
)

func GetPrompt() string {
	return defaultBasePrompt
}

// CreateConversation combines the base prompt, embedding context, and the users query, to create a
// chat completion request, which can then be used to generate a longer form response
func CreateConversation(basePrompt string, embeddingContext []search.Document, userQuery string) []openai.ChatCompletionMessage {
	contextMessages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: basePrompt,
		},
	}

	for _, snippet := range embeddingContext {
		contextMessages = append(contextMessages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: snippet.EpisodeData.Transcript,
		})
	}

	contextMessages = append(contextMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userQuery,
	})

	return contextMessages
}
