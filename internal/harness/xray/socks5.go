package xray

import (
	"context"
	"fmt"
	"net"
	"strconv"
)

// dialSocks5 implements the minimal SOCKS5 CONNECT handshake (RFC 1928,
// no-auth only — matches xray-core's SOCKS inbound default) needed to
// route an http.Transport's dials through xray's local proxy inbound.
// Kept in-tree instead of pulling golang.org/x/net/proxy to avoid an
// extra dependency for ~40 lines of protocol.
func dialSocks5(ctx context.Context, socksAddr, targetAddr string) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", socksAddr)
	if err != nil {
		return nil, fmt.Errorf("dial socks proxy: %w", err)
	}

	// Greeting: version 5, 1 auth method, no-auth (0x00).
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	resp := make([]byte, 2)
	if _, err := readFull(conn, resp); err != nil {
		conn.Close()
		return nil, err
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5: unsupported auth method response %v", resp)
	}

	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		conn.Close()
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		conn.Close()
		return nil, err
	}

	req := []byte{0x05, 0x01, 0x00} // ver, CMD=CONNECT, RSV
	req = append(req, 0x03)         // ATYP=domain name
	req = append(req, byte(len(host)))
	req = append(req, []byte(host)...)
	req = append(req, byte(port>>8), byte(port&0xff))

	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}

	// Reply header: ver, rep, rsv, atyp (then variable address + 2-byte port).
	head := make([]byte, 4)
	if _, err := readFull(conn, head); err != nil {
		conn.Close()
		return nil, err
	}
	if head[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("socks5: connect failed, reply code %d", head[1])
	}

	var addrLen int
	switch head[3] {
	case 0x01:
		addrLen = 4
	case 0x03:
		lb := make([]byte, 1)
		if _, err := readFull(conn, lb); err != nil {
			conn.Close()
			return nil, err
		}
		addrLen = int(lb[0])
	case 0x04:
		addrLen = 16
	default:
		conn.Close()
		return nil, fmt.Errorf("socks5: unknown address type %d", head[3])
	}
	skip := make([]byte, addrLen+2) // address + port
	if _, err := readFull(conn, skip); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
