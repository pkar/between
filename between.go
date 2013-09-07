package between

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// Frontened [...]
type Frontend struct {
	Name          string   `json:"name"`
	Bind          string   `json:"bind"`
	Paths         []string `json:"paths"` // Order matters
	XForwardedFor bool     `json:"x-forwarded-for"`
	Https         bool     `json:"https"`
	Keyfile       string   `json:"keyfile"`
	Certfile      string   `json:"certfile"`
	Active        bool     `json:"active"`
}

// Config is the structure for defining the load balancer
// and reverse proxy.
type Config struct {
	Frontends []*Frontend `json:"frontends"`
	// Paths is an ordered list of Paths. Order matters
	// as first found is used to route to backends.
	Paths map[string][]string `json:"paths"`
}

// RequestHandler adds frontend and backend configurations.
type RequestHandler struct {
	Transport    *http.Transport
	Frontend     *Frontend
	PathBackends []interface{}
}

// CopyBidir [...]
func CopyBidir(conn1 io.ReadWriteCloser, rw1 *bufio.ReadWriter, conn2 io.ReadWriteCloser, rw2 *bufio.ReadWriter) {
	finished := make(chan bool)

	go func() {
		io.Copy(rw2, rw1)
		conn2.Close()
		finished <- true
	}()
	go func() {
		io.Copy(rw1, rw2)
		conn1.Close()
		finished <- true
	}()

	<-finished
	<-finished
}

// Backend retries from a matching path and pool a backend
// connection string
func (rh *RequestHandler) Backend(reqPath string) string {
	var pathChan chan string
	backend := ""
	for _, path := range rh.PathBackends {
		p, ok := path.([]interface{})[0].(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(reqPath, p) {
			c, ok := path.([]interface{})[1].(chan string)
			if !ok {
				continue
			}
			pathChan = c
			backend = <-pathChan
			pathChan <- backend
			break
		}
	}

	return backend
}

// ServeHTTP [...]
func (rh *RequestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.RequestURI = ""
	r.URL.Scheme = "http"

	if rh.Frontend.XForwardedFor {
		remoteAddr, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			r.Header.Add("X-Forwarded-For", remoteAddr)
		}
	}

	reqPath, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	backend := rh.Backend(reqPath + r.URL.Path)
	if backend == "" {
		http.NotFound(w, r)
		return
	}
	r.URL.Host = backend

	// Check for websocket connection
	isWebsocket := false
	if len(r.Header["Connection"]) > 0 && len(r.Header["Upgrade"]) > 0 {
		if strings.ToLower(r.Header["Upgrade"][0]) == "websocket" {
			isWebsocket = true
		}
	}

	switch isWebsocket {
	case false:
		resp, err := rh.Transport.RoundTrip(r)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Error: %v", err)
			return
		}
		if resp.Body != nil {
			defer resp.Body.Close()
		}

		for k, v := range resp.Header {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}

		w.WriteHeader(resp.StatusCode)

		io.Copy(w, resp.Body)
		return
	case true:
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}
		conn, bufrw, err := hj.Hijack()
		defer conn.Close()
		conn2, err := net.Dial("tcp", r.URL.Host)
		if err != nil {
			http.Error(w, "couldn't connect to backend server", http.StatusServiceUnavailable)
			return
		}
		defer conn2.Close()

		err = r.Write(conn2)
		if err != nil {
			log.Printf("writing WebSocket request to backend server failed: %v", err)
			return
		}
		CopyBidir(conn, bufrw, conn2, bufio.NewReadWriter(bufio.NewReader(conn2), bufio.NewWriter(conn2)))
		return
	}
}

// Run reads a frontends configuration, creates a
// request handler and starts up a server which
// is bound to Frontend.Bind.
func (f *Frontend) Run(paths []map[string][]string) {
	mux := http.NewServeMux()
	pathBackends := []interface{}{}

	for _, mapping := range paths {
		for path, backends := range mapping {
			ch := make(chan string, len(backends))
			for _, backend := range backends {
				ch <- backend
			}
			backend := []interface{}{path, ch}
			pathBackends = append(pathBackends, backend)
		}
	}

	requestHandler := &RequestHandler{
		Transport: &http.Transport{
			DisableKeepAlives:  false,
			DisableCompression: false,
		},
		Frontend:     f,
		PathBackends: pathBackends,
	}

	mux.Handle("/", requestHandler)
	srv := &http.Server{Handler: mux, Addr: f.Bind}
	switch f.Https {
	case true:
		err := srv.ListenAndServeTLS(f.Certfile, f.Keyfile)
		if err != nil {
			log.Printf("Starting HTTPS frontend %s failed: %v", f.Name, err)
		}
	case false:
		err := srv.ListenAndServe()
		if err != nil {
			log.Printf("Starting HTTP frontend %s failed: %v", f.Name, err)
		}
	}
}

// Between [...]
type Between struct {
	Config *Config
}

// Run [...]
func (b *Between) Run() {
	for _, frontend := range b.Config.Frontends {
		if !frontend.Active {
			continue
		}
		paths := []map[string][]string{}
		for _, h := range frontend.Paths {
			for path, backend := range b.Config.Paths {
				if path == h {
					m := map[string][]string{path: backend}
					paths = append(paths, m)
					break
				}
			}
		}

		go frontend.Run(paths)
	}
}

// NewBetween [...]
func NewBetween(c *Config) *Between {
	bet := &Between{Config: c}
	return bet
}
