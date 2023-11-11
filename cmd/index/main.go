package index

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"github.com/urfave/cli/v2"

	"oxide-search/embedding"
	"oxide-search/manifest"
)

const (
	dataDirectory = "data"
)

type searchDocument struct {
	manifest.EpisodeData
	Vectors []float32 `json:"vector_data"`
}

func Index(ctx *cli.Context) error {
	manifestData, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("failed to load data manifest: %w", err)
	}

	client, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Addresses: []string{"https://localhost:9200"},
		Username:  "admin", // For testing only. Don't store credentials in code.
		Password:  "admin",
	})

	// For each episode, load the embeddings and index them into opensearch in a document that includes their
	// text content and some episode information
	for _, episode := range manifestData.Episodes {
		embeddingBytes, err := os.ReadFile(filepath.Join(dataDirectory, fmt.Sprintf("%s.embeddings.json", episode.GUID)))
		if err != nil {
			return fmt.Errorf("could not load episode embeddings: %w", err)
		}

		var embeddings []embedding.Storage
		err = json.Unmarshal(embeddingBytes, &embeddings)
		if err != nil {
			return fmt.Errorf("could not load episode embeddings: %w", err)
		}

		for i, e := range embeddings {
			var doc searchDocument
			doc.Title = episode.Title
			doc.GUID = episode.GUID
			doc.Published = episode.Published
			doc.Link = episode.Link
			doc.Description = episode.Description

			doc.Transcript = e.Content
			doc.Vectors = e.Vector
			docBody, err := json.MarshalIndent(doc, "", " ")
			if err != nil {
				return fmt.Errorf("failed to build search document for episode %s, embedding %d: %w", episode.GUID, i, err)
			}

			req := opensearchapi.IndexRequest{
				Index:      "oxide",
				DocumentID: fmt.Sprintf("episode-%s-embedding-%d", episode.GUID, i),
				Body:       bytes.NewReader(docBody),
			}

			insertResponse, err := req.Do(ctx.Context, client)
			if err != nil {
				return fmt.Errorf("error indexing embedding for episode %s, embedding %d: %w", episode.GUID, i, err)
			}
			if insertResponse.StatusCode >= 300 {
				return fmt.Errorf("unexpected indexing response writing embedding for episode %s, embedding %d: %s", episode.GUID, i, insertResponse.String())
			}
		}

		fmt.Printf("Indexed %d embedding documents for %s (%s)\n", len(embeddings), episode.GUID, episode.Title)
	}

	return nil
}
