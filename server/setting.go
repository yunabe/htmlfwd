package main

import (
	"bufio"
	"io"
	"log"
	"os"
	"path"
	"strconv"
	"strings"
)

type Setting struct {
	browserPort          int32
	commandPort          int32
	keepAliveIntervalSec int32
	useSsl               bool
	serverCertificate    string
	serverPrivateKey     string
	authenticateBrowser bool
}

func (setting *Setting) verify() bool {
	ok := true
	if setting.authenticateBrowser {
		if !setting.useSsl {
			log.Println("use_ssl should be true to enable browser authentication.")
			ok = false
		}
	}
	if setting.useSsl {
		if len(setting.serverCertificate) == 0 {
			log.Println("server_certificate should be set to enable ssl.")
			ok = false
		}
		if len(setting.serverPrivateKey) == 0 {
			log.Println("server_private_key should be set to enable ssl")
			ok = false
		}
	}
	return ok;
}

func readSetting() *Setting {
	filename := path.Join(os.ExpandEnv("$HOME"), ".htmlfwdrc")
	file, err := os.Open(filename)
	setting := Setting{
		browserPort:          8888,
		commandPort:          9999,
		keepAliveIntervalSec: 60,
	}
	if err != nil {
		log.Println(err)
		return &setting
	}
	reader := bufio.NewReader(file)
	for {
		line, _, err := reader.ReadLine()
		if err != nil {
			if err != io.EOF {
				log.Println("Failed to read setting line:", err)
			}
			break
		}
		key_value := strings.Split(string(line), "=")
		if len(key_value) != 2 {
			log.Println("Invalid line format:", string(line))
			continue
		}
		key := key_value[0]
		value_str := key_value[1]
		value, err := strconv.ParseInt(value_str, 10, 32)
		if key == "browser_port" {
			setting.browserPort = int32(value)
		} else if key == "command_port" {
			setting.commandPort = int32(value)
		} else if key == "keep_alive_interval" {
			setting.keepAliveIntervalSec = int32(value)
		} else if key == "use_ssl" {
			setting.useSsl = true
		} else if key == "server_certificate" {
			setting.serverCertificate = value_str
		} else if key == "server_private_key" {
			setting.serverPrivateKey = value_str
		} else {
			log.Println("Unknown setting key:", key)
		} 
	}
	if setting.verify() {
		return &setting
	}
	return nil
}
