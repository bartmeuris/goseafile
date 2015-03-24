package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"bufio"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	
	"github.com/hashicorp/logutils"
	"github.com/bartmeuris/goseafile"
	"github.com/bartmeuris/progressio"
)

type Config struct {
	Url       string
	User      string
	Password  string
	AuthToken string
	Library   string
	Script    string
}

type CmdRun func(string, *goseafile.SeaFile, *Config, []string) error

var cmdList = map[string]CmdRun{
	"list":     listCmd,
	"listlibs": listLibsCmd,
	"upload":   uploadCmd,
	"download": downloadCmd,
	"setlib":   setVal,
	"lib":      setVal,
	"library":  setVal,
	"user":     setVal,
	"password": setVal,
	"pass":     setVal,
	"url":      setVal,
}

func setVal(cmd string, sf *goseafile.SeaFile, conf *Config, args []string) error {
	if len(args) == 1 {
		cval := args[0]
		switch(cmd) {
		case "setlib": fallthrough
		case "library": fallthrough
		case "lib":
			conf.Library = cval
		case "user":
			sf.User = cval
			sf.AuthToken = ""
		case "password": fallthrough
		case "pass":
			sf.Password = cval
			sf.AuthToken = ""
			cval = "********"
		case "url":
			sf.Url = cval
			sf.AuthToken = ""
		default:
			return fmt.Errorf("unknown command: %s", cmd)
		}
		log.Printf("# %s '%s'\n", cmd, cval)
	} else if len(args) == 0 {
		return fmt.Errorf("%s: expected an argument, none provided", cmd)
	} else {
		return fmt.Errorf("%s: too many arguments supplied, expected only one", cmd)
	}
	return nil
}

func listLibsCmd(cmd string, sf *goseafile.SeaFile, conf *Config, args []string) error {
	if v, err := sf.ListLibraries(); err == nil {
		log.Printf("# listlibs start\n")
		for _, e := range v {
			log.Printf("%s\n", e.Name)
		}
		log.Printf("# listlibs end\n")
	} else {
		return err
	}
	return nil
}

func showProgress(ch <- chan progressio.Progress, local, lib, remote string) {
	clearstr := ""
	//ss := Metric
	ss := progressio.IEC
	p := progressio.Progress{}
	for p = range ch {
		str := fmt.Sprintf("[%.2f%%] %s => %s::%s (%s/%s) (Speed: %s/sec, AVG: %s/sec) (Remaining: %s)",
			p.Percent,
			local,
			lib,
			remote,
			progressio.FormatSize(ss, p.Transferred, true),
			progressio.FormatSize(ss, p.TotalSize, true),
			progressio.FormatSize(ss, p.Speed, true),
			progressio.FormatSize(ss, p.SpeedAvg, true),
			progressio.FormatDuration(p.Remaining),
		)
		if (len(str) + 1) > len(clearstr) {
			clearstr = strings.Repeat(" ", len(str))
		}
		fmt.Printf("%s\r", clearstr)
		fmt.Printf("%s\r", str)
	}
	fmt.Printf("%s\r[DONE] %s => %s::%s (Size: %s, Time: %s, Speed: %s/sec) \n",
		clearstr,
		local,
		lib,
		remote,
		progressio.FormatSize(ss, p.TotalSize, true),
		progressio.FormatDuration(time.Since(p.StartTime)),
		progressio.FormatSize(ss, p.SpeedAvg, true),
	)
}

func uploadCmd(cmd string, sf *goseafile.SeaFile, conf *Config, args []string) error {
	if l, err := sf.GetLibrary(conf.Library); err != nil {
		return err
	} else if len(args) < 1 || len(args) > 2 {
		// Print help
		return fmt.Errorf("Useage: upload <source file> [remote destination file]")
	} else {
		var local, remote string
		
		local = args[0]
		if len(args) == 1 {
			remote = "/" + filepath.Base(filepath.Clean(args[0]))
		} else {
			remote = "/" + args[1]
			if strings.HasSuffix(remote, "/") {
				remote += path.Base(local)
			}
			remote = path.Clean(remote)
		}

		log.Printf("# Upload '%s' => '%s::%s'\n", local, conf.Library, remote)
		if f, ch, err := progressio.NewProgressFileReader(local); err != nil {
			return err
		} else {
			defer f.Close()
			go showProgress(ch, local, conf.Library, remote)
		
			if err := l.Upload(f, remote); err != nil {
				return err
			}
		}
	}
	return nil
}

func downloadCmd(cmd string, sf *goseafile.SeaFile, conf *Config, args []string) error {
	return fmt.Errorf("Not implemented")
}

func listCmd(cmd string, sf *goseafile.SeaFile, conf *Config, args []string) error {
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
			log.Printf("# list start { \"lib\": \"%s\", \"path\": \"%s\" }\n", conf.Library, arg)
			for _, f := range fl {
				log.Printf("%s\n", f.Name)
			}
			log.Printf("# list end\n")
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

func (c *Command) Run(sf *goseafile.SeaFile, conf *Config, args []string) error {
	if f, ok := cmdList[c.Cmd]; ok {
		return f(c.Cmd, sf, conf, args)
	}
	return fmt.Errorf("unknown command %s", c.Cmd)
}

func runScript(sf *goseafile.SeaFile, conf *Config, stream io.Reader, args ...string) error {
	rd := bufio.NewScanner(stream)
	// Prepare env: add arguments as $1, $2, ...
	for cnt, argv := range args {
		os.Setenv(fmt.Sprintf("%d", cnt+1), argv)
	}
	ln := 0
	for rd.Scan() {
		cmdstring := strings.TrimSpace(rd.Text())
		ln++
		if strings.HasPrefix(cmdstring, "#") {
			continue
		} else if cmdstring == "" {
			continue
		}
		cmdsplit := SplitQuoted(cmdstring)
		if len(cmdsplit) == 0 {
			continue
		}
		cmd := &Command{}

		// Replace environment variables in the commands
		args := make([]string, 0, len(cmdsplit)-1)
		if len(cmdsplit) > 1 {
			for _, s := range cmdsplit[1:] {
				args = append(args, os.ExpandEnv(s))
			}
		}

		if err := cmd.Set(cmdsplit[0]); err != nil {
			log.Fatalf("[ERROR] Script line %d: '%s': %s", ln, cmdstring, err)
		}
		if err := cmd.Run(sf, conf, args); err != nil {
			log.Fatalf("[ERROR] Script line %d: '%s': %s", ln, cmdstring, err)
		}
	}
	if err := rd.Err(); err != nil {
		return err
	}

	return nil
}

/////////////////////////////////////////////////////////////////////////////

func main() {
	var conf, cmdconf Config
	var conffile string
	var cmd Command
	var logDebug, logWarn bool
	
	filter := &logutils.LevelFilter{
		Levels: []logutils.LogLevel{"DEBUG", "WARN", "ERROR"},
		MinLevel: "ERROR",
		Writer: os.Stderr,
	}
	log.SetOutput(filter)

	cmd.Set("listlibs")
	
	flag.StringVar(&conffile, "conf", "", "a json file containing the url, user and password")
	flag.StringVar(&conf.Url, "url", "", "the API endpoint")
	flag.StringVar(&conf.User, "user", "", "the user")
	flag.StringVar(&conf.Password, "password", "", "the user's password")
	flag.StringVar(&conf.AuthToken, "token", "", "a valid auth token")
	flag.StringVar(&conf.Library, "lib", "My Library", "the library to work in")
	flag.StringVar(&conf.Script, "script", "", "A script to execute.")
	flag.BoolVar(&logDebug, "debug", false, "Output warning & debug statements.")
	flag.BoolVar(&logWarn, "warn", false, "Output warnings")
	flag.Var(&cmd, "cmd", "the command to execute. Available commands are: "+strings.Join(cmd.GetCmds(), ", "))
	flag.Parse()
	
	if logWarn {
		filter.MinLevel = "WARN"
	}
	if logDebug {
		filter.MinLevel = "DEBUG"
	}

	if conffile != "" {
		if f, err := ioutil.ReadFile(conffile); err != nil {
			log.Printf("[ERROR] Could not read config file %s: %s\n", conffile, err)
			os.Exit(1)
		} else if err := json.Unmarshal(f, &cmdconf); err != nil {
			log.Printf("[ERROR] Could not decode JSON in config file %s: %s\n", conffile, err)
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
	/*
	if conf.Url == "" {
		log.Fatalf("[ERROR] No valid seafile API endpoint specified\n")
	}
	*/
	sf := &goseafile.SeaFile{
		Url: conf.Url,
		User: conf.User,
		Password: conf.Password,
	}
	/*
		if !sf.Ping() {
			log.Fatalf("[ERROR] no ping reply from %s\n", conf.Url)
		}
	*/
	if conf.Script == "-" {
		if err := runScript(sf, &conf, os.Stdin, flag.Args()...); err != nil {
			log.Fatalf("[ERROR] Script error: %s\n", err)
		}
	} else if conf.Script != "" {
		// Read the script file
		if file, err := os.Open(conf.Script); err != nil {
			log.Fatalf("[ERROR] Could not open script '%s': %s\n", conf.Script, err)
		} else {
			defer file.Close()
			if err := runScript(sf, &conf, file, flag.Args()...); err != nil {
				log.Fatalf("[ERROR] Script error: %s\n", err)
			}
		}
	} else if err := cmd.Run(sf, &conf, flag.Args()); err != nil {
		log.Fatalf("[ERROR] Command %s returned an error: %s\n", cmd.String(), err)
	}
}
