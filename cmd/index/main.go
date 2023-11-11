package index

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"net/http"
	"os"
	"path/filepath"

	"github.com/opensearch-project/opensearch-go"
	"github.com/urfave/cli/v2"

	"oxide-search/embedding"
	"oxide-search/meta"
)

const (
	dataDirectory = "data"
)

type searchDocument struct {
	meta.EpisodeData
	Vectors []float32 `json:"vector_data"`
}

func Index(ctx *cli.Context) error {
	manifest, err := meta.LoadManifest()
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

	for _, episode := range manifest.Episodes {
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
