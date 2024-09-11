// Implementation of a KeyValueServer. Students should write their code in this file.

package kvserver

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/puru/server/kvstore"
)

type keyValueServer struct {
	db         *kvstore.KVStore        // the KV database
	numActive  int                     // Active clients
	getActive  chan int                // Comm channel to get numActive
	numDropped int                     // Dropped clients
	getDropped chan int                // Comm channel to get numDropped
	queryChan  chan query              // Comm channel for db queries
	ln         net.Listener            // Listener
	closeChan  chan int                // Comm channel to handle Close
	connMap    map[*net.TCPConn]client // Map of active client conns
}

// New creates and returns (but does not start) a new KeyValueServer.
func New(store kvstore.KVStore) KeyValueServer {

	kvs := &keyValueServer{
		db:         &store,
		numActive:  0,
		getActive:  make(chan int),
		numDropped: 0,
		getDropped: make(chan int),
		queryChan:  make(chan query, 1),
		ln:         nil,
		closeChan:  make(chan int, 1),
		connMap:    make(map[*net.TCPConn]client),
	}
	return kvs
}

// Start starts the server on a distinct port.
func (kvs *keyValueServer) Start(port int) error {

	// Create a TCP server
	port_str := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", port_str)
	if err != nil {
		return err
	}

	// manage -> goroutine to manage clients (Readers, Writers, server/db queries, closing conns etc.)
	// accept -> gorouting to accept clients
	kvs.ln = ln
	go kvs.manage()
	go kvs.accept()
	return nil
}

// Close shuts down the server.
func (kvs *keyValueServer) Close() {
	// Signal manage and accept to return
	close(kvs.closeChan)
	kvs.ln.Close()
}

// CountActive returns the number of clients currently connected to the server.
func (kvs *keyValueServer) CountActive() int {
	// Send a query to get current count
	kvs.queryChan <- query{qType: "Active"}
	return <-kvs.getActive
}

// CountDropped returns the number of clients dropped by the server.
func (kvs *keyValueServer) CountDropped() int {
	// Send a query to get current count
	kvs.queryChan <- query{qType: "Dropped"}
	cnt := <-kvs.getDropped
	return cnt
}

// Additional methods/functions

// Max size of a client response buffer
const bufferSize = 500

// Struct for keyValueServer queries
// can be external (client requests)
// can be internal (add/close conn, get active/dropped)
type query struct {
	client client   // The client the query came from
	qType  string   // query type            : Union{Put, Get, Delete, Update, Active, Dropped, RemConn, AddConn}
	key    string   // db key                : Union{string, nil}
	suffix []string // parsed request suffix : Union{[value], [oldValue, newValue], nil}
}

// Parse client requests into queries
func parseRequest(c client, req string) query {
	split := strings.Split(strings.TrimSuffix(req, "\n"), ":")
	q := query{
		client: c,
		qType:  split[0],
		key:    split[1],
		suffix: split[2:],
	}
	return q
}

// Struct representing clients
type client struct {
	conn        net.Conn    // client connection
	closeWriter chan int    // Comm channel to close client writer
	closeReader chan int    // Comm channel to close client reader
	respChan    chan string // Comm channel to send respose messages
}

// Creates a new client
func newClient(conn net.Conn) client {
	c := client{
		conn:        conn,
		closeWriter: make(chan int, 1),
		closeReader: make(chan int, 1),
		respChan:    make(chan string, 500),
	}
	return c
}

// Routine to accept a new client
// Init's a new client
// Send -> clientChan to be manage'd
func (kvs *keyValueServer) accept() {
	for {
		select {
		case <-kvs.closeChan: // Close signal
			return
		default:
			conn, err := kvs.ln.Accept()
			if err == nil {
				// create a new client
				// send a query to manage to add conn to internal map
				c := newClient(conn)
				kvs.queryChan <- query{qType: "AddConn", client: c}
			}
		}
	}
}

// Routine to manage the keyValueServer
// Resposible for handling queries
// Responsible for recource cleanup
func (kvs *keyValueServer) manage() {
	for {
		select {
		case <-kvs.closeChan: // Close signal

			// Immediately close all conn's
			// Remove conn's from map
			for k, v := range kvs.connMap {
				v.conn.Close()
				v.closeReader <- 1
				v.closeWriter <- 1
				delete(kvs.connMap, k)
			}
			return

		case query := <-kvs.queryChan: // query management

			switch query.qType {

			// handle CountActive queries
			case "Active":
				kvs.getActive <- len(kvs.connMap)

			// handle CountActive queries
			case "Dropped":
				kvs.getDropped <- kvs.numDropped

			// handle accept queries
			case "AddConn":
				// Add a new client conn to internal map
				// Spawn per-client read routine
				// Spawn per-client write routine
				tcpConn, ok := query.client.conn.(*net.TCPConn)
				if ok {
					kvs.connMap[tcpConn] = query.client
					go reader(kvs, query.client)
					go writer(query.client)
				}

			// handle client Put requests
			case "Put":
				(*kvs.db).Put(query.key, []byte(query.suffix[0]))

			// handle client Get requests
			case "Get":
				value := (*kvs.db).Get(query.key)
				// if response buffer full, drop
				// else send response
				if len(query.client.respChan) >= bufferSize {
					break
				} else {
					for _, v := range value {
						r := fmt.Sprintf("%s:%s\n", query.key, v)
						query.client.respChan <- r
					}
				}

			// handle client Delete requests
			case "Delete":
				(*kvs.db).Delete(query.key)

			// handle client Update requests
			case "Update":
				(*kvs.db).Update(query.key, []byte(query.suffix[0]), []byte(query.suffix[1]))

			// handle reader query upon io.EOF
			case "RemConn":
				// remove conn from map and close
				// signal writer to return
				// update numDropped
				tcpConn, ok := query.client.conn.(*net.TCPConn)
				if ok {
					delete(kvs.connMap, tcpConn)
					tcpConn.Close()
				}
				query.client.closeWriter <- 1
				kvs.numDropped++
			}
		}
	}

}

// Routine to read from a conn
// Spawned by manage upon recieving an AddConn query from accept
func reader(kvs *keyValueServer, c client) {
	rdr := bufio.NewReader(c.conn)
	for {
		select {
		// handle termination due to server Close
		case <-c.closeReader:
			return
		default:
			req, err := rdr.ReadString('\n')
			// return on err
			// additionally send a RemConn query to manage for resource cleanup if EOF is reached
			if err != nil {
				if err == io.EOF {
					kvs.queryChan <- query{client: c, qType: "RemConn"}
				}
				return
			}
			// parse client request into query
			// send query to manage
			q := parseRequest(c, req)
			kvs.queryChan <- q
		}
	}
}

// Routine to write to a conn
// Spawned by manage upon recieving an AddConn query from accept
func writer(c client) {
	for {
		select {
		// handle termination due to server Close
		case <-c.closeWriter:
			return

		// Write response to client if message availablw
		case resp := <-c.respChan:
			c.conn.Write([]byte(resp))
		}
	}
}
