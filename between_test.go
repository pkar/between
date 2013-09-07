package between

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"testing"
)

var (
	conf   *Config
	client *http.Client
)
var config string = `
{
	"frontends": [
		{
			"name": "server1",
			"bind": "0.0.0.0:9010",
			"x-forwarded-for": true,
			"paths": ["localhost/google", "localhost", "127.0.0.1", "localhost/v1/ping"],
			"active": true
		},
		{
			"name": "server2 tls",
			"bind": "0.0.0.0:443",
			"https": true,
			"keyfile": "/path/to/key.pem",
			"certfile": "/path/to/cert.pem",
			"x-forwarded-for": true,
			"paths": ["localhost", "127.0.0.1"],
			"active": false
		}
	],
	"paths": {
		"localhost/search": ["google.com", "yahoo.com"],
		"localhost/v1/ping": ["localhost:9000"],
		"localhost": ["localhost:9010", "localhost:9009"],
		"loco.localhost": ["localhost:9011", "localhost:9012"]
	}
}
`

func init() {
	client = &http.Client{}

	err := json.Unmarshal([]byte(config), &conf)
	if err != nil {
		log.Fatal(err)
	}

	b := NewBetween(conf)
	b.Run()
}

func TestNewBetween(t *testing.T) {
	NewBetween(conf)
}

func TestBetweenRun(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:9010/google", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("test", "hello")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(body))
}
