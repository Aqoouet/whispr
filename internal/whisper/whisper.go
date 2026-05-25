package whisper

type Request struct {
	RuntimeDir string
	ModelPath  string
	Device     string
	Language   string
	BeamSize   int
	InputWAV   string
}

type Backend interface {
	CUDAAvailable(runtimeDir string) (bool, error)
	Transcribe(req Request) (string, error)
}
