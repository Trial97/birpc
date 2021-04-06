package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	birpc "github.com/cgrates/birpc"
)

const (
	network = "tcp4"
	addr    = "127.0.0.1:5000"
)

func TestJSONRPC(t *testing.T) {
	type Args struct{ A, B int }
	type Reply int

	lis, err := net.Listen(network, addr)
	if err != nil {
		t.Fatal(err)
	}

	srv := birpc.NewServer()
	srv.Handle("add", func(ctx context.Context, args *Args, reply *Reply) error {
		*reply = Reply(args.A + args.B)

		var rep Reply
		client := birpc.ClientValueFromContext(ctx)
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
	srv.Handle("addPos", func(ctx context.Context, args []interface{}, result *float64) error {
		*result = args[0].(float64) + args[1].(float64)
		return nil
	})
	srv.Handle("rawArgs", func(ctx context.Context, args []json.RawMessage, reply *[]string) error {
		for _, p := range args {
			var str string
			json.Unmarshal(p, &str)
			*reply = append(*reply, str)
		}
		return nil
	})
	srv.Handle("typedArgs", func(ctx context.Context, args []int, reply *[]string) error {
		for _, p := range args {
			*reply = append(*reply, fmt.Sprintf("%d", p))
		}
		return nil
	})
	srv.Handle("nilArgs", func(ctx context.Context, args []interface{}, reply *[]string) error {
		for _, v := range args {
			if v == nil {
				*reply = append(*reply, "nil")
			}
		}
		return nil
	})
	number := make(chan int, 1)
	srv.Handle("set", func(ctx context.Context, i int, _ *struct{}) error {
		number <- i
		return nil
	})

	go func() {
		conn, err := lis.Accept()
		if err != nil {
			t.Error(err)
			return
		}
		srv.ServeCodec(NewJSONCodec(conn))
	}()

	conn, err := net.Dial(network, addr)
	if err != nil {
		t.Fatal(err)
	}

	clt := birpc.NewClientWithCodec(NewJSONCodec(conn))
	clt.Handle("mult", func(ctx context.Context, args *Args, reply *Reply) error {
		*reply = Reply(args.A * args.B)
		return nil
	})
	go clt.Run()

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

	// Test undefined method.
	err = clt.Call(context.TODO(), "foo", 1, &rep)
	if err.Error() != "birpc: can't find method foo" {
		t.Fatal(err)
	}

	// Test Positional arguments.
	var result float64
	err = clt.Call(context.TODO(), "addPos", []interface{}{1, 2}, &result)
	if err != nil {
		t.Fatal(err)
	}
	if result != 3 {
		t.Fatalf("not expected: %f", result)
	}

	testArgs := func(expected, reply []string) error {
		if len(reply) != len(expected) {
			return fmt.Errorf("incorrect reply length: %d", len(reply))
		}
		for i := range expected {
			if reply[i] != expected[i] {
				return fmt.Errorf("not expected reply[%d]: %s", i, reply[i])
			}
		}
		return nil
	}

	// Test raw arguments (partial unmarshal)
	var reply []string
	var expected []string = []string{"arg1", "arg2"}
	rawArgs := json.RawMessage(`["arg1", "arg2"]`)
	err = clt.Call(context.TODO(), "rawArgs", rawArgs, &reply)
	if err != nil {
		t.Fatal(err)
	}

	if err = testArgs(expected, reply); err != nil {
		t.Fatal(err)
	}

	// Test typed arguments
	reply = []string{}
	expected = []string{"1", "2"}
	typedArgs := []int{1, 2}
	err = clt.Call(context.TODO(), "typedArgs", typedArgs, &reply)
	if err != nil {
		t.Fatal(err)
	}
	if err = testArgs(expected, reply); err != nil {
		t.Fatal(err)
	}

	// Test nil args
	reply = []string{}
	expected = []string{"nil"}
	err = clt.Call(context.TODO(), "nilArgs", nil, &reply)
	if err != nil {
		t.Fatal(err)
	}
	if err = testArgs(expected, reply); err != nil {
		t.Fatal(err)
	}
}
