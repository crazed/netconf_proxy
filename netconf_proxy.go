package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/crazed/ncclient-go"
	"io"
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
	success bool
	output  io.Reader
	client  *ncclient.Ncclient
}

func NetconfWorker(id int, request string, jobs <-chan *ncclient.Ncclient, results chan<- *NetconfResult) {
	for client := range jobs {
		result := new(NetconfResult)
		result.client = client
		if err := client.Connect(); err != nil {
			result.success = false
			result.output = bytes.NewBufferString(err.Error())
			results <- result
		} else {
			defer client.Close()
			client.SendHello()
			result.output = client.Write(request)
			result.success = true
			results <- result
		}
	}
}

func NetconfHandler(w http.ResponseWriter, r *http.Request) {
	t := new(NetconfRequest)
	json.NewDecoder(r.Body).Decode(t)
	// TODO: logging..
	fmt.Println(t)

	jobs := make(chan *ncclient.Ncclient, len(t.Hosts))
	results := make(chan *NetconfResult, len(t.Hosts))

	// Queue up a bunch of work
	for i, host := range t.Hosts {
		log.Println("Creating work for", host)
		go NetconfWorker(i, t.Request, jobs, results)
		client := ncclient.MakeClient(t.Username, t.Password, host, t.Port)
		jobs <- &client
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
		buf := new(bytes.Buffer)
		buf.ReadFrom(result.output)
		output := buf.String()

		// Create a response structure
		resp := struct {
			Hostname string
			Output   string
			Success  bool
		}{}
		resp.Hostname = result.client.Hostname()
		resp.Output = output
		resp.Success = result.success

		// Flush this line
		encoder.Encode(&resp)
	}
}

func main() {
	http.HandleFunc("/netconf", NetconfHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
