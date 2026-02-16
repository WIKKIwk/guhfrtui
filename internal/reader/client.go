package reader

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"
)

// Endpoint describes a reachable reader address.
type Endpoint struct {
	Host string
	Port int
}

func (e Endpoint) Address() string {
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

// Packet is raw bytes received from reader.
type Packet struct {
	When time.Time
	Data []byte
}

type session struct {
	endpoint Endpoint
	conn     net.Conn
	packets  chan Packet
	errs     chan error
	done     chan struct{}
}

// Client manages a single reader TCP session.
type Client struct {
	mu      sync.RWMutex
	session *session
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Connect(ctx context.Context, endpoint Endpoint, timeout time.Duration) error {
	if endpoint.Host == "" || endpoint.Port <= 0 {
		return fmt.Errorf("invalid endpoint")
	}

	c.mu.Lock()
	if c.session != nil {
		c.mu.Unlock()
		return fmt.Errorf("already connected")
	}
	c.mu.Unlock()

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", endpoint.Address())
	if err != nil {
		return err
	}

	s := &session{
		endpoint: endpoint,
		conn:     conn,
		packets:  make(chan Packet, 256),
		errs:     make(chan error, 32),
		done:     make(chan struct{}),
	}

	c.mu.Lock()
	if c.session != nil {
		c.mu.Unlock()
		_ = conn.Close()
		return fmt.Errorf("already connected")
	}
	c.session = s
	c.mu.Unlock()

	go c.readLoop(s)
	return nil
}

func (c *Client) readLoop(s *session) {
	defer func() {
		close(s.packets)
		close(s.errs)
		close(s.done)

		c.mu.Lock()
		if c.session == s {
			c.session = nil
		}
		c.mu.Unlock()
	}()

	buf := make([]byte, 4096)
	for {
		n, err := s.conn.Read(buf)
		if err != nil {
			select {
			case s.errs <- err:
			default:
			}
			return
		}
		if n <= 0 {
			continue
		}

		data := make([]byte, n)
		copy(data, buf[:n])
		packet := Packet{When: time.Now(), Data: data}
		select {
		case s.packets <- packet:
		default:
		}
	}
}

func (c *Client) Disconnect() error {
	c.mu.RLock()
	s := c.session
	c.mu.RUnlock()
	if s == nil {
		return nil
	}

	if err := s.conn.Close(); err != nil {
		return err
	}

	select {
	case <-s.done:
	case <-time.After(1200 * time.Millisecond):
	}
	return nil
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session != nil
}

func (c *Client) Endpoint() (Endpoint, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.session == nil {
		return Endpoint{}, false
	}
	return c.session.endpoint, true
}

func (c *Client) Packets() <-chan Packet {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.session == nil {
		return nil
	}
	return c.session.packets
}

func (c *Client) Errors() <-chan error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.session == nil {
		return nil
	}
	return c.session.errs
}

func (c *Client) SendRaw(data []byte, timeout time.Duration) error {
	if len(data) == 0 {
		return fmt.Errorf("empty payload")
	}

	c.mu.RLock()
	s := c.session
	c.mu.RUnlock()
	if s == nil {
		return fmt.Errorf("not connected")
	}

	_ = s.conn.SetWriteDeadline(time.Now().Add(timeout))
	_, err := s.conn.Write(data)
	return err
}
