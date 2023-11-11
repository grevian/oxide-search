package embeddings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/urfave/cli/v2"

	"oxide-search/embedding"
	"oxide-search/meta"
)

const (
	dataDirectory = "data"
	vectorSize    = 500
)

func Embed(ctx *cli.Context) error {
	manifest, err := meta.LoadManifest()
	if err != nil {
		return fmt.Errorf("failed to load data manifest: %w", err)
	}

	openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	for _, episode := range manifest.Episodes {
		if _, err := os.Stat(filepath.Join(dataDirectory, fmt.Sprintf("%s.embeddings.json", episode.GUID))); !errors.Is(err, os.ErrNotExist) {
			fmt.Println("embeddings file already exists, skipping recreation")
			continue
		}

		stringField := strings.Fields(episode.Transcript)
		fmt.Printf("generating vectors for %d batches of %d words at a time from the transcript of %d words\n", len(stringField)/vectorSize, vectorSize, len(episode.Transcript))

		index := 0
		embeddings := make([]embedding.Storage, 0)
		for index < len(stringField)-vectorSize {
			stringBatch := stringField[index : index+vectorSize]
			index += vectorSize

			embeddingResponse, err := openaiClient.CreateEmbeddings(ctx.Context, openai.EmbeddingRequestStrings{
				Input: []string{strings.Join(stringBatch, " ")},
				Model: openai.AdaEmbeddingV2,
				User:  "josh-hayes-sheen",
			})
			if err != nil {
				e := fmt.Errorf("Error generating embeddings for chunk at indices %d:%d of episode %s: %w\n", index, index+vectorSize, episode.GUID, err)
				fmt.Println(e)
				continue
			}

			embeddings = append(embeddings, embedding.Storage{
				GUID:       episode.GUID,
				VectorSize: vectorSize,
				Model:      embeddingResponse.Model.String(),
				Vector:     embeddingResponse.Data[0].Embedding,
				Content:    strings.Join(stringBatch, " "),
			})
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
