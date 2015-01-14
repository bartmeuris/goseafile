package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/bartmeuris/goseafile"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"strings"
	"time"
)

type Config struct {
	Url       string
	User      string
	Password  string
	AuthToken string
	Library   string
}

type CmdRun func(*goseafile.SeaFile, Config, []string) error

var cmdList = map[string]CmdRun{
	"list":     listCmd,
	"listlibs": listLibsCmd,
	"upload":   uploadCmd,
	"download": downloadCmd,
}

func listLibsCmd(sf *goseafile.SeaFile, conf Config, args []string) error {
	if v, err := sf.ListLibraries(); err == nil {
		fmt.Printf("# listlibs start\n")
		for _, e := range v {
			fmt.Printf("%s\n", e.Name)
		}
		fmt.Printf("# listlibs end\n")
	} else {
		return err
	}
	return nil
}

func getUplFiles(arg string) (string, string) {
	s := strings.Split(arg, "=")
	if len(s) == 1 {
		// Only local file specified, target file is "/<filename>"
		return s[0], "/" + path.Base(s[0])
	} else if len(s) >= 2 {
		return s[0], path.Clean("/" + s[1])
	}
	return "", ""
}

func uploadCmd(sf *goseafile.SeaFile, conf Config, args []string) error {
	ecnt := 0
	if l, err := sf.GetLibrary(conf.Library); err != nil {
		return err
	} else {
		for _, f := range args {
			local, remote := getUplFiles(f)
			if local == "" || remote == "" {
				log.Printf("ERROR: invalid upload file provided: %s\n", f)
				ecnt++
				continue
			}
			fmt.Printf("Upload '%s' => lib: '%s', file: '%s'\n", local, conf.Library, remote)
			if file, err := os.Open(local); err != nil {
				log.Printf("ERROR: Could not open file '%s': %s\n", f, err)
				ecnt++
				continue
			} else if err := l.Upload(remote, file); err != nil {
				log.Printf("ERROR: Could not upload file: %s\n", err)
				ecnt++
				file.Close()
			} else {
				log.Printf("Uploading file succeeded!\n")
				file.Close()
			}
		}
	}
	return nil
}

func downloadCmd(sf *goseafile.SeaFile, conf Config, args []string) error {
	return fmt.Errorf("Not implemented")
}

func listCmd(sf *goseafile.SeaFile, conf Config, args []string) error {
	if l, err := sf.GetLibrary(conf.Library); err != nil {
		return err
	} else {
		arg := ""
		if len(args) > 0 {
			arg = args[0]
		}
		if fl, err := l.List(arg); err != nil {
			return err
		} else {
			fmt.Printf("# list start { \"lib\": \"%s\", \"path\": \"%s\" }\n", conf.Library, arg)
			for _, f := range fl {
				fmt.Printf("%s\n", f.Name)
			}
			fmt.Printf("# list end\n")
		}
	}
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Implement the command type
type Command struct {
	Cmd string
}

func (c *Command) String() string {
	return c.Cmd
}

func (c *Command) Set(s string) error {
	found := false
	s = strings.ToLower(s)
	for _, val := range c.GetCmds() {
		if val == s {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown command: '%s', expected one of: %s", strings.ToLower(s), strings.Join(c.GetCmds(), ", "))
	}
	c.Cmd = s
	return nil
}

func (c *Command) GetCmds() []string {
	r := make([]string, 0, len(cmdList))
	for k := range cmdList {
		r = append(r, k)
	}
	return r
}

func (c *Command) Run(sf *goseafile.SeaFile, conf Config, args []string) error {
	if f, ok := cmdList[c.Cmd]; ok {
		return f(sf, conf, args)
	}
	return fmt.Errorf("unknown command %s", c.Cmd)
}

/////////////////////////////////////////////////////////////////////////////

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

func doAuth(sf *goseafile.SeaFile, token string) bool {
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

func getFilePath() string {
	if u, err := user.Current(); err == nil {
		return path.Clean(u.HomeDir + "/.config/goseafile/tokens.json")
	} else {
		log.Printf("ERROR: could not get user: %s\n", err)
	}
	return ""
}

func getFileToken(file string, conf *Config) *StoredAuth {
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

func setFileToken(file, token string, expire time.Duration, conf *Config) error {
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

func tryAuth(sf *goseafile.SeaFile, conf, cmdc *Config) bool {
	// order to try authentication tokens:
	// - stored if valid/available AND user/pass combination is available
	// - commandline if provided
	// - from config file if available
	// When token auth fails: use username/password:
	// - from commandline if provided
	// - from config file if provided
	// --------
	// Cache auth tokens in ${HOME}/.config/goseafile/tokens.json
	// - encrypt with hash of password

	var tokpath string
	var maxtime = 15 * time.Minute
	tok := ""
	tokpath = getFilePath()
	if st := getFileToken(tokpath, conf); st != nil {
		//log.Printf("Existing token found: %q\n", st)
		if time.Now().Sub(st.TimeStamp) < maxtime {
			//log.Printf("Token %s still valid!\n", st.DecToken)
			tok = st.DecToken
		} else {
			log.Printf("WARN: Token found but not valid anymore\n")
		}
	}

	if doAuth(sf, tok) {
		return true
	} else if tok != "" {
		log.Printf("WARN: Auth failed with stored token -- removing token '%s'", tok)
		if err := setFileToken(tokpath, "", maxtime, conf); err != nil {
			log.Printf("WARN: Could not remove invalid auth token: %s\n", err)
		}
	}

	if doAuth(sf, cmdc.AuthToken) {
		return true
	} else if doAuth(sf, conf.AuthToken) {
		return true
	}
	if conf.Password == "" {
		return false
	}
	if err := sf.Login(conf.User, conf.Password); err != nil {
		log.Printf("ERROR: no valid authentication found (auth error: %s)\n", err)
		return false
	}
	log.Printf("Auth succeeded!\n")
	// Now store the auth token
	if err := setFileToken(tokpath, sf.AuthToken, maxtime, conf); err != nil {
		log.Printf("WARN: Could not save auth token: %s\n", err)
	}
	return true
}

func main() {
	var conf, cmdconf Config
	var conffile string
	var cmd Command
	cmd.Set("listlibs")

	flag.StringVar(&conffile, "conf", "", "a json file containing the url, user and password")
	flag.StringVar(&conf.Url, "url", "", "the API endpoint")
	flag.StringVar(&conf.User, "user", "", "the user")
	flag.StringVar(&conf.Password, "password", "", "the user's password")
	flag.StringVar(&conf.AuthToken, "token", "", "a valid auth token")
	flag.StringVar(&conf.Library, "lib", "My Library", "the library to work in")
	flag.Var(&cmd, "cmd", "the command to execute. The commands are: "+strings.Join(cmd.GetCmds(), ", "))
	flag.Parse()

	if conffile != "" {
		// Read values from the config file
		if f, err := ioutil.ReadFile(conffile); err != nil {
			log.Printf("ERROR: Could not read config file %s: %s\n", conffile, err)
			os.Exit(1)
		} else if err := json.Unmarshal(f, &cmdconf); err != nil {
			log.Printf("ERROR: Could not decode JSON in config file %s: %s\n", conffile, err)
			os.Exit(1)
		}
		if cmdconf.Url != "" {
			conf.Url = cmdconf.Url
		}
		if cmdconf.User != "" {
			conf.User = cmdconf.User
		}
		if cmdconf.Password != "" {
			conf.Password = cmdconf.Password
		}
		if cmdconf.Library != "" {
			conf.Library = cmdconf.Library
		}
	}
	if conf.Url == "" {
		log.Fatalf("ERROR: No valid seafile API endpoint specified\n")
	}
	sf := &goseafile.SeaFile{Url: conf.Url}
	/*
	if !sf.Ping() {
		log.Fatalf("ERROR: no ping reply from %s\n", conf.Url)
	}
	*/

	if !tryAuth(sf, &conf, &cmdconf) {
		log.Fatalf("ERROR: Authentication failure")
	}

	if err := cmd.Run(sf, conf, flag.Args()); err != nil {
		log.Fatalf("ERROR: Command %s returned an error: %s\n", cmd.String(), err)
	}
}
