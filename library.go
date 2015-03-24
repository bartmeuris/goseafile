package goseafile

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
)

// Library represents a SeaFile library linked to a SeaFile instance
type Library struct {
	sf         *SeaFile `json:"-"`
	Permission string
	Encrypted  bool
	Mtime      int64
	Owner      string
	Id         string
	Size       int
	Name       string
	Virtual    bool
	Desc       string
	Root       string
}

// GetLibrary returns the library object for a library with the given name,
// or an error if it could not be found
func (s *SeaFile) GetLibrary(lib string) (*Library, error) {
	if libl, err := s.ListLibraries(); err != nil {
		return nil, err
	} else {
		for _, l := range libl {
			if l.Name == lib {
				return l, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find library '%s'", lib)
}

// ListLibraries returns a list with Library objects for each library available
// for the logged in user.
func (s *SeaFile) ListLibraries() ([]*Library, error) {
	var v []*Library
	if err := s.req("GET", "/repos/", nil, &v); err != nil {
		return nil, err
	}
	for i, _ := range v {
		v[i].sf = s
	}
	return v[0:], nil
}

func newLibrary(seafile *SeaFile, id string) *Library {
	lib := &Library{
		Id: id,
		sf: seafile,
	}
	if err := lib.Update(); err != nil {
		return nil
	}
	return lib
}

// GetOwner returns the owner from the library
func (l *Library) GetOwner() string {
	var own struct {
		Owner string
	}
	if err := l.sf.req("GET", "/repos/"+l.Id+"/owner/", nil, &own); err != nil {
		return ""
	} else {
		return own.Owner
	}
}

// upload with a pipewriter -> stream upload
func streamUpload(f io.Reader, filename, fieldname string, params map[string]string) (string, *io.PipeReader, error) {
	// First handle closable resources
	r, w := io.Pipe()
	rc, ok := f.(io.ReadCloser)
	if !ok && r != nil {
		rc = ioutil.NopCloser(r)
	}
	writer := multipart.NewWriter(w)
	ctype := writer.FormDataContentType()
	go func() {
		// This runs in background and writes to the PipeWriter
		// which blocks until something is read from the returned PipeReader
		// This allows the streaming to be efficient and prevents loading
		// the entire file in memory.
		defer rc.Close()
		defer writer.Close()
		
		// Send the file
		if pw, err := writer.CreateFormFile(fieldname, filename); err != nil {
			w.CloseWithError(err)
			return
		} else if _, err := io.Copy(pw, rc); err != nil {
			w.CloseWithError(err)
			return
		}
		// Write the parameters
		for key, val := range params {
			writer.WriteField(key, val)
		}
		// Don't use defer, it's possible we use the CloseWithError above
		w.Close()
	}()
	return ctype, r, nil
}


// Upload uploads data from an io.Reader to a file with the specified
// target path in the current library.
func (l *Library) Upload(fileio io.Reader, tgtpath string) error {
	// http://manual.seafile.com/develop/web_api.html#upload-file
	// 1. Get upload url
	var upllink string
	if err := l.sf.req("GET", "/repos/"+l.Id+"/upload-link/", nil, &upllink); err != nil {
		return err
	}

	// 2 - upload the file
	// https://github.com/gebi/go-fileupload-example/blob/master/main.go
	// http://matt.aimonetti.net/posts/2013/07/01/golang-multipart-file-upload-example/
	if req, err := l.sf.newReq("POST", upllink); err != nil {
		return err
	} else {
		tgtpath = filepath.Clean(tgtpath)
		fn := filepath.Base(tgtpath)
		tgtpath = filepath.Dir(tgtpath)
		if tgtpath == "" {
			tgtpath = "/"
		}
		formval := map[string]string{
			"parent_dir": tgtpath,
			"filename":   fn,
			"__fake": "fake field",
		}
		if ctype, r, err := streamUpload(fileio, fn, "file", formval); err != nil {
			return err
		} else {
			req.Body = r
			req.Header.Set("Content-Type", ctype)
			// Now send the request
			if resp, err := http.DefaultClient.Do(req); err != nil {
				return err
			} else if resp.Status != "200 OK" {
				return fmt.Errorf("expected return status '200 OK', got '%s'", resp.Status)
			}
		}
	}
	return nil
}

// List returns a list of all Files in the specified path.
func (l *Library) List(path string) ([]File, error) {
	var flist []File
	urls := "/repos/" + l.Id + "/dir/"
	if path != "" {
		urls = urls + "?p=" + url.QueryEscape(path)
	}
	if err := l.sf.req("GET", urls, nil, &flist); err != nil {
		return nil, err
	} else {
		for i, _ := range flist {
			flist[i].lib = l
		}
		return flist, nil
	}
}

// Update refreshes the Library information
func (l *Library) Update() error {
	if err := l.sf.req("GET", "/repos/"+l.Id+"/", nil, l); err != nil {
		return err
	}
	return nil
}
