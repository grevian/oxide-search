package transcribe

import (
	"fmt"
	"github.com/sashabaranov/go-openai"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	"oxide-search/meta"
)

const (
	dataDirectory = "data"
)

// gatherChunks collects any split files for the given UUID present in the data directory
func gatherChunks(GUID string) ([]string, error) {
	var transcriptionFiles []string
	entries, err := os.ReadDir(dataDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not read files in data directory: %w", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), fmt.Sprintf("%s-chunked", GUID)) {
			transcriptionFiles = append(transcriptionFiles, entry.Name())
		}
	}

	return transcriptionFiles, nil
}

// chunkFiles splits files into small enough pieces to be transcribed by Whisper, if necessary
func chunkFiles(episode *meta.EpisodeData) ([]string, error) {
	var transcriptionFiles []string

	fileInfo, err := os.Stat(filepath.Join(dataDirectory, episode.Filename))
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", episode.Filename, err)
	}

	// OpenAI has a file size limit of 25mb for whisper transcriptions
	filesizeMB := fileInfo.Size() / 1000 / 1000
	if filesizeMB > 25 {
		transcriptionFiles, err := gatherChunks(episode.GUID)
		if err != nil {
			return nil, fmt.Errorf("could not read files in data directory: %w", err)
		}
		if len(transcriptionFiles) > 0 {
			fmt.Println("File is already chunked")
			return transcriptionFiles, nil
		}

		fmt.Printf("File is %d MB, files over 25MB will need to be chunked\n", filesizeMB)
		fmt.Printf("Splitting into 20 minute chunks \n")
		cmd := exec.Command("ffmpeg", "-i", filepath.Join(dataDirectory, episode.Filename), "-f", "segment", "-segment_time", "1200", "-c", "copy", filepath.Join(dataDirectory, fmt.Sprintf("%s-chunked", episode.GUID)+"-%02d.mp3"))
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println(string(output))
			return nil, fmt.Errorf("failed to split files: %w", err)
		}

		return gatherChunks(episode.GUID)
	} else {
		transcriptionFiles = append(transcriptionFiles, episode.Filename)
		return transcriptionFiles, nil
	}
}

func Transcribe(ctx *cli.Context) error {
	manifest, err := meta.LoadManifest()
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	for _, episode := range manifest.Episodes {
		if episode.Transcript != "" {
			fmt.Printf("transcription already exists for episode %s (%s), skipping transcription\n", episode.GUID, episode.Title)
			continue
		}

		transcriptionFiles, err := chunkFiles(&episode)
		if err != nil {
			return err
		}
		fmt.Printf("transcribing the following files: %s \n", strings.Join(transcriptionFiles, ", "))

		openaiClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

		var transcript strings.Builder
		for _, file := range transcriptionFiles {
			response, err := openaiClient.CreateTranscription(ctx.Context, openai.AudioRequest{
				Model:    openai.Whisper1,
				FilePath: filepath.Join(dataDirectory, file),
				Prompt:   "",
				Language: "en",
			})
			if err != nil {
				return fmt.Errorf("unexpected error from whisper: %w", err)
			}
			transcript.WriteString(response.Text)
			transcript.WriteString(" ")
		}
		episode.Transcript = transcript.String()
		manifest.Episodes[episode.GUID] = episode
	}

	err = meta.UpdateManifest(manifest)
	if err != nil {
		return fmt.Errorf("failed to update manifest with transcriptions: %w", err)
	}

	return nil
}
