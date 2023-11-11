# Colonel Data Hallucination

Download the Oxide & Friends podcasts, transcribe them with whisper, break them into chunks for vectorization and load those chunks into opensearch, then
serve queries by vectorizing the user query and finding a few Knn values in opensearch, then load the conversation snippets into a chatCompletionContext
and forward the users query along with that context to GPT to give an expanded response (Kind of like RAG, but also kind of really shitty)

## Requirements

I tried pretty hard, but could not avoid a dependency on ffmpeg, the podcast MP3s are too big to submit to whisper in one shot and need to be split up, and I
could not find any pure go implementations of mp3 capabilities with the capability of doing this splitting. So, You'll need ffmpeg installed and in your PATH

Otherwise you need to set a `OPENAI_API_KEY` environment value for most of the commands

## Commands

`oxide-search download` downloads the oxide podcast MP3s and details from transistor.fm (Probably violating their ToS, sorry guys, the downloads do have a bit of throttling applied)
`oxide-search transcribe` submit the podcasts to openai's whisper model for transcription
`oxide-search embeddings` chunk the transcriptions up into 500~ word segments and have openai generate embedding vectors from those chunks
`oxide-search index` push the embeddings plus some details about their segments and the podcast into an opensearch index
`oxide-search query` submit a user query for vectorization, pull back some Knn matches from opensearch then construct a chatcompletion query with context from the transcriptions, before submitting the users query to openai for a response

