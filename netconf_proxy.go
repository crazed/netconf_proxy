package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/crazed/ncclient-go"
	"io"
	"log"
	"net/http"
	"text/template"
)

const VERSION = "0.0.4"

type Node struct {
	Facts    map[string]string
	Hostname string
}

type NetconfRequest struct {
	Hosts      []string
	Nodes      []Node
	Username   string
	Password   string
	Key        string
	Port       int
	Request    string
	APIVersion string
}

type NetconfResult struct {
	success bool
	output  io.Reader
	client  *ncclient.Ncclient
}

func NetconfTemplateWorker(template *template.Template, client *ncclient.Ncclient, node *Node) *NetconfResult {
	result := new(NetconfResult)
	result.client = client
	// Make sure we can connect
	if err := client.Connect(); err != nil {
		result.success = false
		result.output = bytes.NewBufferString(err.Error())
	} else {
		defer client.Close()
		client.SendHello()
		var request bytes.Buffer
		err := template.Execute(&request, node)
		// Make sure we can properly generate our RPC command using
		// the supplied template.
		if err != nil {
			result.output = bytes.NewBufferString(err.Error())
			result.success = false
		} else {
			// Finally, make sure our request gets written to the
			// client.
			if output, err := client.Write(request.String()); err != nil {
				result.output = bytes.NewBufferString(err.Error())
				result.success = false
			} else {
				result.output = output
				result.success = true
			}
		}
	}
	return result
}

func NetconfWorker(request string, client *ncclient.Ncclient) *NetconfResult {
	result := new(NetconfResult)
	result.client = client
	if err := client.Connect(); err != nil {
		result.success = false
		result.output = bytes.NewBufferString(err.Error())
	} else {
		defer client.Close()
		client.SendHello()
		if output, err := client.Write(request); err != nil {
			result.output = bytes.NewBufferString(err.Error())
			result.success = false
		} else {
			result.output = output
			result.success = true
		}
	}
	return result
}

func NetconfHandler(w http.ResponseWriter, r *http.Request) {
	t := new(NetconfRequest)
	t.APIVersion = "v1"

	json.NewDecoder(r.Body).Decode(t)
	log.Printf("Received a request to run '%s' on %d hosts", t.Request, len(t.Nodes))

	results := make(chan *NetconfResult, len(t.Hosts))

	// Queue up a bunch of work
	for _, host := range t.Hosts {
		client := ncclient.MakeClient(t.Username, t.Password, host, t.Key, t.Port)
		go func() {
			results <- NetconfWorker(t.Request, &client)
		}()
	}

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
		if err := encoder.Encode(&resp); err != nil {
			log.Println("encoding error:", err)
		}
	}
}

func NetconfV2Handler(w http.ResponseWriter, r *http.Request) {
	t := new(NetconfRequest)
	t.APIVersion = "v2"

	json.NewDecoder(r.Body).Decode(t)

	// If we did not get a Port, default to 22
	if t.Port == 0 {
		t.Port = 22
	}

	results := make(chan *NetconfResult, len(t.Nodes))

	template := template.Must(template.New("rpc-request").Parse(t.Request))

	// Launch a goroutine for every node we have
	for _, node := range t.Nodes {
		client := ncclient.MakeClient(t.Username, t.Password, node.Hostname, t.Key, t.Port)
		go func() {
			results <- NetconfTemplateWorker(template, &client, &node)
		}()
	}

	// Use http.Flusher if we can so clients can read results in real time
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	} else {
		log.Println("Could not flush!")
	}

	encoder := json.NewEncoder(w)
	for i := 0; i < len(t.Nodes); i++ {
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
		if err := encoder.Encode(&resp); err != nil {
			log.Println("encoding error:", err)
		}
	}
}

func main() {
	var useTls bool
	var tlsCertFile string
	var tlsKeyFile string
	var listen string
	var wantsVersion bool
	flag.BoolVar(&useTls, "secure", false, "Enable TLS server, requires cert and key flags")
	flag.StringVar(&tlsCertFile, "cert", "", "TLS certificate file path")
	flag.StringVar(&tlsKeyFile, "key", "", "TLS key file path")
	flag.StringVar(&listen, "listen", ":8080", "Listen string passed to ListenAndServe")
	flag.BoolVar(&wantsVersion, "version", false, "return version number and exit")
	flag.Parse()

	if wantsVersion {
		fmt.Println("netconf_proxy version:", VERSION)
		return
	}

	http.HandleFunc("/netconf", NetconfHandler)
	http.HandleFunc("/v2", NetconfV2Handler)
	if useTls {
		if tlsCertFile == "" || tlsKeyFile == "" {
			panic("Must set key file and cert file")
		}
		log.Printf("Listening on '%s' using TLS (key: %s cert: %s)", listen, tlsKeyFile, tlsCertFile)
		log.Fatal(http.ListenAndServeTLS(listen, tlsCertFile, tlsKeyFile, nil))
	} else {
		log.Printf("Listening on '%s', no TLS!", listen)
		log.Fatal(http.ListenAndServe(listen, nil))
	}
}
