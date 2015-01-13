package goseafile


import (
	"net/http"
	"net/url"
	"encoding/json"
	"log"
	"fmt"
	"io/ioutil"
	"strings"
)

type SeaFile struct {
	AuthToken string
	Url string
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
		if (s.AuthToken != "") {
			req.Header.Add("Authorization", "Token " + s.AuthToken)
		}
		req.ParseForm()
		return req, nil
	}
}
func (s *SeaFile) reqResp(method, fnc string, form url.Values) (*http.Response, error) {
	if req, err := s.newReq(method, fnc); err != nil {
		return nil, err
	} else if (req == nil ) {
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
			return fmt.Errorf("Request error: got status %d", resp.StatusCode)
		}
		if (rv != nil) {
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
	if !s.Ping() {
		// Prevent sending credentials to a wrong url
		return fmt.Errorf("server did not respond correctly to ping")
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
	var rv string
	if err := s.req("GET", "/ping/", nil, &rv); err != nil {
		log.Printf("ERROR: ping failed: %s\n", err)
		return false
	} else if rv != "pong" {
		return false
	}
	return true
}

