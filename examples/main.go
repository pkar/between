package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/signal"

	"between"
)

func main() {
	confFile := flag.String("conf", "config.json", "json config file")
	flag.Parse()

	confBytes, err := ioutil.ReadFile(*confFile)
	if err != nil {
		log.Fatal(err)
	}
	var config between.Config
	err = json.Unmarshal([]byte(confBytes), &config)
	if err != nil {
		log.Fatal(err)
	}

	b := between.NewBetween(&config)
	b.Run()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	select {
	case sig := <-interrupt:
		log.Println(sig)
	}
}
