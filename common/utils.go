package common

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// supported OS types
type OperatingSystem string

const (
	Linux  OperatingSystem = "Linux"
	Darwin OperatingSystem = "Darwin"
)

func (operatingSystem OperatingSystem) String() string {
	switch operatingSystem {
	case Linux:
		return "linux"
	case Darwin:
		return "darwin"
	}

	return "unsupported"
}

// GetOS get os type
func GetOS() (operatingSystem string) {
	return runtime.GOOS
}

// RunCommand runs the cmd and returns the combined stdout and stderr
func RunCommand(cmd ...string) (string, error) {
	output, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %q: %s (%s)", strings.Join(cmd, " "), err, output)
	}
	return string(output), nil
}

// IsCommandAvailable checks to see if a binary is available in the current path
func IsCommandAvailable(name string) bool {
	if _, err := exec.LookPath(name); err != nil {
		return false
	}
	return true
}

// timestampFile return a unique timestamped filename
func timestampFile(filename string) string {
	return fmt.Sprintf(filename + "-" + time.Now().Format("20060102150405"))
}

// ValidateIp ensures a valid IP4/IP6 address is provided
func ValidateIp(ip string) error {
	if ip := net.ParseIP(ip); ip != nil {
		return nil
	}
	return fmt.Errorf("%s is not a valid v4 or v6 IP", ip)
}

// diffConfig diffs the contents of two files
func diffConfig(oldCfg, newCfg string) bool {
	cfgOld, err := ioutil.ReadFile(oldCfg)
	if err != nil {
		log.Fatalf("unable to read file: %v\n", err)
	}
	cfgNew, err := ioutil.ReadFile(newCfg)
	if err != nil {
		log.Fatalf("unable to read file: %v\n", err)
	}
	if string(cfgOld) != string(cfgNew) {
		return false
	}
	return true
}

func FileToString(file string) string {
	fileContent, err := ioutil.ReadFile(file)
	if err != nil {
		log.Errorf("unable to read the file [%s] %v\n", file, err)
		return ""
	}
	return string(fileContent)
}

// CopyFile source destination
func CopyFile(src, dst string) error {
	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer w.Close()
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()

	_, err = io.Copy(w, r)
	return err
}

// GetIPv4Linux returns the first address from hostname -I
func GetIPv4Linux() (string, error) {
	out, err := RunCommand("hostname", "-I")
	if err != nil {
		return "", err
	}
	items := strings.Split(string(out), " ")
	if len(items) == 0 {
		return "", fmt.Errorf("failed to exec hostname")
	}

	return items[0], nil
}

// GetDarwinIPv4 returns the first address from ipconfig getifaddr en0
func GetDarwinIPv4() (string, error) {
	osxIP, err := RunCommand("ipconfig", "getifaddr", "en0")
	if err != nil {
		return "", err
	}
	return osxIP, nil
}

// GetPubIP retrieves current global IP address using https://checkip.amazonaws.com/
func GetPubIP() (string, error) {
	c := http.DefaultClient
	req, err := http.NewRequest("GET", "https://checkip.amazonaws.com/", nil)
	if err != nil {
		return "", err
	}
	res, err := c.Do(req)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= http.StatusBadRequest {
		defer func() {
			_ = res.Body.Close()
		}()
		return "", fmt.Errorf("%s: %s %s", res.Status, req.Method, req.URL)
	}
	ip, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response from https://checkip.amazonaws.com: %w", err)
	}
	return strings.TrimSpace(string(ip)), nil
}

func IsNAT(nodeOS string) (bool, error) {
	var hostIP string
	var err error
	if nodeOS == Darwin.String() {
		hostIP, err = GetDarwinIPv4()
		if err != nil {
			return false, err
		}
	}
	if nodeOS == Linux.String() {
		hostIP, err = GetIPv4Linux()
		if err != nil {
			return false, err
		}
	}
	pubIP, err := GetPubIP()
	if err != nil {
		return false, err
	}
	if hostIP != pubIP {
		return true, nil
	}
	return false, nil
}

// CreateDirectory create a directory if one does not exist
func CreateDirectory(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create the directory %s: %v", path, err)
		}
	}
	return nil
}

func FileExists(f string) bool {
	if _, err := os.Stat(f); err != nil {
		return false
	}
	return true
}

// ReadKeyFileToString reads the key file and strips any newline chars that create wireguard issues
func ReadKeyFileToString(s string) (string, error) {
	buf, err := ioutil.ReadFile(s)
	if err != nil {
		return "", fmt.Errorf("unable to read file: %v\n", err)
	}
	rawStr := string(buf)
	return strings.Replace(rawStr, "\n", "", -1), nil
}
