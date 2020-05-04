// DO NOT MODIFY THIS FILE!

package rdp

import (
	"fmt"
)

// MsgType is an integer code describing an LSP message type.
type MsgType int

const (
	MsgConnect MsgType = iota // Sent by clients to make a connection w/ the server.
	MsgData                   // Sent by clients/servers to send data.
	MsgAck                    // Sent by clients/servers to ack connect/data msgs.
)

// Message represents a message used by the LSP protocol.
type Message struct {
	Type   MsgType // One of the message types listed above.
	ConnID int     // Unique client-server connection ID.
	SeqNum int     // Message sequence number.
	// Size   int     // Size of the payload.
	// Checksum uint16  // Message checksum.
	Payload []byte // Data message payload.
}

// 数据完整性校验。是否有必要？
// 因为UDP的首部校验和会校验首部以及应用层的payload。
// 看起来如果应用层也校验的话，似乎多此一举了。
// 同理UDP也有报文长度字段，所以应用层消息格式就不必再包含消息长度字段了。

// NewConnect returns a new connect message.
func NewConnect() *Message {
	return &Message{Type: MsgConnect}
}

// NewData returns a new data message with the specified connection ID,
// sequence number, and payload.
// func NewData(connID, seqNum, size int, payload []byte, checksum uint16) *Message {
func NewData(connID, seqNum int, payload []byte) *Message {
	return &Message{
		Type:   MsgData,
		ConnID: connID,
		SeqNum: seqNum,
		// Size:    size,
		Payload: payload,
		// Checksum: checksum,
	}
}

// NewAck returns a new acknowledgement message with the specified
// connection ID and sequence number.
func NewAck(connID, seqNum int) *Message {
	return &Message{
		Type:   MsgAck,
		ConnID: connID,
		SeqNum: seqNum,
	}
}

// String returns a string representation of this message. To pretty-print a
// message, you can pass it to a format string like so:
//     msg := NewConnect()
//     fmt.Printf("Connect message: %s\n", msg)
func (m *Message) String() string {
	var name, payload, checksum string
	switch m.Type {
	case MsgConnect:
		name = "Connect"
	case MsgData:
		name = "Data"
		// checksum = " " + strconv.Itoa(int(m.Checksum))
		payload = " " + string(m.Payload)
	case MsgAck:
		name = "Ack"
	}
	return fmt.Sprintf("[%s %d %d%s%s]", name, m.ConnID, m.SeqNum, checksum, payload)
}
