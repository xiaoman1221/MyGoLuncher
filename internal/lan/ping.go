package lan

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	statusProtocolVersion = 47
	maxStatusPacket       = 2 << 20
)

type pingResult struct {
	MOTD    string
	Version string
	Online  int
	Max     int
	Players []PlayerInfo
}

type statusResponse struct {
	Version struct {
		Name string `json:"name"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
		} `json:"sample"`
	} `json:"players"`
	Description json.RawMessage `json:"description"`
}

// pingMinecraft performs a modern (1.7+) Server List Ping.
func pingMinecraft(ctx context.Context, host string, port int, timeout time.Duration) (pingResult, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return pingResult{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	handshake := &bytes.Buffer{}
	writeVarInt(handshake, 0) // packet id
	writeVarInt(handshake, statusProtocolVersion)
	writeString(handshake, strings.Trim(host, "[]"))
	_ = binary.Write(handshake, binary.BigEndian, uint16(port))
	writeVarInt(handshake, 1) // next state: status
	if err := writePacket(conn, handshake.Bytes()); err != nil {
		return pingResult{}, err
	}
	request := &bytes.Buffer{}
	writeVarInt(request, 0)
	if err := writePacket(conn, request.Bytes()); err != nil {
		return pingResult{}, err
	}

	payload, err := readPacket(conn)
	if err != nil {
		return pingResult{}, err
	}
	reader := bytes.NewReader(payload)
	packetID, err := readVarInt(reader)
	if err != nil {
		return pingResult{}, err
	}
	if packetID != 0 {
		return pingResult{}, errors.New("Minecraft 状态响应无效")
	}
	raw, err := readString(reader)
	if err != nil {
		return pingResult{}, err
	}
	var status statusResponse
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return pingResult{}, fmt.Errorf("解析 Minecraft 状态失败: %w", err)
	}
	result := pingResult{
		MOTD:    cleanMOTD(flattenChat(status.Description)),
		Version: strings.TrimSpace(status.Version.Name),
		Online:  status.Players.Online,
		Max:     status.Players.Max,
	}
	seen := map[string]bool{}
	for _, sample := range status.Players.Sample {
		name := strings.TrimSpace(sample.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		result.Players = append(result.Players, PlayerInfo{Name: name})
	}
	return result, nil
}

func flattenChat(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var component struct {
		Text      string            `json:"text"`
		Translate string            `json:"translate"`
		Extra     []json.RawMessage `json:"extra"`
	}
	if err := json.Unmarshal(raw, &component); err != nil {
		return ""
	}
	var builder strings.Builder
	if component.Text != "" {
		builder.WriteString(component.Text)
	} else if component.Translate != "" {
		builder.WriteString(component.Translate)
	}
	for _, extra := range component.Extra {
		builder.WriteString(flattenChat(extra))
	}
	return builder.String()
}

func writePacket(w io.Writer, payload []byte) error {
	buffer := &bytes.Buffer{}
	writeVarInt(buffer, int32(len(payload)))
	buffer.Write(payload)
	_, err := w.Write(buffer.Bytes())
	return err
}

func readPacket(r io.Reader) ([]byte, error) {
	length, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if length <= 0 || int(length) > maxStatusPacket {
		return nil, errors.New("Minecraft 数据包长度无效")
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeVarInt(w io.Writer, value int32) {
	unsigned := uint32(value)
	for {
		if unsigned&^0x7f == 0 {
			_, _ = w.Write([]byte{byte(unsigned)})
			return
		}
		_, _ = w.Write([]byte{byte(unsigned&0x7f | 0x80)})
		unsigned >>= 7
	}
}

func readVarInt(r io.Reader) (int32, error) {
	var value uint32
	for shift := uint(0); shift < 35; shift += 7 {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		value |= uint32(b[0]&0x7f) << shift
		if b[0]&0x80 == 0 {
			return int32(value), nil
		}
	}
	return 0, errors.New("Minecraft VarInt 过长")
}

func writeString(w io.Writer, value string) {
	writeVarInt(w, int32(len(value)))
	_, _ = io.WriteString(w, value)
}

func readString(r io.Reader) (string, error) {
	length, err := readVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 || length > maxStatusPacket {
		return "", errors.New("Minecraft 字符串长度无效")
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(r, data); err != nil {
		return "", err
	}
	return string(data), nil
}
