# KeyValueServer

## Setting up Go

You can download and install Go for your operating system from [the official site](https://golang.org/doc/install).
**Tested with Go version 1.23.**

## A key-value messaging system

This repository contains the code for a key-value messaging system implementation. It also contains the tests to test my implementation and an example 'server runner' binary useful for testing purposes.

### Specifications

- The server manages and interacts with its clients concurrently using Goroutines and channels. Multiple clients should be able to connect/disconnect to the server simultaneously.

- When a client wants to put a value into the server, it sends the following request string: `Put:key:value`. When a client wants to get a value from the server, it sends the following request string: `Get:key`. When a client wants to delete a key from the server, it sends the following request string: `Delete:key`. When a client wants to update an oldValue with a newValue for a given key, it sends the following request string `Update:key:oldValue:newValue`. These are the only possible messages acceptable from the client.

- When the server reads a `Get()` request message from a client, it responds with the following string: `key:value[newline]` to the client that sent the message. It sends this string once for every value associated with the given key. No response is sent for a `Put()` request, a `Delete()` request, or an `Update()` request.

- Every message that is sent or received should be terminated by the newline (\n) character.

- The server implements a CountActive() function that returns the number of clients that are currently connected to it.

- The server implements a CountDropped() function that returns the number of clients that have been disconnected from and thus dropped from the server.

- The server is responsive to slow-reading clients. To better understand what this means, consider a scenario in which a client does not call Read for an extended period of time. If during this time the server continues to write messages to the client’s TCP connection, eventually the TCP connections output buffer will reach maximum capacity and subsequent calls to Write made by the server will block. To handle these cases, the server keeps a queue of at most 500 outgoing messages to be written to the client at a later time. Messages sent to a slow-reading client whose outgoing message buffer has reached the maximum capacity of 500 are simply dropped. If the slow-reading client starts reading again in the future, the server ensures that any buffered messages in its queue are written back to the client. A buffered channel is used to implement this properly.

The below `go test` commands should work out-of-the-box.

### Running the official tests

To test execute the following command from inside the `src/github.com/puru/server` directory:

```sh
$ go test
```

You can also check the code for race conditions using Go's race detector by executing
the following command:

```sh
$ go test -race
```

To execute a single unit test, you can use the `-test.run` flag and specify a regular expression
identifying the name of the test to run. For example,

```sh
$ go test -race -test.run TestBasic1
```

### Using `srunner`

The repository also contains a a simple `srunner` (server runner)
program that you can use to create and start an instance of the `KeyValueServer`. The program
simply creates an instance of the server, starts it on a default port, and blocks forever,
running the server in the background.

To compile and build the `srunner` program into a binary that you can run, execute the
command below from within the `src/github.com/cmu440/` directory:

```bash
$ go install github.com/cmu440/srunner
```

Then you can run the `srunner` binary from anywhere by executing:

```bash
$ $HOME/go/bin/srunner
```

(`$HOME/go/bin` is Go's default destination for `go install`'d binaries. If the command fails or you want to change the install directory, see the options [here](https://go.dev/doc/code#Command).)

You can test the server using Netcat (i.e. run the `srunner` binary in the background, execute `nc localhost 9999`, type the message you wish to send, and then
hit enter). You can get more information on how to use netcat using the man pages (`man nc`).
