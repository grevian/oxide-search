package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"

	"oxide-search/manifest"
)

const (
	indexName = "oxide"
)

type Document struct {
	manifest.EpisodeData
	Vectors []float32 `json:"vector_data"`
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

func QueryEmbedding(ctx context.Context, client *opensearch.Client, queryVector []float32, size int, K int) ([]Document, error) {
	queryBytes, err := json.Marshal(struct {
		Size  int   `json:"size"`
		Query query `json:"query"`
	}{
		Size: size,
		Query: query{
			Knn: knnSearch{
				vectorData: vectorData{
					Vector: queryVector,
					K:      K,
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vector query: %w", err)
	}

	searchReq := opensearchapi.SearchRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(queryBytes),
	}

	searchResponse, err := searchReq.Do(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to execute vector query: %w", err)
	}

	if searchResponse.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected response to vector query: %s", searchResponse.String())
	}

	bodyBytes, err := io.ReadAll(searchResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read search response body")
	}

	result := struct {
		Hits struct {
			Hits []struct {
				Id     string   `json:"_id"`
				Source Document `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}{}
	err = json.Unmarshal(bodyBytes, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize search results")
	}

	response := make([]Document, len(result.Hits.Hits))
	for i := range result.Hits.Hits {
		response[i] = result.Hits.Hits[i].Source
	}

	return response, nil
}
