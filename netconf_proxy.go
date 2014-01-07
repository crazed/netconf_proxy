package main

import (
	"encoding/json"
	"fmt"
	"github.com/Juniper/go-netconf/netconf"
	"log"
	"net/http"
)

type NetconfRequest struct {
	Hosts    []string
	Username string
	Password string
	Port     int
	Request  string
}

type NetconfResult struct {
	success  bool
	output   string
	hostname string
}

// This is necessary since netconf.Session has no way to retrieve
// the hostname/ip that is associated with the session.
type NetconfWrapper struct {
	Hostname string
	Client   *netconf.Session
}

func NetconfWorker(id int, request string, jobs <-chan *NetconfWrapper, results chan<- *NetconfResult) {
	for w := range jobs {
		result := new(NetconfResult)
		result.hostname = w.Hostname
		if reply, err := w.Client.Exec(request); err != nil {
			fmt.Println(reply)
			result.success = false
			result.output = reply.RawReply
			results <- result
		} else {
			fmt.Println(reply)
			result.success = true
			result.output = reply.RawReply
			results <- result
		}
	}
}

func NetconfHandler(w http.ResponseWriter, r *http.Request) {
	t := new(NetconfRequest)
	json.NewDecoder(r.Body).Decode(t)
	// TODO: logging..
	fmt.Println(t)

	jobs := make(chan *NetconfWrapper, len(t.Hosts))
	results := make(chan *NetconfResult, len(t.Hosts))

	// Queue up a bunch of work
	for i, host := range t.Hosts {
		log.Println("Creating work for", host)
		go NetconfWorker(i, t.Request, jobs, results)
		target := fmt.Sprintf("%s:%d", host, t.Port)
		client, err := netconf.DialSSH(target, netconf.SSHConfigPassword(t.Username, t.Password))
		fmt.Println(client)
		if err != nil {
			fmt.Println("Failed to connect to:", host)
		}
		jobs <- &NetconfWrapper{Client: client, Hostname: host}
	}
	close(jobs)

	// Use http.Flusher if we can so clients can read results in real time
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	} else {
		log.Println("Could not flush!")
	}

	encoder := json.NewEncoder(w)
	for i := 0; i < len(t.Hosts); i++ {
		result := <-results

		// Pull our entire response output into a string
		//buf := new(bytes.Buffer)
		//buf.ReadFrom(result.output)
		//output := buf.String()

		// Create a response structure
		resp := struct {
			Hostname string
			Output   string
			Success  bool
		}{}
		resp.Hostname = result.hostname
		resp.Output = result.output
		resp.Success = result.success

		// Flush this line
		encoder.Encode(&resp)
	}
}

func main() {
	http.HandleFunc("/netconf", NetconfHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
