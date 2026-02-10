package websocket

import "time"

// Transport defines an abstraction layer for protocol-independent message delivery
type Transport interface {
	Send(message []byte) error
	Close(code int, reason string) error
	SetReadDeadline(deadline time.Time) error
	SetWriteDeadline(deadline time.Time) error
	EnablePongHandler(handler func(string) error)
	SendPing() error
	ReadMessage() (int, []byte, error)
}
