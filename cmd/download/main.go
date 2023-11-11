package download

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/urfave/cli/v2"
	"oxide-search/manifest"
)

const (
	oxideRSSFeed  = "https://feeds.transistor.fm/oxide-and-friends.rss"
	dataDirectory = "data"
)

func Download(ctx *cli.Context) error {
	// TODO allow an argument to force rebuild, or to incrementally build data

	// Load our data manifest if it exists, if it doesn't we'll create a new one
	manifestData, err := manifest.Load()
	if err != nil {
		return fmt.Errorf("unexpected error reading download manifest: %w", err)
	}

	// Access the oxide and friends podcast RSS from transistor.fm
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(oxideRSSFeed)
	if err != nil {
		return fmt.Errorf("failed to process RSS from %s: %w", oxideRSSFeed, err)
	}

	manifestData.LastUpdated = feed.Updated

	// Download a few episodes from the RSS feed, and grab their description, link, and GUID too
	const maxEpisodes = 10
	var processedEpisodes = 0
	for _, item := range feed.Items {
		if _, exists := manifestData.Episodes[item.GUID]; exists {
			fmt.Printf("skipping existing item %s\n", item.GUID)
			continue
		}
		if processedEpisodes >= maxEpisodes {
			continue
		}
		processedEpisodes++

		if len(item.Enclosures) != 1 {
			return fmt.Errorf("unexpected number of enclosures (%d) in podcast item %s (%s)", len(item.Enclosures), item.GUID, item.Title)
		}

		fmt.Println("Downloading podcast mp3...")
		resp, err := http.Get(item.Enclosures[0].URL)
		if err != nil {
			return fmt.Errorf("failed to download podcast file %s: %w", item.Enclosures[0].URL, err)
		}
		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("unexpected http status %d while downloading file: %s", resp.StatusCode, string(bodyBytes))
		}
		filename := fmt.Sprintf("%s.mp3", item.GUID)
		file, err := os.Create(filepath.Join(dataDirectory, filename))
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				fmt.Printf("skipping existing file at %s\n", filename)
				continue
			}
		}

		var expectedLength int64
		foundLength, err := fmt.Sscanf(item.Enclosures[0].Length, "%d", &expectedLength)
		if err != nil || foundLength != 1 {
			return fmt.Errorf("failed to parse expected file length from string %s: %w", item.Enclosures[0].Length, err)
		}

		written, err := io.Copy(file, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to write file locally: %w", err)
		}
		if written != expectedLength {
			return fmt.Errorf("downloaded file was not the expected length: expected %d and got %d bytes", expectedLength, written)
		}
		_ = resp.Body.Close()
		manifestData.Episodes[item.GUID] = manifest.EpisodeData{
			Title:       item.Title,
			Description: item.Description,
			Link:        item.Link,
			Filename:    filename,
			GUID:        item.GUID,
			Published:   item.Published,
		}
		time.Sleep(time.Millisecond * 2000) // Be nice to transistor.fm
	}

	err = manifest.Update(manifestData)
	if err != nil {
		return fmt.Errorf("failed to write updated manifest: %w", err)
	}

	return nil
}
