package streams

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// A minimal RFC 6455 websocket client, just enough to drive CamillaDSP's
// control socket on 127.0.0.1: one request/response of text frames per
// connection, client-side masking, ping/pong, fragmented replies and a clean
// close handshake (CamillaDSP warns about every connection dropped without a
// close frame). No extensions, no TLS, no proxies.

const (
	wsGUID          = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	wsOpContinue    = 0x0
	wsOpText        = 0x1
	wsOpBinary      = 0x2
	wsOpClose       = 0x8
	wsOpPing        = 0x9
	wsOpPong        = 0xA
	wsMaxMessageLen = 4 << 20 // CamillaDSP replies are small; a full config is a few KB
)

var errWSClosed = errors.New("websocket closed by peer")

type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
}

// wsDial opens a websocket to ws://addr/ and completes the handshake. The
// deadline bounds the whole exchange that follows as well.
func wsDial(addr string, deadline time.Time) (*wsConn, error) {
	d := net.Dialer{Deadline: deadline}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(deadline)

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	req := "GET / HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("websocket handshake: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("websocket handshake: unexpected status %s", resp.Status)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		conn.Close()
		return nil, errors.New("websocket handshake: missing Upgrade: websocket")
	}
	if got, want := resp.Header.Get("Sec-WebSocket-Accept"), wsAccept(key); got != want {
		conn.Close()
		return nil, errors.New("websocket handshake: bad Sec-WebSocket-Accept")
	}
	return &wsConn{conn: conn, br: br}, nil
}

// wsAccept computes the Sec-WebSocket-Accept value for a client key.
func wsAccept(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// writeFrame sends one final, masked frame.
func (w *wsConn) writeFrame(op byte, payload []byte) error {
	hdr := make([]byte, 0, 14)
	hdr = append(hdr, 0x80|op)
	n := len(payload)
	switch {
	case n < 126:
		hdr = append(hdr, 0x80|byte(n))
	case n <= 0xFFFF:
		hdr = append(hdr, 0x80|126)
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(n))
	default:
		hdr = append(hdr, 0x80|127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	hdr = append(hdr, mask[:]...)
	buf := make([]byte, len(hdr)+n)
	copy(buf, hdr)
	for i, b := range payload {
		buf[len(hdr)+i] = b ^ mask[i%4]
	}
	_, err := w.conn.Write(buf)
	return err
}

// readFrame reads one frame. Server frames should not be masked, but a masked
// one is unmasked rather than rejected.
func (w *wsConn) readFrame() (fin bool, op byte, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(w.br, h[:]); err != nil {
		return
	}
	fin = h[0]&0x80 != 0
	op = h[0] & 0x0F
	masked := h[1]&0x80 != 0
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(w.br, b[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(w.br, b[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > wsMaxMessageLen {
		err = fmt.Errorf("websocket frame too large (%d bytes)", n)
		return
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(w.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(w.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}

// WriteText sends a text message.
func (w *wsConn) WriteText(msg []byte) error { return w.writeFrame(wsOpText, msg) }

// ReadMessage returns the next text or binary message, answering pings and
// joining fragments.
func (w *wsConn) ReadMessage() ([]byte, error) {
	var msg []byte
	inMessage := false
	for {
		fin, op, payload, err := w.readFrame()
		if err != nil {
			return nil, err
		}
		switch op {
		case wsOpPing:
			if err := w.writeFrame(wsOpPong, payload); err != nil {
				return nil, err
			}
			continue
		case wsOpPong:
			continue
		case wsOpClose:
			_ = w.writeFrame(wsOpClose, payload) // echo, completing the handshake
			return nil, errWSClosed
		case wsOpText, wsOpBinary:
			if inMessage {
				return nil, errors.New("websocket: new message inside a fragmented one")
			}
			msg = append(msg[:0], payload...)
			inMessage = true
		case wsOpContinue:
			if !inMessage {
				return nil, errors.New("websocket: continuation without a message")
			}
			msg = append(msg, payload...)
		default:
			return nil, fmt.Errorf("websocket: unknown opcode %#x", op)
		}
		if len(msg) > wsMaxMessageLen {
			return nil, errors.New("websocket message too large")
		}
		if fin {
			return msg, nil
		}
	}
}

// Close sends a normal-closure frame, waits briefly for the peer's close
// frame, and closes the connection.
func (w *wsConn) Close() error {
	_ = w.writeFrame(wsOpClose, []byte{0x03, 0xE8}) // 1000 normal closure
	_ = w.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	for {
		_, op, _, err := w.readFrame()
		if err != nil || op == wsOpClose {
			break
		}
	}
	return w.conn.Close()
}

// camillaCommand sends one command to CamillaDSP's websocket at addr and
// returns the reply's "value" (nil for commands without one). cmd is either a
// bare command name ("GetState") or a one-key object ({"SetConfig": "..."}).
// A reply whose "result" is anything other than "Ok" is an error.
func camillaCommand(addr string, cmd any, timeout time.Duration) (json.RawMessage, error) {
	name, ok := cmd.(string)
	if !ok {
		m, isMap := cmd.(map[string]any)
		if !isMap || len(m) != 1 {
			return nil, errors.New("camilladsp: command must be a name or a one-key object")
		}
		for k := range m {
			name = k
		}
	}
	body, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}

	ws, err := wsDial(addr, time.Now().Add(timeout))
	if err != nil {
		return nil, fmt.Errorf("camilladsp %s: %w", name, err)
	}
	defer ws.Close()
	if err := ws.WriteText(body); err != nil {
		return nil, fmt.Errorf("camilladsp %s: %w", name, err)
	}
	reply, err := ws.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("camilladsp %s: %w", name, err)
	}

	var parsed map[string]struct {
		Result json.RawMessage `json:"result"`
		Value  json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(reply, &parsed); err != nil {
		return nil, fmt.Errorf("camilladsp %s: bad reply %q: %w", name, truncate(reply, 200), err)
	}
	r, ok := parsed[name]
	if !ok {
		return nil, fmt.Errorf("camilladsp %s: reply for another command: %q", name, truncate(reply, 200))
	}
	if string(r.Result) != `"Ok"` {
		return nil, fmt.Errorf("camilladsp %s: %s", name, truncate(r.Result, 300))
	}
	return r.Value, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
