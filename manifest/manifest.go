package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dataDirectory = "data"
	manifestName  = "manifest.json"
)

type EpisodeData struct {
	Title       string
	Description string
	Link        string
	Filename    string
	GUID        string
	Published   string
	Transcript  string
}

type Downloads struct {
	LastUpdated string
	Episodes    map[string]EpisodeData
}

func Load() (*Downloads, error) {
	var manifest Downloads
	manifestBytes, err := os.ReadFile(filepath.Join(dataDirectory, manifestName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("unexpected error reading download manifest: %w", err)
	}

	if errors.Is(err, os.ErrNotExist) {
		// If the manifest doesn't exist, create it in place
		manifest.Episodes = make(map[string]EpisodeData)
	} else {
		err = json.Unmarshal(manifestBytes, &manifest)
		if err != nil {
			return nil, fmt.Errorf("unexpected error parsing download manifest: %w", err)
		}
	}

	return &manifest, nil
}

func Update(manifest *Downloads) error {
	manifestBytes, err := json.MarshalIndent(manifest, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest for updating: %w", err)
	}
	err = os.WriteFile(filepath.Join(dataDirectory, manifestName), manifestBytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated manifest: %w", err)
	}

	return nil
}
