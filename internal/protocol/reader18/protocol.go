package reader18

import (
	"fmt"
)

// Command codes from UHFReader18 style protocol.
const (
	CmdInventory           byte = 0x01
	CmdInventorySingle     byte = 0x0F
	CmdGetReaderInfo       byte = 0x21
	CmdSetRegion           byte = 0x22
	CmdSetScanTime         byte = 0x25
	CmdSetWorkMode         byte = 0x35
	CmdGetWorkMode         byte = 0x36
	CmdAcoustoOptic        byte = 0x33
	CmdSetOutputPower      byte = 0x2F
	CmdSetAntennaMux       byte = 0x3F
	StatusSuccess          byte = 0x00
	StatusNoTag            byte = 0x01
	StatusCmdError         byte = 0xFE
	StatusCRCError         byte = 0xFF
	DefaultReaderAddress   byte = 0x00
	BroadcastReaderAddress      = byte(0xFF)

	StatusNoTagOrTimeout byte = 0xFB
	StatusAntennaError   byte = 0xF8
)

// Frame is one decoded response frame.
type Frame struct {
	Length   byte
	Address  byte
	Command  byte
	Status   byte
	Data     []byte
	Raw      []byte
	CRCValid bool
}

// BuildCommand builds one wire packet for the given command and payload.
// Packet format: Len(1) + Adr(1) + Cmd(1) + Data(n) + CRC_L(1) + CRC_H(1)
func BuildCommand(address byte, command byte, payload []byte) []byte {
	length := byte(len(payload) + 4)
	packet := make([]byte, 0, int(length)+1)
	packet = append(packet, length, address, command)
	packet = append(packet, payload...)

	crc := crc16MCRF4XX(packet)
	packet = append(packet, byte(crc&0xFF), byte(crc>>8))
	return packet
}

// VerifyPacket checks CRC validity for a full packet.
func VerifyPacket(packet []byte) bool {
	if len(packet) < 6 {
		return false
	}
	expectedTotal := int(packet[0]) + 1
	if expectedTotal != len(packet) {
		return false
	}
	crc := crc16MCRF4XX(packet[:len(packet)-2])
	return byte(crc&0xFF) == packet[len(packet)-2] && byte(crc>>8) == packet[len(packet)-1]
}

// ParseFrames decodes as many valid frames as possible from stream data.
// It returns parsed frames and remaining bytes that were not enough for a full frame.
func ParseFrames(stream []byte) (frames []Frame, remaining []byte) {
	if len(stream) == 0 {
		return nil, nil
	}

	buf := stream
	frames = make([]Frame, 0, 4)

	for len(buf) > 0 {
		if len(buf) < 6 {
			break
		}

		total := int(buf[0]) + 1
		if total < 6 {
			buf = buf[1:]
			continue
		}
		if total > len(buf) {
			break
		}

		raw := buf[:total]
		if !VerifyPacket(raw) {
			buf = buf[1:]
			continue
		}

		dataEnd := total - 2
		data := make([]byte, dataEnd-4)
		copy(data, raw[4:dataEnd])

		frameRaw := make([]byte, total)
		copy(frameRaw, raw)

		frames = append(frames, Frame{
			Length:   raw[0],
			Address:  raw[1],
			Command:  raw[2],
			Status:   raw[3],
			Data:     data,
			Raw:      frameRaw,
			CRCValid: true,
		})

		buf = buf[total:]
	}

	remaining = make([]byte, len(buf))
	copy(remaining, buf)
	return frames, remaining
}

// InventorySingleCommand returns a one-shot inventory command.
func InventorySingleCommand(address byte) []byte {
	return BuildCommand(address, CmdInventory, nil)
}

// InventoryCommand returns legacy inventory command with TID address/length payload.
func InventoryCommand(address, tidAddr, tidLen byte) []byte {
	return BuildCommand(address, CmdInventory, []byte{tidAddr, tidLen})
}

// InventoryG2Command builds SDK-compatible inventory command (0x01).
// If tidLen is zero, payload is: Q,Session,Target,Antenna,ScanTime.
// Otherwise payload is: Q,Session,TIDAddr,TIDLen,Target,Antenna,ScanTime.
func InventoryG2Command(address, qValue, session, tidAddr, tidLen, target, antenna, scanTime byte) []byte {
	if tidLen == 0 {
		return BuildCommand(address, CmdInventory, []byte{qValue, session, target, antenna, scanTime})
	}
	return BuildCommand(address, CmdInventory, []byte{qValue, session, tidAddr, tidLen, target, antenna, scanTime})
}

// InventorySingleTagCommand sends single inventory command (0x0F).
func InventorySingleTagCommand(address byte) []byte {
	return BuildCommand(address, CmdInventorySingle, nil)
}

// GetReaderInfoCommand returns command to query module details.
func GetReaderInfoCommand(address byte) []byte {
	return BuildCommand(address, CmdGetReaderInfo, nil)
}

// SetScanTimeCommand sets inventory duration unit (100ms steps in common firmware).
func SetScanTimeCommand(address, value byte) []byte {
	return BuildCommand(address, CmdSetScanTime, []byte{value})
}

// SetOutputPowerCommand sets output power value in dBm-like scale.
func SetOutputPowerCommand(address, value byte) []byte {
	return BuildCommand(address, CmdSetOutputPower, []byte{value})
}

// SetOutputPowerByAntCommand sets output power per antenna (one byte per port).
func SetOutputPowerByAntCommand(address byte, powers []byte) []byte {
	payload := make([]byte, 0, len(powers))
	for _, p := range powers {
		payload = append(payload, p)
	}
	return BuildCommand(address, CmdSetOutputPower, payload)
}

// SetFrequencyRangeCommand sets high/low channel bytes for command 0x22.
func SetFrequencyRangeCommand(address, high, low byte) []byte {
	return BuildCommand(address, CmdSetRegion, []byte{high, low})
}

// SetWorkModeCommand sets work mode payload.
func SetWorkModeCommand(address byte, payload []byte) []byte {
	return BuildCommand(address, CmdSetWorkMode, payload)
}

// SetAntennaMuxCommand sets active antenna bitmask.
func SetAntennaMuxCommand(address, antCfg byte) []byte {
	return BuildCommand(address, CmdSetAntennaMux, []byte{antCfg})
}

// InventoryTagCount extracts tag count from inventory response payload.
func InventoryTagCount(frame Frame) (int, error) {
	if frame.Command != CmdInventory {
		return 0, fmt.Errorf("not inventory frame")
	}
	if frame.Status != StatusSuccess {
		return 0, nil
	}
	if len(frame.Data) == 0 {
		return 0, nil
	}

	// In this protocol, first byte is number of EPC IDs in response.
	return int(frame.Data[0]), nil
}

// SingleInventoryResult is decoded data from command 0x0F response.
type SingleInventoryResult struct {
	Antenna  byte
	TagCount int
	EPC      []byte
}

// InventoryG2Tag is one parsed tag from inventory command 0x01 response.
type InventoryG2Tag struct {
	Antenna int
	EPC     []byte
	RSSI    int
}

// ParseInventoryG2Tags parses inventory payload from command 0x01.
// Data format: AntMask(1), TagNum(1), repeated [EpcLen(1), EPC(n), RSSI(1)].
func ParseInventoryG2Tags(frame Frame) ([]InventoryG2Tag, error) {
	if frame.Command != CmdInventory {
		return nil, fmt.Errorf("not inventory frame")
	}
	if len(frame.Data) < 2 {
		return nil, nil
	}

	tagNum := int(frame.Data[1])
	if tagNum <= 0 {
		return nil, nil
	}

	antenna := antennaIDFromMask(frame.Data[0])
	cursor := 2
	tags := make([]InventoryG2Tag, 0, tagNum)
	for i := 0; i < tagNum; i++ {
		if cursor >= len(frame.Data) {
			return nil, fmt.Errorf("inventory payload truncated at tag %d", i)
		}
		epcLen := int(frame.Data[cursor])
		cursor++
		if epcLen <= 0 || cursor+epcLen > len(frame.Data) {
			return nil, fmt.Errorf("inventory invalid epc len at tag %d", i)
		}

		epc := make([]byte, epcLen)
		copy(epc, frame.Data[cursor:cursor+epcLen])
		cursor += epcLen
		if cursor >= len(frame.Data) {
			return nil, fmt.Errorf("inventory missing rssi at tag %d", i)
		}
		rssi := int(frame.Data[cursor])
		cursor++

		tags = append(tags, InventoryG2Tag{
			Antenna: antenna,
			EPC:     epc,
			RSSI:    rssi,
		})
	}
	return tags, nil
}

// ParseSingleInventoryResult decodes single inventory payload.
// Expected payload layout: Ant(1), Count(1), EPCLen(1), EPC(n)
func ParseSingleInventoryResult(frame Frame) (SingleInventoryResult, error) {
	if frame.Command != CmdInventorySingle {
		return SingleInventoryResult{}, fmt.Errorf("not single-inventory frame")
	}
	if frame.Status != StatusNoTag && frame.Status != StatusSuccess {
		return SingleInventoryResult{}, fmt.Errorf("single-inventory status 0x%02X", frame.Status)
	}
	if len(frame.Data) < 3 {
		return SingleInventoryResult{}, fmt.Errorf("single-inventory payload too short")
	}

	ant := frame.Data[0]
	count := int(frame.Data[1])
	epcLen := int(frame.Data[2])
	if epcLen < 0 || len(frame.Data) < 3+epcLen {
		return SingleInventoryResult{}, fmt.Errorf("single-inventory invalid epc len")
	}

	epc := make([]byte, epcLen)
	copy(epc, frame.Data[3:3+epcLen])
	return SingleInventoryResult{
		Antenna:  ant,
		TagCount: count,
		EPC:      epc,
	}, nil
}

func antennaIDFromMask(mask byte) int {
	switch mask {
	case 1:
		return 1
	case 2:
		return 2
	case 4:
		return 3
	case 8:
		return 4
	case 16:
		return 5
	case 32:
		return 6
	case 64:
		return 7
	case 128:
		return 8
	default:
		return int(mask) + 1
	}
}

// crc-16-mcrf4xx (poly 0x8408, init 0xFFFF, refin/refout true).
func crc16MCRF4XX(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&0x0001 != 0 {
				crc = (crc >> 1) ^ 0x8408
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}
