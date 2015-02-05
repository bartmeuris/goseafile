package goseafile

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type SeaFile struct {
	AuthToken string
	Url       string
	SaveAuth  bool
}

var AuthError = fmt.Errorf("authentication error")
var ThrottledError = fmt.Errorf("request was throttled")
var NotFoundError = fmt.Errorf("request was throttled")

func getError(status int, expectedstats []int) error {
	if expectedstats == nil {
		expectedstats = []int{ 200, 201, 202 }
	}
	for _, es := range expectedstats {
		if status == es {
			return nil
		}
	}
	switch (status) {
	case 301:
		// moved
	case 400:
		// Bad request
	case 403:
		return AuthError
	case 404:
		return NotFoundError
	case 409:
		// conflict
	case 429:
		return ThrottledError
	case 440:
		// repo password required
	case 441:
		// repo password magic required
	case 500:
		// Internal server error
	case 520:
		// Operation failed
	}
	return fmt.Errorf("unexpected http status: %d", status)
}

func (s *SeaFile) newReq(method, entry string) (*http.Request, error) {
	var rurl string
	if strings.HasPrefix(entry, "http") {
		rurl = entry
	} else {
		rurl = s.Url + "/" + strings.TrimPrefix(entry, "/")
	}
	//log.Printf("Sending request to: %s\n", rurl)
	if req, err := http.NewRequest(method, rurl, nil); err != nil {
		return nil, err
	} else {
		req.Header.Add("Accept", "application/json")
		if s.AuthToken != "" {
			req.Header.Add("Authorization", "Token "+s.AuthToken)
		}
		req.ParseForm()
		return req, nil
	}
}
func (s *SeaFile) reqResp(method, fnc string, form url.Values) (*http.Response, error) {
	if req, err := s.newReq(method, fnc); err != nil {
		return nil, err
	} else if req == nil {
		return nil, fmt.Errorf("request nil")
	} else {
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Body = ioutil.NopCloser(strings.NewReader(form.Encode()))
		}
		if resp, err := http.DefaultClient.Do(req); err != nil {
			return nil, err
		} else {
			return resp, nil
		}
	}
}
func (s *SeaFile) req(method, fnc string, form url.Values, rv interface{}) error {

	if resp, err := s.reqResp(method, fnc, form); err != nil {
		return err
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			if body, err := ioutil.ReadAll(resp.Body); err == nil {
				log.Printf("Request: %s %s\n", method, fnc)
				log.Printf("Body:\n---\n%s\n---\n", string(body))
			}
			return fmt.Errorf("request error: got status %d", resp.StatusCode)
		}
		if rv != nil {
			dec := json.NewDecoder(resp.Body)
			if err := dec.Decode(rv); err != nil {
				return err
			}
			/*
				// For debug purposes
				if body, err := ioutil.ReadAll(resp.Body); err != nil {
					return err
				} else {
					rd := strings.NewReader(string(body))
					//dec := json.NewDecoder(resp.Body)
					dec := json.NewDecoder(rd)
					if err := dec.Decode(rv); err != nil {
						log.Printf("Error decoding body: %s\n", err)
						log.Printf("Body:\n----\n%s\n----\n", string(body))
						return err
					}
				}
			*/
		}
		return nil
	}
}

func (s *SeaFile) Login(user string, password string) error {
	var tok struct {
		Token string
	}
	v := url.Values{
		"username": {user},
		"password": {password},
	}
	if err := s.req("POST", "/auth-token/", v, &tok); err != nil {
		return err
	}
	s.AuthToken = tok.Token
	return nil
}

func (s *SeaFile) Authed() bool {
	var rv string
	if err := s.req("GET", "/auth/ping/", nil, &rv); err != nil {
		log.Printf("ERROR: auth/ping failed: %s\n", err)
		s.AuthToken = ""
		return false
	} else if rv != "pong" {
		s.AuthToken = ""
		return false
	}
	return true
}

func (s *SeaFile) Ping() bool {
	if resp, err := s.reqResp("GET", "/ping/", nil); err != nil {
		return false
	} else {
		defer resp.Body.Close()
		// Ping is throttled, and can get a status 429 back. This is interpreted as "bad"
		if resp.StatusCode == 429 {
			// Valid, but throttled.
			return true
		} else if resp.StatusCode == 200 {
			// Output should be "pong"
			if b, err := ioutil.ReadAll(resp.Body); err != nil {
				return false
			} else if string(b) == "\"pong\"" {
				return true
			} else {
				log.Printf("ERROR: Unexpected value from ping: '%s'\n", string(b))
			}
		} else {
			log.Printf("ERROR: Unknown status code as response to ping: %d\n", resp.StatusCode)
		}
	}
	return false
}
