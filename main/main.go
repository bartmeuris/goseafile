package main

import (
	"os"
	"log"
	"fmt"
	"flag"
	"path"
	"strings"
	"io/ioutil"
	"os/user"
	"time"
	"encoding/json"
	"github.com/bartmeuris/goseafile"
)

type Config struct {
	Url string
	User string
	Password string
	AuthToken string
	Library string
}

type CmdRun func(*goseafile.SeaFile, Config, []string) error

var cmdList = map[string]CmdRun {
	"list": listCmd,
	"listlibs": listLibsCmd,
	"upload": uploadCmd,
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
	s := strings.Split(arg, ";")
	if len(s) == 1 {
		// Only local file specified, target file is "/<filename>"
		return s[0], "/" + path.Base(s[0])
	} else if len(s) >= 2 {
		return s[0], path.Clean(s[1])
	}
	return "", ""
}

func uploadCmd(sf *goseafile.SeaFile, conf Config, args []string) error {
	ecnt := 0
	if l, err := sf.GetLibrary(conf.Library); err != nil {
		return err
	} else {
		for _, f := range args  {
			local, remote := getUplFiles(f)
			if local == "" || remote == "" {
				log.Printf("ERROR: invalid upload file provided: %s\n", f)
				ecnt++
				continue
			}
			if file, err := os.Open(local); err != nil {
				log.Printf("ERROR: Could not open file '%s': %s\n", f, err)
				ecnt++
				continue;
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
	for _, val := range(c.GetCmds()) {
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
	for k := range(cmdList) {
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
	Id string
	Token string
	TimeStamp time.Time
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
	// - 
	if u, err := user.Current(); err == nil {
		log.Printf("User homedir: %s\n", u.HomeDir)
	} else {
		log.Printf("ERROR: could not get user: %s\n", err)
	}

	if conf.AuthToken != "" {
		sf.AuthToken = conf.AuthToken
		if !sf.Authed() {
			conf.AuthToken = ""
			log.Printf("WARNING: provided auth token not valid!\n")
		}
	}
	if sf.AuthToken == "" && conf.User != "" && conf.Password != "" {
		if err := sf.Login(conf.User, conf.Password); err != nil {
			log.Printf("ERROR: no valid authentication found (auth error: %s)\n", err)
			return false
		}
	} else {
		log.Printf("ERROR: No valid authentication provided")
		return false
	}
	if !sf.Authed() {
		log.Printf("ERROR: auth verification failed.\n")
		return false
	}
	log.Printf("Auth succeeded!\n")
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
	flag.Var(&cmd, "cmd", "the command to execute. The commands are: "+ strings.Join(cmd.GetCmds(), ", "))
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
	if ! sf.Ping() {
		log.Fatalf("ERROR: no ping reply from %s\n", conf.Url)
	}
	
	if !tryAuth(sf, &conf, &cmdconf) {
		log.Fatalf("ERROR: Authentication failure")
	}

	if err := cmd.Run(sf, conf, flag.Args()); err != nil {
		log.Fatalf("ERROR: Command %s returned an error: %s\n", cmd.String(), err)
	}
}

