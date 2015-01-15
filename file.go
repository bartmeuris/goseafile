package goseafile

type File struct {
	lib   *Library `json:"-"`
	Id    string
	Mtime int64
	Type  string
	Name  string
	Size  int64
}
