package goseafile

import (
	"fmt"
	"os"
	"io"
	"time"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
)

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

func NewLibrary(seafile *SeaFile, id string) *Library {
	lib := &Library{
		Id: id,
		sf: seafile,
	}
	if err := lib.Update(); err != nil {
		return nil
	}
	return lib
}

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

func copyPct(w io.Writer, r io.Reader, fsize int64, pctcb chan TransferProgress) (written int64, err error) {
	if pctcb == nil || fsize < 0 {
		return io.Copy(w, r)
	}
	// Copy the data ourself so we're able to provide a "percentage" feedback
	// This is largely a copy of the Go 1.4 io.Copy routine
	if pctcb != nil {
		defer close(pctcb)
	}
	buf := make([]byte, 32*1024)
	pwpct := float64(-1.0) // Previously written percentage
	tstart := time.Now()
	//pwritten := int64(0)
	bc := 0
	var blocktimes   [10]time.Time
	var blockwritten [10]int64
	for {
		//blockStart := time.Now()
		blocktimes[bc % len(blocktimes)] = time.Now()
		blockwritten[bc % len(blocktimes)] = written
		nr, er := r.Read(buf)
		if nr > 0 {
			nw, ew := w.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
				
				pct := float64(int64((float64(written) / float64(fsize)) * 10000.0)) / 100.0
				// Only update the percentage if it changed
				if pctcb != nil && pct != pwpct {
					lastt := time.Duration(0)
					if ! blocktimes[ (bc + 1) % len(blocktimes)].IsZero()  {
						lastt  = time.Since(blocktimes[ (bc + 1) % len(blocktimes)])
					}

					prog := &TransferProgress{
						StartTime: tstart,
						Transferred: written,
						TotalSize: fsize,
						Percent: pct,
					}

					if lastt != time.Duration(0) {
						prog.SpeedLastSec = int64( (float64(written - blockwritten[(bc+1) % len(blockwritten)]) / float64(lastt.Nanoseconds())) * float64(time.Second) )
					} else {
						prog.SpeedLastSec = 0
					}

					prog.SpeedAvgSec  = int64((float64(written) / float64(time.Since(tstart).Nanoseconds())) * float64(time.Second))
					// Round speed to kb/s to avoid spikes
					prog.Remaining = time.Duration(((fsize - written) / prog.SpeedAvgSec )) * time.Second
					select {
						case pctcb <- *prog:
							// We could write the percentage
							pwpct = pct
						default:
							// Writing the percentage would block - skip
					}
				}
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
		bc++
	}
	return written, err
}

// upload with a pipewriter -> stream upload
func streamUpload(f io.Reader, fsize int64, pctch chan TransferProgress, filename, fieldname string, params map[string]string) (string, *io.PipeReader, error) {
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
		if pw, err := writer.CreateFormFile(fieldname, filename); err != nil {
			w.CloseWithError(err)
			return
		} else if _, err := copyPct(pw, rc, fsize, pctch); err != nil {
			w.CloseWithError(err)
			return
		}
		// Write the parameters
		for key, val := range params {
			writer.WriteField(key, val)
		}
		w.Close()
	}()
	return ctype, r, nil
}

func (l *Library) UploadFile(file, targetpath string) error {
	if f, err := os.Open(file); err == nil {
		defer f.Close()
		// Get the filesize by seeking to the end of the file, and back to offset 0
		fsize, err := f.Seek(0, os.SEEK_END)
		if err != nil {
			return err
		}
		if _, err := f.Seek(0, os.SEEK_SET); err != nil {
			return err
		}
		return l.Upload(f, fsize, targetpath)
	} else {
		return err
	}
}

func (l *Library) Upload(fileio io.Reader, fsize int64, tgtpath string) error {
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
		}
		if ctype, r, err := streamUpload(fileio, fsize, l.sf.TransferPct,  fn, "file", formval); err != nil {
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

func (l *Library) Update() error {
	if err := l.sf.req("GET", "/repos/"+l.Id+"/", nil, l); err != nil {
		return err
	}
	return nil
}
