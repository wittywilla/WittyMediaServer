package portmap

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// natpmpClient speaks NAT-PMP (RFC 6886) to one gateway.
type natpmpClient struct {
	addr    string // host:port of the gateway
	timeout time.Duration
}

func (c *natpmpClient) Protocol() string { return "NAT-PMP" }

// ask sends a request and returns the matching answer, trying twice.
func (c *natpmpClient) ask(ctx context.Context, req []byte, size int) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp4", c.addr) // connected: only this address can answer
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	buf := make([]byte, 64)
	for try := range 2 {
		_ = conn.SetDeadline(time.Now().Add(c.timeout << try))
		if _, err := conn.Write(req); err != nil {
			return nil, err
		}
		n, err := conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if n < size || buf[0] != 0 || buf[1] != req[1]+128 {
			continue // not the answer to this question
		}
		if code := binary.BigEndian.Uint16(buf[2:4]); code != 0 {
			return nil, fmt.Errorf("the router refused (NAT-PMP result %d)", code)
		}
		return buf[:n], nil
	}
	return nil, errors.New("no answer")
}

func (c *natpmpClient) ExternalIP(ctx context.Context) (netip.Addr, error) {
	b, err := c.ask(ctx, []byte{0, 0}, 12)
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.AddrFrom4([4]byte(b[8:12])), nil
}

func (c *natpmpClient) mapRequest(external, internal int, lease uint32) ([]byte, error) {
	req := make([]byte, 12)
	req[1] = 2 // TCP
	binary.BigEndian.PutUint16(req[4:], uint16(internal))
	binary.BigEndian.PutUint16(req[6:], uint16(external))
	binary.BigEndian.PutUint32(req[8:], lease)
	return req, nil
}

func (c *natpmpClient) Map(ctx context.Context, external, internal int) (int, time.Duration, error) {
	req, _ := c.mapRequest(external, internal, leaseSeconds)
	b, err := c.ask(ctx, req, 16)
	if err != nil {
		return 0, 0, err
	}
	return int(binary.BigEndian.Uint16(b[10:12])), time.Duration(binary.BigEndian.Uint32(b[12:16])) * time.Second, nil
}

func (c *natpmpClient) Unmap(ctx context.Context, external, internal int) error {
	req, _ := c.mapRequest(0, internal, 0) // lease 0 and port 0 remove it (RFC 6886, 3.4)
	_, err := c.ask(ctx, req, 16)
	return err
}
