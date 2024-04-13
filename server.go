package mrpc

import (
	"encoding/json"
	"fmt"
	"github.com/matf16/mrpc/codec"
	"github.com/matf16/mrpc/option"
	"github.com/matf16/mrpc/request"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

type Server struct{}

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

func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }()
	var opt option.Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error:", err)
		return
	}
	if opt.MagicNumber != option.MagicNumber {
		log.Printf("rpc server: invalid magic number %x\n", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s\n", opt.CodecType)
		return
	}
	server.ServeCodec(f(conn))
}

var invalidRequest = struct{}{}

func (server *Server) ServeCodec(cc codec.Codec) {
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		// read request
		req, err := server.ReadRequest(cc)
		if err != nil {
			if req == nil {
				break
			}
			req.H.Error = err.Error()
			server.SendResponse(cc, req.H, invalidRequest, sending)
			continue
		}
		// handle request and send back
		wg.Add(1)
		go server.HandleRequest(cc, req, sending, wg)
	}
	wg.Wait()

}

func (server *Server) ReadRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (server *Server) ReadRequest(cc codec.Codec) (*request.Request, error) {
	h, err := server.ReadRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request.Request{H: h}
	req.Argv = reflect.New(reflect.TypeOf(""))
	if err := cc.ReadBody(req.Argv.Interface()); err != nil {
		log.Println("rpc server: read argv error:", err)
		return nil, err
	}
	return req, nil
}

func (server *Server) SendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) HandleRequest(cc codec.Codec, r *request.Request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println(r.H, r.Argv.Elem())
	r.Replyv = reflect.ValueOf(fmt.Sprintf("mrpc resp %d", r.H.Seq))
	server.SendResponse(cc, r.H, r.Replyv.Interface(), sending)
}
