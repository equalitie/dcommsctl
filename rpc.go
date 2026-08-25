package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	wsstream "github.com/sourcegraph/jsonrpc2/websocket"
)

var (
	ctxType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errType = reflect.TypeOf((*error)(nil)).Elem()
)

type rpcFunc struct {
	fn      reflect.Value
	wantCtx bool
	argType reflect.Type // nil when the function takes no args
}

type registry struct{ methods map[string]rpcFunc }

func newRegistry() *registry { return &registry{methods: map[string]rpcFunc{}} }

// register validates the signature once, at startup, and stores the function.
func (r *registry) register(name string, fn any) {
	v := reflect.ValueOf(fn)
	t := v.Type()
	if t.Kind() != reflect.Func {
		panic("rpcws: Register(" + name + "): not a function")
	}
	if t.NumOut() != 2 || t.Out(1) != errType {
		panic("rpcws: Register(" + name + "): must return (result, error)")
	}
	var f rpcFunc
	f.fn = v
	switch t.NumIn() {
	case 0:
	case 1:
		if t.In(0) == ctxType {
			f.wantCtx = true
		} else {
			f.argType = t.In(0)
		}
	case 2:
		if t.In(0) != ctxType {
			panic("rpcws: Register(" + name + "): with two params the first must be context.Context")
		}
		f.wantCtx = true
		f.argType = t.In(1)
	default:
		panic("rpcws: Register(" + name + "): want (), (args), (ctx), or (ctx, args)")
	}
	r.methods[name] = f
}

func (r *registry) call(ctx context.Context, name string, params *json.RawMessage) (any, error) {
	m, ok := r.methods[name]
	if !ok {
		return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: "method not found: " + name}
	}
	var in []reflect.Value
	if m.wantCtx {
		in = append(in, reflect.ValueOf(ctx))
	}
	if m.argType != nil {
		argPtr := reflect.New(m.argType)
		if params != nil {
			if err := json.Unmarshal(*params, argPtr.Interface()); err != nil {
				return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams, Message: err.Error()}
			}
		}
		in = append(in, argPtr.Elem())
	}
	out := m.fn.Call(in)
	if e := out[1]; !e.IsNil() {
		return nil, e.Interface().(error)
	}
	return out[0].Interface(), nil
}

// session holds the cleanup functions for one connection's active
// subscriptions, keyed by subscription id.
type session struct {
	mu   sync.Mutex
	subs map[string]func()
}

func newSession() *session { return &session{subs: map[string]func(){}} }

// add stores cleanup under a fresh id and returns the id.
func (s *session) add(cleanup func()) string {
	id := newID()
	s.mu.Lock()
	s.subs[id] = cleanup
	s.mu.Unlock()
	return id
}

// take atomically removes and returns the cleanup for id, if present. Removing
// before the caller runs it guarantees each cleanup fires at most once.
func (s *session) take(id string) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.subs[id]
	if ok {
		delete(s.subs, id)
	}
	return c, ok
}

// takeAll atomically removes and returns every remaining cleanup.
func (s *session) takeAll() []func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]func(), 0, len(s.subs))
	for id, c := range s.subs {
		all = append(all, c)
		delete(s.subs, id)
	}
	return all
}

// TODO maybe using something with a higher uniqueness guarantee
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

type Server struct {
	// Logger, if non-nil, receives connect/disconnect and cleanup messages.
	Logger *log.Logger

	reg      *registry
	upgrader websocket.Upgrader
}

func NewJRPCServer() *Server {
	return &Server{
		reg:      newRegistry(),
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

// Register exposes fn under method name. Valid ways to call:
//
//	func(args A) (R, error)
//	func() (R, error)
//	func(ctx context.Context, args A) (R, error)
//	func(ctx context.Context) (R, error)
//
// The context is optional. Only used for server-sent messages

func (s *Server) Register(name string, fn any) { s.reg.register(name, fn) }

func (s *Server) Handler() http.Handler { return s }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logf("upgrade: %v", err)
		return
	}
	sess := newSession()
	conn := jsonrpc2.NewConn(r.Context(), wsstream.NewObjectStream(ws), &connHandler{srv: s, sess: sess})
	s.logf("client connected")

	<-conn.DisconnectNotify()

	cleanups := sess.takeAll()
	s.logf("client disconnected: running %d cleanup(s)", len(cleanups))
	for _, c := range cleanups {
		c()
	}
}

func (s *Server) logf(format string, args ...any) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

type connHandler struct {
	srv  *Server
	sess *session
}

func (h *connHandler) Handle(ctx context.Context, c *jsonrpc2.Conn, req *jsonrpc2.Request) {
	ctx = context.WithValue(ctx, connKey, c)
	ctx = context.WithValue(ctx, sessKey, h.sess)

	result, err := h.srv.reg.call(ctx, req.Method, req.Params)
	if req.Notif {
		return
	}
	if err != nil {
		var rpcErr *jsonrpc2.Error
		if !errors.As(err, &rpcErr) {
			rpcErr = &jsonrpc2.Error{Code: jsonrpc2.CodeInternalError, Message: err.Error()}
		}
		c.ReplyWithError(ctx, req.ID, rpcErr)
		return
	}
	c.Reply(ctx, req.ID, result)
}

type ctxKey int

const (
	connKey ctxKey = iota
	sessKey
)

func connFromContext(ctx context.Context) *jsonrpc2.Conn {
	c, _ := ctx.Value(connKey).(*jsonrpc2.Conn)
	return c
}

func sessionFromContext(ctx context.Context) *session {
	s, _ := ctx.Value(sessKey).(*session)
	return s
}

// ---- helpers for use inside RPC functions ----

// Notify pushes a server-initiated JSON-RPC notification to the current client.
// It is safe to call from a goroutine that captured a context carrying the
// connection (the handler's ctx, or one returned by StreamContext).
func Notify(ctx context.Context, method string, params any) error {
	conn := connFromContext(ctx)
	if conn == nil {
		return errors.New("rpcws: no connection in context")
	}
	return conn.Notify(context.Background(), method, params)
}

// AddCleanup registers cleanup for the current connection and returns a unique
// subscription id. cleanup runs on Unsubscribe(id) or client disconnect,
// exactly once, and is removed the moment it is taken to run.
func AddCleanup(ctx context.Context, cleanup func()) string {
	s := sessionFromContext(ctx)
	if s == nil {
		return ""
	}
	return s.add(cleanup)
}

// StreamContext returns a cancelable context rooted independently of the request
// (so it outlives the call) that still carries the connection for Notify.
// Register its CancelFunc with AddCleanup so the stream stops on unsubscribe or
// disconnect.
func StreamContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.WithValue(context.Background(), connKey, connFromContext(ctx))
	return context.WithCancel(base)
}

// ---- built-in unsubscribe method ----

// UnsubscribeArgs is the parameter shape for the built-in Unsubscribe method.
type UnsubscribeArgs struct {
	SubscriptionID string `json:"subscription_id"`
}

// UnsubscribeResult is what Unsubscribe returns.
type UnsubscribeResult struct {
	Stopped bool `json:"stopped"`
}

// Unsubscribe is a ready-to-register method that runs and removes the cleanup
// for the given subscription id. Register it with:
//
//	s.Register("unsubscribe", rpcws.Unsubscribe)
//
// An unknown or already-stopped id returns Stopped=false rather than an error.
func Unsubscribe(ctx context.Context, a UnsubscribeArgs) (UnsubscribeResult, error) {
	s := sessionFromContext(ctx)
	if s == nil {
		return UnsubscribeResult{}, errors.New("no session available")
	}
	cleanup, ok := s.take(a.SubscriptionID)
	if !ok {
		return UnsubscribeResult{Stopped: false}, nil
	}
	cleanup()
	return UnsubscribeResult{Stopped: true}, nil
}
