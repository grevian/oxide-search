package main

import (
	"log"
	"os"
	"oxide-search/cmd/embeddings"
	"oxide-search/cmd/index"
	"oxide-search/cmd/query"
	"oxide-search/cmd/transcribe"

	"github.com/urfave/cli/v2"

	"oxide-search/cmd/download"
)

func main() {
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:    "download",
				Aliases: []string{"d"},
				Usage:   "Download podcast data to a local cache",
				Action:  download.Download,
			},
			{
				Name:    "transcribe",
				Aliases: []string{"t"},
				Usage:   "Submit downloaded files to whisper for transcription",
				Action:  transcribe.Transcribe,
			},
			{
				Name:    "embed",
				Aliases: []string{"e"},
				Usage:   "Generate embeddings from transcriptions",
				Action:  embeddings.Embed,
			},
			{
				Name:    "index",
				Aliases: []string{"i"},
				Usage:   "Load embeddings into an Opensearch index",
				Action:  index.Index,
			}, {
				Name:    "query",
				Aliases: []string{"q"},
				Usage:   "Make a query with embedding context",
				Action:  query.Query,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

}
