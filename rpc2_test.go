package birpc

import (
	"context"
	"net"
	"testing"
	"time"
)

const (
	network = "tcp4"
	addr    = "127.0.0.1:5000"
)

func TestTCPGOB(t *testing.T) {
	type Args struct{ A, B int }
	type Reply int

	lis, err := net.Listen(network, addr)
	if err != nil {
		t.Fatal(err)
	}

	srv := NewServer()
	srv.Handle("add", func(ctx context.Context, args *Args, reply *Reply) error {
		*reply = Reply(args.A + args.B)

		var rep Reply
		client := ClientValueFromContext(ctx)
		if client == nil {
			t.Fatal("expected client not nil")
		}
		err := client.Call(context.TODO(), "mult", Args{2, 3}, &rep)
		if err != nil {
			t.Fatal(err)
		}

		if rep != 6 {
			t.Fatalf("not expected: %d", rep)
		}

		return nil
	})
	number := make(chan int, 1)
	srv.Handle("set", func(client context.Context, i int, _ *struct{}) error {
		number <- i
		return nil
	})
	go srv.Accept(lis)

	conn, err := net.Dial(network, addr)
	if err != nil {
		t.Fatal(err)
	}

	clt := NewClient(conn)
	clt.Handle("mult", func(client context.Context, args *Args, reply *Reply) error {
		*reply = Reply(args.A * args.B)
		return nil
	})
	go clt.Run()
	defer clt.Close()

	// Test Call.
	var rep Reply
	err = clt.Call(context.TODO(), "add", Args{1, 2}, &rep)
	if err != nil {
		t.Fatal(err)
	}
	if rep != 3 {
		t.Fatalf("not expected: %d", rep)
	}

	// Test notification.
	err = clt.Notify("set", 6)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case i := <-number:
		if i != 6 {
			t.Fatalf("unexpected number: %d", i)
		}
	case <-time.After(time.Second):
		t.Fatal("did not get notification")
	}

	// Test blocked request
	clt.SetBlocking(true)
	err = clt.Call(context.TODO(), "add", Args{1, 2}, &rep)
	if err != nil {
		t.Fatal(err)
	}
	if rep != 3 {
		t.Fatalf("not expected: %d", rep)
	}

	// Test undefined method.
	err = clt.Call(context.TODO(), "foo", 1, &rep)
	if err.Error() != "birpc: can't find method foo" {
		t.Fatal(err)
	}
}
