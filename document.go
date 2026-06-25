package sanur

type Document interface {
	Write(path string) error
	Bytes() ([]byte, error)
}
