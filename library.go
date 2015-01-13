package goseafile

import (
	"log"
	"io"
	"fmt"
	"io/ioutil"
	"net/http"
	"mime/multipart"
	"path/filepath"
)

type Library struct {
	sf *SeaFile `json:"-"`
	Permission string
	Encrypted bool
	Mtime int64
	Owner string
	Id string
	Size int
	Name string
	Virtual bool
	Desc string
	Root string
}


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

func (l *Library) GetOwner() string {
	var own struct {
		Owner string
	}
	if err := l.sf.req("GET", "/repos/" + l.Id + "/owner/", nil, &own); err != nil {
		return ""
	}
	return own.Owner
}


func streamUpload(w *io.PipeWriter, r io.Reader, filename, fieldname string, params map[string]string ) error {
	// First handle closable resources
	rc, ok := r.(io.ReadCloser)
	if !ok && r != nil {
		rc = ioutil.NopCloser(r)
	}

	log.Printf("Creating multipart writer...\n")
	writer := multipart.NewWriter(w)
	// Now write the file
	go func() {
		defer rc.Close()
		defer writer.Close()
		log.Printf("Creating and uploading formfile...\n")
		if pw, err := writer.CreateFormFile(fieldname, filename); err != nil {
			w.CloseWithError(err)
			return
		} else if _, err := io.Copy(pw, rc); err != nil {
			w.CloseWithError(err)
			return
		}
		log.Printf("Adding Parameters...\n")
		// Write the parameters
		for key, val := range params {
			writer.WriteField(key, val)
		}
		log.Printf("Uploading done.\n")
		w.Close()
	}()
	return nil
}

func (l *Library) Upload(path string, file io.Reader) error {
	// http://manual.seafile.com/develop/web_api.html#upload-file
	// 1. Get upload url
	var upllink string
	if err := l.sf.req("GET", "/repos/" + l.Id + "/upload-link/", nil, &upllink); err != nil {
		return err
	}

	// 2 - upload the file
	// https://github.com/gebi/go-fileupload-example/blob/master/main.go
	// http://matt.aimonetti.net/posts/2013/07/01/golang-multipart-file-upload-example/
	log.Printf("Uploading file to: %s\n", upllink)
	if req, err := l.sf.newReq("POST", upllink); err != nil {
		return err
	} else {
		path = filepath.Clean(path)
		fn := filepath.Base(path)
		path = filepath.Dir(path)
		if path == "" {
			path = "/"
		}

		// Continue with upload!
		log.Printf("Creating Pipe...\n")
		r,w := io.Pipe()
		if err := streamUpload(w, file, fn, "file", map[string]string{ "parent_dir": path, "filename": fn }); err != nil {
			return err
		}
		req.Body = r
		// Now send the request
		log.Printf("Sending request...\n")
		if resp, err := http.DefaultClient.Do(req); err != nil {
			return err
		} else {
			log.Printf("Response status: %s\n", resp.Status)
		}
	}
	return nil
}

