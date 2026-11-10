package messaging

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/endpoint"
)

// Handler serves requests that passed authentication.
type Handler interface {
	Info() Info
	Message(Envelope) Receipt
}

const (
	maxConnections = 16
	maxSocketPath  = 100
)

// Server listens on one Unix socket and serves authenticated requests.
type Server struct {
	listener net.Listener
	path     string
	token    string
	handler  Handler
	slots    chan struct{}
	wg       sync.WaitGroup
	once     sync.Once
}

// Listen binds path (replacing a stale socket file) and serves handler until Close.
func Listen(path, token string, handler Handler) (*Server, error) {
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("socket path is %d bytes; the limit is %d", len(path), maxSocketPath)
	}
	if token == "" {
		return nil, errors.New("a session token is required")
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("socket path is occupied by a file that is not a socket")
		}
		if conn, dialErr := net.DialTimeout("unix", path, time.Second); dialErr == nil {
			conn.Close()
			return nil, errors.New("another process is serving this endpoint")
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	s := &Server{listener: listener, path: path, token: token, handler: handler, slots: make(chan struct{}, maxConnections)}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}

// Close stops accepting, waits for running requests and removes the socket file.
func (s *Server) Close() {
	s.once.Do(func() {
		s.listener.Close()
		s.wg.Wait()
		os.Remove(s.path)
	})
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		select {
		case s.slots <- struct{}{}:
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer func() { <-s.slots }()
				s.serve(conn)
			}()
		default:
			conn.SetDeadline(time.Now().Add(time.Second))
			_ = writeFrame(conn, Reply{Receipt: &Receipt{Status: Rejected, Reason: Busy}})
			conn.Close()
		}
	}
}

func (s *Server) serve(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(CallTimeout))
	if !peerIsCurrentUser(conn) {
		return
	}
	var first hello
	if err := readFrame(conn, &first); err != nil || first.Type != "hello" || first.V != ProtocolVersion || !endpoint.SameToken(first.Token, s.token) {
		return
	}
	var request Request
	if err := readFrame(conn, &request); err != nil {
		return
	}
	switch request.Type {
	case "info":
		info := s.handler.Info()
		_ = writeFrame(conn, Reply{Info: &info})
	case "message":
		if request.Message == nil {
			_ = writeFrame(conn, Reply{Error: "message is missing"})
			return
		}
		if err := request.Message.Validate(); err != nil || len(request.Message.Body) > MaxBody {
			reason := TooLarge
			if err != nil {
				_ = writeFrame(conn, Reply{Error: err.Error()})
				return
			}
			_ = writeFrame(conn, Reply{Receipt: &Receipt{Status: Rejected, Reason: reason}})
			return
		}
		receipt := s.handler.Message(*request.Message)
		_ = writeFrame(conn, Reply{Receipt: &receipt})
	default:
		_ = writeFrame(conn, Reply{Error: "unknown request type"})
	}
}

// Call sends one request to the endpoint at path with its token and returns the reply.
func Call(path, token, fromEndpoint string, request Request) (Reply, error) {
	var reply Reply
	conn, err := net.DialTimeout("unix", path, CallTimeout)
	if err != nil {
		return reply, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(CallTimeout))
	if err := writeFrame(conn, hello{V: ProtocolVersion, Type: "hello", Token: token, From: fromEndpoint}); err != nil {
		return reply, err
	}
	request.V = ProtocolVersion
	if err := writeFrame(conn, request); err != nil {
		return reply, err
	}
	if err := readFrame(conn, &reply); err != nil {
		return reply, fmt.Errorf("no reply (wrong token, other user or session gone): %w", err)
	}
	if reply.Error != "" {
		return reply, errors.New(reply.Error)
	}
	return reply, nil
}
