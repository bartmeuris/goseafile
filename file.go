package goseafile

// File represents a file in a SeaFile library
type File struct {
	lib   *Library `json:"-"`
	Id    string
	Mtime int64
	Type  string
	Name  string
	Size  int64
}
