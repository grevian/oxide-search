package embeddings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/urfave/cli/v2"

	"oxide-search/embedding"
	"oxide-search/manifest"
)

const (
	dataDirectory = "data"

	// Definitely worth experimenting with this
	vectorSize = 500
)

func Embed(ctx *cli.Context) error {
	manifestData, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("failed to load data manifest: %w", err)
	}

	// Split the transcripts up into 500~ word chunks and submit them to openai to create embeddings, which we
	// then store alongside the files
	openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	for _, episode := range manifestData.Episodes {

		stringField := strings.Fields(episode.Transcript)
		fmt.Printf("generating vectors for %d batches of %d words at a time from the transcript of %d words\n", len(stringField)/vectorSize, vectorSize, len(episode.Transcript))

		index := 0
		embeddings := make([]embedding.Storage, 0)
		for index < len(stringField)-vectorSize {

			// Take overlapping sets of windows to use as embeddings, for example of our index is 1000 and our window size is 500
			var batches []string
			// Take the window on the current index, 1000-1500
			if index+vectorSize <= len(stringField) {
				batches = append(batches, strings.Join(stringField[index:index+vectorSize], " "))
			}
			// Slide the window forwards and take the terms 1250-1750
			if index+vectorSize+(vectorSize/2) <= len(stringField) {
				batches = append(batches, strings.Join(stringField[index+(vectorSize/2):index+vectorSize+(vectorSize/2)], " "))
			}
			// Slide the window backwards and take the terms 750-1250
			if index > vectorSize {
				batches = append(batches, strings.Join(stringField[index-(vectorSize/2):index+(vectorSize/2)], " "))
			}

			index += vectorSize

			embeddingResponse, err := openaiClient.CreateEmbeddings(ctx.Context, openai.EmbeddingRequestStrings{
				Input: batches,
				Model: openai.AdaEmbeddingV2,
			})
			if err != nil {
				e := fmt.Errorf("Error generating embeddings for chunk at indices %d:%d of episode %s: %w\n", index, index+vectorSize, episode.GUID, err)
				fmt.Println(e)
				continue
			}

			for i := range embeddingResponse.Data {
				embeddings = append(embeddings, embedding.Storage{
					GUID:       episode.GUID,
					VectorSize: vectorSize,
					Model:      "text-embedding-ada-002",
					Vector:     embeddingResponse.Data[i].Embedding,
					Content:    batches[i],
				})
			}

		}

		embeddingBytes, err := json.MarshalIndent(embeddings, "", " ")
		if err != nil {
			return fmt.Errorf("failed to serialize embeddings data for episode %s: %w", episode.GUID, err)
		}

		err = os.WriteFile(filepath.Join(dataDirectory, fmt.Sprintf("%s.embeddings.json", episode.GUID)), embeddingBytes, 0644)
		if err != nil {
			return fmt.Errorf("failed to write embeddings data for episode %s: %w", episode.GUID, err)
		}
	}

	return nil
}
