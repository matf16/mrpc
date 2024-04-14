package mrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/matf16/mrpc/codec"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
)

const MagicNumber = 0x5a6b7c8d

type Option struct {
	MagicNumber    int
	CodecType      codec.Type
	ConnectTimeout time.Duration
	HandleTimeout  time.Duration
}

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeout: 10 * time.Second,
}

type request struct {
	h      *codec.Header
	argv   reflect.Value
	replyv reflect.Value
	mType  *methodType
	svc    *service
}

type Server struct {
	serviceMap sync.Map
}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

func (server *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		// handle conn
		go server.ServeConn(conn)
	}
}

func Accept(lis net.Listener) { DefaultServer.Accept(lis) }

func Register(rcvr interface{}) error { return DefaultServer.Register(rcvr) }

func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error:", err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server: invalid magic number %x\n", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s\n", opt.CodecType)
		return
	}
	server.ServeCodec(f(conn), &opt)
}

var invalidRequest = struct{}{}

func (server *Server) ServeCodec(cc codec.Codec, opt *Option) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		// read request
		req, err := server.readRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		// handle request and send back
		wg.Add(1)
		go server.handleRequest(cc, req, sending, wg, opt.HandleTimeout)
	}
	wg.Wait()

}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.svc, req.mType, err = server.findService(h.ServiceMethod)
	if err != nil {
		return nil, err
	}
	req.argv = req.mType.NewArgv()
	req.replyv = req.mType.NewReplyv()
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}

	if err := cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read body err:", err)
		return nil, err
	}

	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, r *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	// avoid goroutine leak
	sent := make(chan struct{}, 1)
	called := make(chan struct{}, 1)
	go func() {
		err := r.svc.call(r.mType, r.argv, r.replyv)
		called <- struct{}{}
		if err != nil {
			r.h.Error = err.Error()
			server.sendResponse(cc, r.h, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		server.sendResponse(cc, r.h, r.replyv.Interface(), sending)
		sent <- struct{}{}
	}()
	if timeout == 0 {
		<-called
		<-sent
		return
	}
	select {
	case <-time.After(timeout):
		r.h.Error = fmt.Errorf("rpc server: request handle timeout: expect within %d", timeout).Error()
		server.sendResponse(cc, r.h, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

func (server *Server) Register(rcvr interface{}) error {
	s := newService(rcvr)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc: service already defined: " + s.name)
	}
	return nil
}

func (server *Server) findService(serviceMethod string) (svc *service, mType *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service: " + serviceName)
		return
	}
	svc = svci.(*service)
	mType = svc.method[methodName]
	if mType == nil {
		err = errors.New("rpc server: can't find method: " + methodName)
	}
	return
}

const (
	connected        = "200 Connected to m RPC"
	defaultRPCPath   = "/_mrpc_"
	defaultDebugPath = "/debug/mrpc"
)

func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 must CONNECT\n")
		return
	}
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("rpc hijacking: ", r.RemoteAddr, " : ", err.Error())
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
	server.ServeConn(conn)
}

func (server *Server) HandleHTTP() {
	http.Handle(defaultRPCPath, server)
	http.Handle(defaultDebugPath, debugHTTP{server})
	log.Println("rpc server debug on: " + defaultDebugPath)
}

func HandleHTTP() {
	DefaultServer.HandleHTTP()
}
