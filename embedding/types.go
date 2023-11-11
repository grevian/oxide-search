package embedding

type Storage struct {
	GUID       string
	VectorSize int
	Model      string
	Vector     []float32
	Content    string
}
