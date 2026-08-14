package kcp

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"
)

type channelConn struct {
	mu        sync.Mutex
	channel   *KChannel
	buffer    bytes.Buffer
	inbound   chan []byte
	done      chan struct{}
	closed    bool
	closeOnce sync.Once

	readDeadline  time.Time
	writeDeadline time.Time
}

func newChannelConn(channel *KChannel) *channelConn {
	return &channelConn{
		channel: channel,
		inbound: make(chan []byte, 16),
		done:    make(chan struct{}),
	}
}

func (c *channelConn) pushInbound(data []byte) error {
	if c == nil {
		return ErrChannelClosed
	}
	copyData := append([]byte(nil), data...)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrChannelClosed
	}
	done := c.done
	c.mu.Unlock()

	select {
	case <-done:
		return ErrChannelClosed
	case c.inbound <- copyData:
		return nil
	default:
		return ErrReceiveBufferFull
	}
}

func (c *channelConn) Read(p []byte) (int, error) {
	if c == nil {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	for {
		c.mu.Lock()
		if c.buffer.Len() > 0 {
			n, err := c.buffer.Read(p)
			c.mu.Unlock()
			return n, err
		}
		closed := c.closed
		done := c.done
		deadline := c.readDeadline
		c.mu.Unlock()

		if closed {
			return 0, io.EOF
		}
		timer, timerC, err := deadlineTimer(deadline)
		if err != nil {
			return 0, err
		}
		select {
		case <-done:
			stopTimer(timer)
			return 0, io.EOF
		case data := <-c.inbound:
			stopTimer(timer)
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				return 0, io.EOF
			}
			if _, err := c.buffer.Write(data); err != nil {
				c.mu.Unlock()
				return 0, err
			}
			c.mu.Unlock()
		case <-timerC:
			stopTimer(timer)
			return 0, errKCPDeadlineExceeded
		}
	}
}

func (c *channelConn) Write(p []byte) (int, error) {
	if c == nil {
		return 0, io.ErrClosedPipe
	}
	c.mu.Lock()
	closed := c.closed
	channel := c.channel
	deadline := c.writeDeadline
	c.mu.Unlock()
	if closed || channel == nil {
		return 0, io.ErrClosedPipe
	}
	if timer, _, err := deadlineTimer(deadline); err != nil {
		return 0, err
	} else {
		stopTimer(timer)
	}
	if err := channel.Send(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *channelConn) Close() error {
	if c != nil {
		c.close()
		if c.channel != nil {
			c.channel.Close()
		}
	}
	return nil
}

func (c *channelConn) close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		done := c.done
		c.mu.Unlock()
		close(done)
	})
}

func (c *channelConn) LocalAddr() net.Addr {
	if c == nil || c.channel == nil || c.channel.service == nil {
		return nil
	}
	return c.channel.service.Addr()
}

func (c *channelConn) RemoteAddr() net.Addr {
	if c == nil || c.channel == nil {
		return nil
	}
	return c.channel.RemoteAddr()
}

func (c *channelConn) SetDeadline(t time.Time) error {
	if c == nil {
		return io.ErrClosedPipe
	}
	c.mu.Lock()
	c.readDeadline = t
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}

func (c *channelConn) SetReadDeadline(t time.Time) error {
	if c == nil {
		return io.ErrClosedPipe
	}
	c.mu.Lock()
	c.readDeadline = t
	c.mu.Unlock()
	return nil
}

func (c *channelConn) SetWriteDeadline(t time.Time) error {
	if c == nil {
		return io.ErrClosedPipe
	}
	c.mu.Lock()
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}

func deadlineTimer(deadline time.Time) (*time.Timer, <-chan time.Time, error) {
	if deadline.IsZero() {
		return nil, nil, nil
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil, nil, errKCPDeadlineExceeded
	}
	timer := time.NewTimer(remaining)
	return timer, timer.C, nil
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

type kcpDeadlineError struct{}

func (kcpDeadlineError) Error() string   { return "kcp: i/o deadline exceeded" }
func (kcpDeadlineError) Timeout() bool   { return true }
func (kcpDeadlineError) Temporary() bool { return true }

var errKCPDeadlineExceeded error = kcpDeadlineError{}
