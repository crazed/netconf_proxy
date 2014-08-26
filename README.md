## netconf_proxy
This provides a simple way to execute [NETCONF](http://en.wikipedia.org/wiki/NETCONF) commands across an arbitrary amount of nodes. At the moment it is extremely bare bones, but functional.

**Installation**

If you have Go installed and configured, it's as simple as running:

         $ go get github.com/crazed/netconf_proxy
         $ go install github.com/crazed/netconf_proxy

**Example Usage**

Once installed, start up the daemon like so:

        $ netconf_proxy
        2014/01/07 12:39:54 Listening on ':8080', no TLS!


Your proxy is now up and running on port 8080, check the `--help` flag to enable TLS and change what port you're running on. You can make a call with something like this:

        curl localhost:8080/netconf -d'
          {"username":"admin",
           "password":"SUPER SECRET PASSWORD",
           "port":22,
           "request":"<rpc><get-chassis-inventory/></rpc>",
           "hosts":["10.10.1.1", "10.10.1.2"]}'

The response output will look something like this:

        {"Hostname":"10.10.1.1", "Success": true, "Output":"<rpc-reply>...</rpc-reply>"}
        {"Hostname":"10.10.1.2", "Success": true, "Output":"<rpc-reply>...</rpc-reply>"}

The HTTP connection will stream lines of JSON data after each NETCONF connection sends data back to the proxy server.

**V2 API with templating support**

Essentially this is a natural progression on the original mentality. This allows you to pass a list of nodes to run something against, and provide a list of facts that can be used to templatize your request. For example:

        curl localhost:8080/v2/netconf -d'
          {"username":"admin",
           "password":"SUPER SECRET PASSWORD",
           "port":22,
           "request": "<get-interface-information><interface-name>{{.Facts.upstream_port}}</interface-name></get-interface-information>",
           "nodes":[
             { "hostname": "10.10.1.1", "facts": { "upstream_port": "ae0" } },
             { "hostname": "10.10.1.2", "facts": { "upstream_port": "ae1" } }
           ]}'

In this case, we will change the rpc request per node based on the upstream_port fact.

**Python Client**

In an attempt to bring some metadata to NETCONF servers, there is a sample python client called [python-netcommander](http://github.com/crazed/python-netcommander). It should be simple to plug your own data source in to populate username, password, hosts, etc which can be piped into `netconf_proxy`.

