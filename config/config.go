package config

import (
	"encoding/json"
	"log"
	"os"
)

type Configuration struct {
	LogFile     string `json:"logFile,omitempty"`
	LogPath     string `json:"logPath,omitempty"`
	MaxSize     int    `json:"maxSize,omitempty"`
	MaxBackups  int    `json:"maxBackups,omitempty"`
	MaxAge      int    `json:"maxAge,omitempty"`
	RollOver    string `json:"rollOver,omitempty"`
	RabbitMQUrl string `json:"rabbitMQUrl,omitempty"`
}

var Config Configuration

func (r *Configuration) Load(filename string) {
	//con := Configuration{}

	file, _ := os.Open(filename)

	decoder := json.NewDecoder(file)
	err := decoder.Decode(&Config)
	if err != nil {
		panic(err.Error())
	}

	Config.RabbitMQUrl = "amqp://guest:guest@localhost:5672/"

	if Config.LogPath == "" {
		Config.LogPath = "/logs"
	}

	log.Printf("%+v", Config)
}
