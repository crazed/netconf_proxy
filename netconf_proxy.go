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

// This function is used to take a raw RPC request in string form, and an Ncclient.
// With this, it will attempt to connect to our netconf client, then initiate the
// NETCONF protocol and write our request. The return value is a NetconfResult, which
// is used by retrieveResults to flush our data to the HTTP caller.
func performWork(request string, client *ncclient.Ncclient) (result *NetconfResult) {
	// Make sure we can connect
	if err := client.Connect(); err != nil {
		result.output = bytes.NewBufferString(err.Error())
		result.success = false
		// If we're good..
	} else {
		// Ensure we always close our client connections! Then start the NETCONF protocol.
		defer client.Close()
		client.SendHello()
		// Make sure our request gets written to the
		// client.
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

// Read through a channel, and write the results out to a json Encoder.
// This blocks when called while waiting to read from our results channel.
// It is important that resultCount be equal to the number of items that will be
// dropped into our results channel.
func retrieveResults(results chan *NetconfResult, resultCount int, encoder *json.Encoder) {
	for i := 0; i < resultCount; i++ {
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

// Create a new NetconfRequest and initialize the default values
func newNetconfRequest(body io.Reader) *NetconfRequest {
	// Decode our JSON body into a NetconfRequest struct
	request := new(NetconfRequest)
	json.NewDecoder(body).Decode(request)
	// Make sure we have a valid SSH port to deal with
	if request.Port == 0 {
		request.Port = 22
	}
	return request
}

// When an HTTP request contains a template, use this worker function to process our template
// before calling performWork.
func NetconfTemplateWorker(template *template.Template, client *ncclient.Ncclient, node *Node) (result *NetconfResult) {
	// When using a template.Template, the Execute method expects
	// an io.Writer interface. Here's a quick way to satisfy that
	// requirement, by using a buffer.
	var requestBuffer bytes.Buffer
	// Make sure we can properly generate our RPC command using
	// the supplied template. Store the results in our buffer.
	err := template.Execute(&requestBuffer, node)
	if err != nil {
		// If we do have an error, short circuit here
		result := new(NetconfResult)
		result.success = false
		result.output = bytes.NewBufferString("Template error: " + err.Error())
		return result
	}
	// Convert our buffer into a string, which is what our ncclient.Client
	// expects as input when calling the Write method.
	request := requestBuffer.String()
	// Continue on with our request
	return performWork(request, client)
}

// Default worker function, which was used uring proof of concept phases of this proxy.
// Keep it around as there are some pieces of code written to use this directly.
// TODO: deprecate this function
func NetconfWorker(request string, client *ncclient.Ncclient) *NetconfResult {
	// Internally call performWork, which lets us deprecate this
	// function over time.
	return performWork(request, client)
}

// HTTP request handler for the "v1" of our API, which is not even name spaced,
// but was used during the proof of concept.
func NetconfHandler(w http.ResponseWriter, r *http.Request) {
	n := newNetconfRequest(r.Body)
	n.APIVersion = "v1"
	log.Printf("Received a request to run '%s' on %d hosts", n.Request, len(n.Hosts))
	// Create a channel to allow communication between our go routines
	// and our main process.
	results := make(chan *NetconfResult, len(n.Hosts))
	// Create one go routine for every Host we are handling, essentially
	// this creates a new NETCONF over SSH connection for every host requested.
	for _, host := range n.Hosts {
		client := ncclient.MakeClient(n.Username, n.Password, host, n.Key, n.Port)
		go func() {
			results <- NetconfWorker(n.Request, &client)
		}()
	}

	// Use http.Flusher if we can so clients can read results in real time
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	} else {
		log.Println("Could not flush!")
	}

	// Block while read in results, and write them out
	// to our client.
	retrieveResults(results, len(n.Hosts), json.NewEncoder(w))
}

// Our V2 handler, there's a bit of boiler plate here shared with the original handler.
// It would be good to eventually simplify this stuff a bit more.
func NetconfV2Handler(w http.ResponseWriter, r *http.Request) {
	n := newNetconfRequest(r.Body)
	n.APIVersion = "v2"
	log.Printf("Received a request to run '%s' on %d hosts", n.Request, len(n.Nodes))
	results := make(chan *NetconfResult, len(n.Nodes))
	template := template.Must(template.New("rpc-request").Parse(n.Request))

	// Launch a goroutine for every node we have
	for _, node := range n.Nodes {
		client := ncclient.MakeClient(n.Username, n.Password, node.Hostname, n.Key, n.Port)
		go func() {
			results <- NetconfTemplateWorker(template, &client, &node)
		}()
	}

	// Block while read in results, and write them out
	// to our client.
	retrieveResults(results, len(n.Nodes), json.NewEncoder(w))
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

	// Mount our two netconf handlers appropriately.
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
