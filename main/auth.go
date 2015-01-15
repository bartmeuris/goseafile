package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/bartmeuris/goseafile"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"time"
)

type StoredAuth struct {
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

func DoAuth(sf *goseafile.SeaFile, token string) bool {
	if token == "" {
		return false
	}
	sf.AuthToken = token
	if sf.Authed() {
		return true
	}
	fmt.Printf("WARN: Token '%s' invalid\n", token)
	return false
}

func getAESKey(conf *Config) []byte {
	// Create a AES key of 32 bytes to select AES-256
	//return []byte(strings.Repeat(conf.Password, (32 / len(conf.Password)) + 1))[0:32]
	ret := sha256.Sum256([]byte(conf.Password))
	return ret[0:32]
}

func getTokId(conf *Config) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(conf.Url+"##"+conf.User)))
}

func GetFilePath() string {
	if u, err := user.Current(); err == nil {
		return path.Clean(u.HomeDir + "/.config/goseafile/tokens.json")
	} else {
		log.Printf("ERROR: could not get user: %s\n", err)
	}
	return ""
}

func GetFileToken(file string, conf *Config) *StoredAuth {
	if file == "" {
		return nil
	}
	key := getAESKey(conf)
	if key == nil {
		log.Printf("Could not get AES key\n")
		return nil
	}
	if b, err := ioutil.ReadFile(file); err == nil {
		keys := make(map[string]StoredAuth)
		if err := json.Unmarshal(b, &keys); err == nil {
			if k, ok := keys[getTokId(conf)]; ok {
				if btok, err := decrypt(key, k.Token); err != nil {
					log.Printf("Could decrypt token: %s\n", err)
				} else {
					k.DecToken = string(btok)
					return &k
				}
			} else {
				log.Printf("Token not found for %s (%s@%s)\n", getTokId(conf), conf.User, conf.Url)
			}
		} else {
			log.Printf("Could not unmarshal '%s' contents: %s\n", file, err)
		}
	} else {
		log.Printf("Error reading file '%s': %s\n", file, err)
	}
	return nil
}

func SetFileToken(file, token string, expire time.Duration, conf *Config) error {
	if file == "" {
		return fmt.Errorf("file given was empty")
	}
	key := getAESKey(conf)
	if key == nil {
		return nil
	}
	if btok, err := encrypt(key, []byte(token)); err == nil {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(path.Dir(file), 0700); err != nil {
			return err
		}
		keys := make(map[string]StoredAuth)
		// read the existing tokpath
		if b, err := ioutil.ReadFile(file); err == nil {
			if err := json.Unmarshal(b, &keys); err != nil {
				return err
			}
		} else {
			log.Printf("WARN: Could not re-read file '%s' -- ignoring\n", file)
		}
		if expire > 0 {
			// remove expired tokens
			now := time.Now()
			for kt := range keys {
				if now.Sub(keys[kt].TimeStamp) > expire {
					log.Printf("Removing expired key: %q\n", keys[kt])
					delete(keys, kt)
				}
			}
		}
		id := getTokId(conf)
		if token == "" {
			if _, ok := keys[id]; ok {
				delete(keys, id)
			}
		} else {
			// append new token
			keys[id] = StoredAuth{
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
