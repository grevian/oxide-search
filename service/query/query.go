package query

type RequestPayload struct {
	UserQuery string
}

type ResponseBody struct {
	UserQuery    string
	ChatResponse string
	Sources      []string
	Embeddings   []string
}
