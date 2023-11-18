package index

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"github.com/urfave/cli/v2"

	"oxide-search/embedding"
	"oxide-search/manifest"
	"oxide-search/search"
)

const (
	dataDirectory = "data"
)

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

		var bulkRequest bytes.Buffer

		for i, e := range embeddings {
			var doc search.Document
			doc.Id = fmt.Sprintf("episode-%s-embedding-%d", episode.GUID, i)
			doc.Title = episode.Title
			doc.GUID = episode.GUID
			doc.Published = episode.Published
			doc.Link = episode.Link
			doc.Description = episode.Description
			doc.VectorId = i

			doc.Transcript = e.Content
			doc.Vectors = e.Vector
			docBody, err := json.Marshal(doc)
			if err != nil {
				return fmt.Errorf("failed to build search document for episode %s, embedding %d: %w", episode.GUID, i, err)
			}
			indexRequestBody, err := json.Marshal(
				struct {
					Index struct {
						IndexName string `json:"_index"`
						Id        string `json:"_id"`
					} `json:"index"`
				}{
					struct {
						IndexName string `json:"_index"`
						Id        string `json:"_id"`
					}{
						"oxide",
						fmt.Sprintf("episode-%s-embedding-%d", episode.GUID, i),
					},
				})
			if err != nil {
				return fmt.Errorf("failed to build indexing directive for episode %s, embedding %d: %w", episode.GUID, i, err)
			}

			bulkRequest.WriteString(string(indexRequestBody) + "\n")
			bulkRequest.WriteString(string(docBody) + "\n")
		}

		req := opensearchapi.BulkRequest{
			Index: "oxide",
			Body:  bytes.NewReader(bulkRequest.Bytes()),
		}

		insertResponse, err := req.Do(ctx.Context, client)
		if err != nil {
			return fmt.Errorf("error indexing embedding for episode %s %w", episode.GUID, err)
		}
		if insertResponse.StatusCode >= 300 {
			return fmt.Errorf("unexpected indexing response writing embeddings for episode %s: %s", episode.GUID, insertResponse.String())
		}

		fmt.Printf("Indexed %d embedding documents for %s (%s)\n", len(embeddings), episode.GUID, episode.Title)
	}

	return nil
}
