The connection system is responsible for delivering requests from one component to another. 

## Request

The `Req` class is a general abstraction for the messages sent from one component to another. A request has common fields including ID (an [UUID](https://en.wikipedia.org/wiki/Universally_unique_identifier) string), send time, receive time, source, and destination. Each concrete request type can contain different detailed information.

## Port

Ports are the connection points owned by components. For example, a cache module usually has a `ToTop` port and a `ToBottom` port, connecting the modules on the top of the cache or at the bottom of the cache, respectively. It is a convention to name the port with uppercase camelcase names, starting with the word "To". However, all the ports are bidirectional. In addition, an Akita port does not map to the hardware concept port. Usually in hardware concepts, having two ports means the component can handle two requests concurrently. In Akita, the degree of concurrency is defined in the component logic, rather than the port. Finally, a port has an incoming buffer that stores the packets sent from the connection to the component. The connection is responsible for maintaining the out-going buffer for packets sent from the component to the connection.

Ports define a `Send` function to be called by the component to send a request. The `Send` function may return an error. Ignoring the error is dangerous as it may cause unexpected request drops.

Ports also define a `Recv` function for the connection. When connection delivers a request, the `Recv` function of the destination port is called. The destination port should put the request into its buffer and notifies the component with the component's `NotifyRecv` function. The component will, later on, call the `Peek` function or the `Retrieve` function of the port to read the incoming request. 

## Connection

Connection defines how ports are connected. All the connections are best-effort connections and they guarantee that the packets will arrive at the destination port at some time in the future. No packet will drop nor negatively acknowledged. 

Connections define a `PlugIn` function to associate a port with a connection. Once the `PlugIn` function is called, the connection should provide a request delivery service for the port. 

In the Akita repo, only a `DirectConnection` is provided. More complex connections that model the timing detail on the connection are provided in the [NOC](https://gitlab.com/akita/noc) repo. A `DirectConnection` is not a simple emulator connection, nor will introduce inaccuracy. In real hardware where two components are connected with wires, the two components transfer messages with a predefined format over the wires, and therefore, can transfer one message per cycle. A `DirectConnection` can accurately model the wire-based connection. 


 

