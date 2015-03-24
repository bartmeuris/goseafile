package goseafile

// This file implements authentication token caching for user/password
// combinations. It saves the authentication tokens, encrypted with the 
// SHA256 of the provided password, so you need to specify the user's
// password in order to unlock the authentication token. This prevents
// unnecessary posts of the user's password over public connections.
// Files are stored in ~/.config/goseafile/tokens.json (the user's home
// directory)

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"time"
)

type storedAuth struct {
	Token     []byte
	TimeStamp time.Time
	DecToken  string `json:"-"`
}

func encrypt(key, text []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	b := base64.StdEncoding.EncodeToString(text)
	ciphertext := make([]byte, aes.BlockSize+len(b))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], []byte(b))
	return ciphertext, nil
}

func decrypt(key, text []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(text) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	iv := text[:aes.BlockSize]
	text = text[aes.BlockSize:]
	cfb := cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(text, text)
	data, err := base64.StdEncoding.DecodeString(string(text))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (sf *SeaFile) getAESKey() []byte {
	// Create a AES key of 32 bytes to select AES-256
	//return []byte(strings.Repeat(conf.Password, (32 / len(conf.Password)) + 1))[0:32]
	ret := sha256.Sum256([]byte(sf.Password))
	return ret[0:32]
}

func (sf *SeaFile) getTokId() string {
	return fmt.Sprintf("%x", md5.Sum([]byte(sf.Url+"##"+sf.User)))
}

func (sf *SeaFile) getFilePath() string {
	if u, err := user.Current(); err == nil {
		return path.Clean(u.HomeDir + "/.config/goseafile/tokens.json")
	} else {
		log.Printf("[ERROR] could not get user: %s\n", err)
	}
	return ""
}

func (sf *SeaFile) getFileToken(file string) *storedAuth {
	if file == "" {
		return nil
	}
	key := sf.getAESKey()
	if key == nil {
		log.Printf("[WARN] Could not get AES key\n")
		return nil
	}
	if b, err := ioutil.ReadFile(file); err == nil {
		keys := make(map[string]storedAuth)
		if err := json.Unmarshal(b, &keys); err == nil {
			if k, ok := keys[sf.getTokId()]; ok {
				if btok, err := decrypt(key, k.Token); err != nil {
					log.Printf("[ERROR] could not decrypt token: %s\n", err)
				} else {
					k.DecToken = string(btok)
					return &k
				}
			} else {
				log.Printf("[WARN] Token not found for %s (%s@%s)\n", sf.getTokId(), sf.User, sf.Url)
			}
		} else {
			log.Printf("[WARN] Could not unmarshal '%s' contents: %s\n", file, err)
		}
	} else {
		log.Printf("[WARN] Could not read file '%s': %s\n", file, err)
	}
	return nil
}

func (sf *SeaFile) setFileToken(file, token string, expire time.Duration) error {
	if file == "" {
		return fmt.Errorf("file given was empty")
	}
	key := sf.getAESKey()
	if key == nil {
		return nil
	}
	if btok, err := encrypt(key, []byte(token)); err == nil {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(path.Dir(file), 0700); err != nil {
			return err
		}
		keys := make(map[string]storedAuth)
		// read the existing tokpath
		if b, err := ioutil.ReadFile(file); err == nil {
			if err := json.Unmarshal(b, &keys); err != nil {
				return err
			}
		} else {
			log.Printf("[WARN] Could not re-read file '%s' -- ignoring\n", file)
		}
		if expire > 0 {
			// remove expired tokens
			now := time.Now()
			for kt := range keys {
				if now.Sub(keys[kt].TimeStamp) > expire {
					log.Printf("[DEBUG] Removing expired key: %q\n", keys[kt])
					delete(keys, kt)
				}
			}
		}
		id := sf.getTokId()
		if token == "" {
			if _, ok := keys[id]; ok {
				delete(keys, id)
			}
		} else {
			// append new token
			keys[id] = storedAuth{
				Token:     btok,
				TimeStamp: time.Now(),
			}
		}
		// marshal and rewrite config file
		if bytes, err := json.Marshal(keys); err != nil {
			return err
		} else if err := ioutil.WriteFile(file, bytes, 0600); err != nil {
			return err
		}
	} else {
		return err
	}
	return nil
}

func (sf *SeaFile) doAuth(token string) bool {
	if token == "" {
		return false
	}
	sf.AuthToken = token
	if sf.Authed() {
		return true
	}
	log.Printf("[WARN] Token '%s' invalid\n", token)
	return false
}

func (sf *SeaFile) tryAuth() bool {
	// order to try authentication tokens:
	// - stored if valid/available AND user/pass combination is available
	// Cache auth tokens in ${HOME}/.config/goseafile/tokens.json
	// - encrypt with hash of password

	var tokpath string
	var maxtime = 30 * time.Minute
	log.Println("[DEBUG] Trying to authenticate...")
	tok := ""
	tokpath = sf.getFilePath()
	if st := sf.getFileToken(tokpath); st != nil {
		log.Printf("[DEBUG] Existing token found: %q\n", st)
		if time.Now().Sub(st.TimeStamp) < maxtime {
			log.Printf("[DEBUG] Token %s still valid!\n", st.DecToken)
			tok = st.DecToken
		} else {
			log.Printf("[WARN] Token found but not valid anymore\n")
		}
	}

	if sf.doAuth(tok) {
		return true
	} else if tok != "" {
		log.Printf("[WARN] Auth failed with stored token -- removing token '%s'", tok)
		if err := sf.setFileToken(tokpath, "", maxtime); err != nil {
			log.Printf("[WARN] Could not remove invalid auth token: %s\n", err)
		}
	}

	if sf.Password == "" {
		return false
	}
	if err := sf.Login(sf.User, sf.Password); err != nil {
		log.Printf("[ERROR] no valid authentication found (auth error: %s)\n", err)
		return false
	}
	log.Printf("[DEBUG] Auth succeeded!\n")
	// Now store the auth token
	if err := sf.setFileToken(tokpath, sf.AuthToken, maxtime); err != nil {
		log.Printf("[WARN] Could not save auth token: %s\n", err)
	}
	return true
}

