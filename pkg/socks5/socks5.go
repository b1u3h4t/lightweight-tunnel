package socks5

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
)

const (
	socks5Version = 0x05

	authMethodNoAuth   = 0x00
	authMethodUsername = 0x02
	authMethodNoAccept = 0xFF

	cmdConnect = 0x01
	cmdBind    = 0x02
	cmdUDP     = 0x03

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSuccess              = 0x00
	repGeneralFailure       = 0x01
	repConnectionNotAllowed = 0x02
	repNetworkUnreachable   = 0x03
	repHostUnreachable      = 0x04
	repConnectionRefused    = 0x05
	repTTLExpired           = 0x06
	repCommandNotSupported  = 0x07
	repAddressNotSupported  = 0x08
)

type Server struct {
	addr   string
	logger *log.Logger
}

type Config struct {
	ListenAddr string
	Username   string
	Password   string
}

func NewServer(cfg *Config) *Server {
	return &Server{
		addr:   cfg.ListenAddr,
		logger: log.Default(),
	}
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	defer listener.Close()

	s.logger.Printf("SOCKS5 proxy listening on %s", s.addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.logger.Printf("Accept error: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	s.logger.Printf("New SOCKS5 connection from %s", clientAddr)

	if err := s.handshake(conn); err != nil {
		s.logger.Printf("Handshake failed for %s: %v", clientAddr, err)
		return
	}

	target, err := s.parseRequest(conn)
	if err != nil {
		s.logger.Printf("Request parse failed for %s: %v", clientAddr, err)
		return
	}

	s.logger.Printf("SOCKS5 connect: %s -> %s", clientAddr, target)

	remote, err := net.Dial("tcp", target)
	if err != nil {
		s.sendReply(conn, repHostUnreachable, nil)
		s.logger.Printf("Dial failed for %s -> %s: %v", clientAddr, target, err)
		return
	}
	defer remote.Close()

	localAddr := remote.LocalAddr().(*net.TCPAddr)
	if err := s.sendReply(conn, repSuccess, localAddr); err != nil {
		s.logger.Printf("Send reply failed for %s: %v", clientAddr, err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(remote, conn)
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, remote)
	}()

	wg.Wait()
	s.logger.Printf("SOCKS5 connection closed: %s", clientAddr)
}

func (s *Server) handshake(conn net.Conn) error {
	buf := make([]byte, 256)

	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("read version failed: %w", err)
	}

	if n < 3 || buf[0] != socks5Version {
		return errors.New("invalid socks version")
	}

	nmethods := int(buf[1])
	if n != 2+nmethods {
		return errors.New("invalid number of methods")
	}

	hasNoAuth := false
	for i := 0; i < nmethods; i++ {
		if buf[2+i] == authMethodNoAuth {
			hasNoAuth = true
			break
		}
	}

	if !hasNoAuth {
		conn.Write([]byte{socks5Version, authMethodNoAccept})
		return errors.New("no acceptable auth method")
	}

	_, err = conn.Write([]byte{socks5Version, authMethodNoAuth})
	return err
}

func (s *Server) parseRequest(conn net.Conn) (string, error) {
	buf := make([]byte, 256)

	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("read request failed: %w", err)
	}

	if n < 10 || buf[0] != socks5Version {
		return "", errors.New("invalid request")
	}

	cmd := buf[1]
	if cmd != cmdConnect {
		_ = s.sendReply(conn, repCommandNotSupported, nil)
		return "", errors.New("unsupported command")
	}

	atyp := buf[3]
	var dstAddr string
	var dstPort uint16

	switch atyp {
	case atypIPv4:
		if n < 10 {
			return "", errors.New("invalid IPv4 address")
		}
		dstAddr = net.IP(buf[4:8]).String()
		dstPort = binary.BigEndian.Uint16(buf[8:10])

	case atypDomain:
		if n < 5 {
			return "", errors.New("invalid domain length")
		}
		domainLen := int(buf[4])
		if n < 5+domainLen+2 {
			return "", errors.New("invalid domain")
		}
		dstAddr = string(buf[5 : 5+domainLen])
		dstPort = binary.BigEndian.Uint16(buf[5+domainLen : 5+domainLen+2])

	case atypIPv6:
		if n < 22 {
			return "", errors.New("invalid IPv6 address")
		}
		dstAddr = net.IP(buf[4:20]).String()
		dstPort = binary.BigEndian.Uint16(buf[20:22])

	default:
		_ = s.sendReply(conn, repAddressNotSupported, nil)
		return "", errors.New("unsupported address type")
	}

	return net.JoinHostPort(dstAddr, strconv.Itoa(int(dstPort))), nil
}

func (s *Server) sendReply(conn net.Conn, rep byte, bindAddr *net.TCPAddr) error {
	buf := make([]byte, 10)
	buf[0] = socks5Version
	buf[1] = rep
	buf[2] = 0x00

	if bindAddr != nil {
		ip := bindAddr.IP.To4()
		if ip == nil {
			buf[3] = atypIPv6
			copy(buf[4:20], bindAddr.IP)
			binary.BigEndian.PutUint16(buf[20:22], uint16(bindAddr.Port))
		} else {
			buf[3] = atypIPv4
			copy(buf[4:8], ip)
			binary.BigEndian.PutUint16(buf[8:10], uint16(bindAddr.Port))
		}
	} else {
		buf[3] = atypIPv4
	}

	_, err := conn.Write(buf)
	return err
}

func Run(addr string) error {
	cfg := &Config{
		ListenAddr: addr,
	}
	server := NewServer(cfg)
	return server.Start()
}
