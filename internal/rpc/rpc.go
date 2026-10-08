package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	wsstream "github.com/sourcegraph/jsonrpc2/websocket"

	"eq/dcommsctl/internal/log"
)

var (
	ctxType = reflect.TypeFor[context.Context]()
	errType = reflect.TypeFor[error]()
)

type rpcFunc struct {
	fn         reflect.Value
	wantCtx    bool
	argType    reflect.Type
	resultType reflect.Type
}

type rpcMethodSpec struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
	Result map[string]any `json:"result"`
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

	f := rpcFunc{
		fn:         v,
		resultType: t.Out(0),
	}

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

func schemaFor(t reflect.Type) any {
	if t == nil {
		return nil
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		fields := make(map[string]any)

		for field := range t.Fields() {

			if !field.IsExported() {
				continue
			}

			tag := field.Tag.Get("json")
			name, options := parseJSONTag(tag)

			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}

			fieldSchema := schemaFor(field.Type)

			if field.Anonymous && tag == "" {
				if embedded, ok := fieldSchema.(map[string]any); ok {
					maps.Copy(fields, embedded)
					continue
				}
			}

			if options["omitempty"] {
				fields[name] = map[string]any{
					"type":     fieldSchema,
					"optional": true,
				}
			} else {
				fields[name] = fieldSchema
			}
		}

		return fields

	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": schemaFor(t.Elem()),
		}

	case reflect.Map:
		return map[string]any{
			"type":   "object",
			"values": schemaFor(t.Elem()),
		}

	case reflect.String:
		return "string"

	case reflect.Bool:
		return "boolean"

	case reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64:
		return "integer"

	case reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64:
		return "integer"

	case reflect.Float32, reflect.Float64:
		return "number"

	case reflect.Interface:
		return "any"

	default:
		return t.String()
	}
}

func parseJSONTag(tag string) (string, map[string]bool) {
	options := make(map[string]bool)

	if tag == "" {
		return "", options
	}

	parts := strings.Split(tag, ",")
	for _, option := range parts[1:] {
		options[option] = true
	}

	return parts[0], options
}

func (r *registry) SaveSpecs(filename string) error {
	specs := make([]rpcMethodSpec, 0, len(r.methods))

	for name, method := range r.methods {
		spec := rpcMethodSpec{Name: name}

		if method.argType != nil {
			spec.Params, _ = schemaFor(method.argType).(map[string]any)
		}
		if method.resultType != nil {
			spec.Result, _ = schemaFor(method.resultType).(map[string]any)
		}

		specs = append(specs, spec)
	}

	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})

	data, err := json.MarshalIndent(specs, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')

	return os.WriteFile(filename, data, 0o644)
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

type Server struct {
	// Logger, if non-nil, receives connect/disconnect and cleanup messages.
	Logger *slog.Logger

	Reg      *registry
	upgrader websocket.Upgrader
}

func NewJRPCServer() *Server {
	return &Server{
		Reg:      newRegistry(),
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
func (s *Server) Register(name string, fn any) { s.Reg.register(name, fn) }

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
		s.Logger.Info(fmt.Sprintf(format, args...))
	}
}

type connHandler struct {
	srv  *Server
	sess *session
}

type ctxKey int

const (
	connKey ctxKey = iota
	sessKey
)

func (h *connHandler) Handle(ctx context.Context, c *jsonrpc2.Conn, req *jsonrpc2.Request) {
	ctx = context.WithValue(ctx, connKey, c)
	ctx = context.WithValue(ctx, sessKey, h.sess)

	// Params are logged raw rather than re-encoded: they arrive as *json.RawMessage and
	// jsonValue passes that through, preserving the exact wire form (including key
	// order) that a client and server disagreeing about is worth seeing.
	log.LogJSON("--> request", req.Params, "method", req.Method, "id", req.ID)

	result, err := h.srv.Reg.call(ctx, req.Method, req.Params)
	if req.Notif {
		return
	}
	if err != nil {
		var rpcErr *jsonrpc2.Error
		if !errors.As(err, &rpcErr) {
			rpcErr = &jsonrpc2.Error{Code: jsonrpc2.CodeInternalError, Message: err.Error()}
		}
		log.LogJSON("<-- error", nil,
			"method", req.Method, "id", req.ID, "code", rpcErr.Code, "message", rpcErr.Message)
		c.ReplyWithError(ctx, req.ID, rpcErr)
		return
	}

	log.LogJSON("<-- result", result, "method", req.Method, "id", req.ID)

	c.Reply(ctx, req.ID, result)
}
