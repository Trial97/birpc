birpc
====

[![GoDoc](https://godoc.org/github.com/cgrates/birpc?status.png)](https://godoc.org/github.com/cgrates/birpc)
[![Build Status](https://travis-ci.org/cgrates/birpc.png)](https://travis-ci.org/cgrates/birpc)

birpc is a fork of net/rpc package in the standard library.
The main goal is to add bi-directional support to calls.
That means server can call the methods of client.
This is not possible with net/rpc package.
In order to do this it adds a `*Client` argument to method signatures.

Install
--------

    go get github.com/cgrates/birpc

Example server
---------------

```go
package main

import (
	"fmt"
	"net"

	"github.com/cgrates/birpc"
)

type Args struct{ A, B int }
type Reply int

func main() {
	srv := birpc.NewServer()
	srv.Handle("add", func(client *birpc.Client, args *Args, reply *Reply) error {

		// Reversed call (server to client)
		var rep Reply
		client.Call("mult", Args{2, 3}, &rep)
		fmt.Println("mult result:", rep)

		*reply = Reply(args.A + args.B)
		return nil
	})

	lis, _ := net.Listen("tcp", "127.0.0.1:5000")
	srv.Accept(lis)
}
```

Example Client
---------------

```go
package main

import (
	"fmt"
	"net"

	"github.com/cgrates/birpc"
)

type Args struct{ A, B int }
type Reply int

func main() {
	conn, _ := net.Dial("tcp", "127.0.0.1:5000")

	clt := birpc.NewClient(conn)
	clt.Handle("mult", func(client *birpc.Client, args *Args, reply *Reply) error {
		*reply = Reply(args.A * args.B)
		return nil
	})
	go clt.Run()

	var rep Reply
	clt.Call("add", Args{1, 2}, &rep)
	fmt.Println("add result:", rep)
}
```
