## netconf_proxy
This provides a simple way to execute [NETCONF](http://en.wikipedia.org/wiki/NETCONF) commands across an arbitrary amount of nodes. At the moment it is extremely bare bones, but functional.

**Installation**

If you have Go installed and configured, it's as simple as running:

         go install github.com/crazed/netconf_proxy

**Example Usage**

Once installed, start up the daemon like so:

        netconf_proxy

There is currently very little output, sorry, but your proxy is now up and running on port 8080. You can make a call with something like this:

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

**Python Client**

In an attempt to bring some metadata to NETCONF servers, there is a sample python client called [python-netcommander](http://github.com/crazed/python-netcommander). It should be simple to plug your own data source in to populate username, password, hosts, etc which can be piped into `netconf_proxy`.

