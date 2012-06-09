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
		value, err := strconv.ParseInt(key_value[1], 10, 32)
		if err != nil {
			log.Println("Invalid setting value:", key_value[1])
			continue
		}
		if key == "browser_port" {
			setting.browserPort = int32(value)
		} else if key == "command_port" {
			setting.commandPort = int32(value)
		} else if key == "keep_alive_interval" {
			setting.keepAliveIntervalSec = int32(value)
		} else {
			log.Println("Unknown setting key:", key)
		}
	}
	return &setting
}
