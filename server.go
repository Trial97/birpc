package birpc

import (
	"context"
	"io"
	"log"
	"net"
	"reflect"
	"unicode"
	"unicode/utf8"

	"github.com/cenkalti/hub"
)

// Precompute the reflect type for error.  Can't use error directly
// because Typeof takes an empty interface value.  This is annoying.
var typeOfError = reflect.TypeOf((*error)(nil)).Elem()
var typeOfCtx = reflect.TypeOf((*context.Context)(nil)).Elem()

// var typeOfClient = reflect.TypeOf((*Client)(nil))

// unique type to prevent assignment.
type clientContextKey struct{}

// WithClient returns a new context based on the provided parent ctx.
func WithClient(ctx context.Context, c *Client) context.Context {
	return context.WithValue(ctx, clientContextKey{}, c)
}

// ClientValueFromContext retuns the client from context
func ClientValueFromContext(ctx context.Context) *Client {
	c, _ := ctx.Value(clientContextKey{}).(*Client)
	return c
}

const (
	clientConnected hub.Kind = iota
	clientDisconnected
)

// Server responds to RPC requests made by Client.
type Server struct {
	handlers map[string]*handler
	eventHub *hub.Hub
}

type handler struct {
	fn        reflect.Value
	argType   reflect.Type
	replyType reflect.Type
}

type connectionEvent struct {
	Client *Client
}

type disconnectionEvent struct {
	Client *Client
}

func (connectionEvent) Kind() hub.Kind    { return clientConnected }
func (disconnectionEvent) Kind() hub.Kind { return clientDisconnected }

// NewServer returns a new Server.
func NewServer() *Server {
	return &Server{
		handlers: make(map[string]*handler),
		eventHub: &hub.Hub{},
	}
}

// Handle registers the handler function for the given method. If a handler already exists for method, Handle panics.
func (s *Server) Handle(method string, handlerFunc interface{}) {
	addHandler(s.handlers, method, handlerFunc)
}

func addHandler(handlers map[string]*handler, mname string, handlerFunc interface{}) {
	if _, ok := handlers[mname]; ok {
		panic("birpc: multiple registrations for " + mname)
	}

	method := reflect.ValueOf(handlerFunc)
	mtype := method.Type()
	// Method needs three ins: *client, *args, *reply.
	if mtype.NumIn() != 3 {
		log.Panicln("method", mname, "has wrong number of ins:", mtype.NumIn())
	}
	// First arg must be a pointer to birpc.Client.
	if ctxType := mtype.In(0); ctxType != typeOfCtx {
		log.Panicln("method", mname, "first argument", ctxType.String(), "not context.Context")
	}
	// Second arg need not be a pointer.
	argType := mtype.In(1)
	if !isExportedOrBuiltinType(argType) {
		log.Panicln(mname, "argument type not exported:", argType)
	}
	// Third arg must be a pointer.
	replyType := mtype.In(2)
	if replyType.Kind() != reflect.Ptr {
		log.Panicln("method", mname, "reply type not a pointer:", replyType)
	}
	// Reply type must be exported.
	if !isExportedOrBuiltinType(replyType) {
		log.Panicln("method", mname, "reply type not exported:", replyType)
	}
	// Method needs one out.
	if mtype.NumOut() != 1 {
		log.Panicln("method", mname, "has wrong number of outs:", mtype.NumOut())
	}
	// The return type of the method must be error.
	if returnType := mtype.Out(0); returnType != typeOfError {
		log.Panicln("method", mname, "returns", returnType.String(), "not error")
	}
	handlers[mname] = &handler{
		fn:        method,
		argType:   argType,
		replyType: replyType,
	}
}

// Is this type exported or a builtin?
func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// PkgPath will be non-empty even for an exported type,
	// so we need to check the type name as well.
	return isExported(t.Name()) || t.PkgPath() == ""
}

// Is this an exported - upper case - name?
func isExported(name string) bool {
	rune, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(rune)
}

// OnConnect registers a function to run when a client connects.
func (s *Server) OnConnect(f func(*Client)) {
	s.eventHub.Subscribe(clientConnected, func(e hub.Event) {
		go f(e.(connectionEvent).Client)
	})
}

// OnDisconnect registers a function to run when a client disconnects.
func (s *Server) OnDisconnect(f func(*Client)) {
	s.eventHub.Subscribe(clientDisconnected, func(e hub.Event) {
		go f(e.(disconnectionEvent).Client)
	})
}

// Accept accepts connections on the listener and serves requests
// for each incoming connection.  Accept blocks; the caller typically
// invokes it in a go statement.
func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Print("rpc.Serve: accept:", err.Error())
			return
		}
		go s.ServeConn(conn)
	}
}

// ServeConn runs the server on a single connection.
// ServeConn blocks, serving the connection until the client hangs up.
// The caller typically invokes ServeConn in a go statement.
// ServeConn uses the gob wire format (see package gob) on the
// connection.  To use an alternate codec, use ServeCodec.
func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	s.ServeCodec(NewGobCodec(conn))
}

// ServeCodec is like ServeConn but uses the specified codec to
// decode requests and encode responses.
func (s *Server) ServeCodec(codec Codec) {
	s.ServeCodecWithState(codec, NewState())
}

// ServeCodecWithState is like ServeCodec but also gives the ability to
// associate a state variable with the client that persists across RPC calls.
func (s *Server) ServeCodecWithState(codec Codec, state *State) {
	defer codec.Close()

	// Client also handles the incoming connections.
	c := NewClientWithCodec(codec)
	c.server = true
	c.handlers = s.handlers
	c.State = state

	s.eventHub.Publish(connectionEvent{c})
	c.Run()
	s.eventHub.Publish(disconnectionEvent{c})
}
