package tui

import (
	"fmt"
	"strings"

	reader18 "new_era_go/internal/protocol/reader18"
)

func (m *Model) handleProtocolFrame(frame reader18.Frame) {
	m.lastRX = formatHex(frame.Raw, 52)

	switch frame.Command {
	case reader18.CmdInventory:
		m.awaitingProbe = false
		if m.inventoryAutoAddr {
			m.inventoryAddress = frame.Address
			m.inventoryAutoAddr = false
			m.pushLog(fmt.Sprintf("inventory address detected: 0x%02X", frame.Address))
		}
		tags, err := reader18.ParseInventoryG2Tags(frame)
		if err != nil {
			if !m.inventoryRunning {
				m.pushLog("inventory parse error: " + err.Error())
			}
			return
		}
		if len(tags) > 0 {
			if m.seenTagEPC == nil {
				m.seenTagEPC = make(map[string]struct{})
			}
			m.inventoryNoTagHit = 0
			newCount := 0
			for _, tag := range tags {
				epcText := strings.ReplaceAll(formatHex(tag.EPC, 96), " ", "")
				if epcText == "" {
					continue
				}
				m.lastTagEPC = epcText
				m.lastTagAntenna = tag.Antenna
				m.lastTagRSSI = tag.RSSI
				if _, exists := m.seenTagEPC[epcText]; exists {
					continue
				}
				m.seenTagEPC[epcText] = struct{}{}
				m.inventoryTagTotal++
				newCount++
				getBotSyncClient().onNewEPC(epcText)
				m.pushLog(fmt.Sprintf("new tag ant=%d epc=%s rssi=%d total=%d", tag.Antenna, epcText, tag.RSSI, m.inventoryTagTotal))
			}
			if newCount > 0 {
				if m.activeScreen == screenControl {
					m.status = fmt.Sprintf("New tag(s): +%d total=%d", newCount, m.inventoryTagTotal)
				}
			} else if m.inventoryRunning && m.inventoryRounds%12 == 0 && m.lastTagEPC != "" {
				if m.activeScreen == screenControl {
					m.status = fmt.Sprintf("Tag seen again: %s", trimText(m.lastTagEPC, 28))
				}
			}
			return
		}
		if frame.Status == reader18.StatusSuccess {
			if count, err := reader18.InventoryTagCount(frame); err == nil && count > 0 {
				if m.activeScreen == screenControl && m.inventoryRounds%6 == 0 {
					m.status = fmt.Sprintf("Reader reports %d tag(s), waiting EPC frame...", count)
				}
				if m.inventoryRounds%30 == 0 {
					m.pushLog(fmt.Sprintf("inventory count-only response: %d tag(s)", count))
				}
			}
			return
		}

		switch frame.Status {
		case reader18.StatusNoTag, 0x02, 0x03, 0x04, reader18.StatusNoTagOrTimeout:
			m.onNoTagObserved()
			if m.activeScreen == screenControl && m.inventoryRunning && m.inventoryRounds%24 == 0 {
				m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusAntennaError:
			if m.inventoryRunning && m.inventoryRounds%20 == 0 {
				m.status = fmt.Sprintf("Reading... antenna check (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusCmdError:
			if !m.inventoryRunning {
				m.pushLog("inventory status: illegal command (0xFE)")
			}
		case reader18.StatusCRCError:
			if !m.inventoryRunning {
				m.pushLog("inventory status: parameter error (0xFF)")
			}
		default:
			if !m.inventoryRunning {
				m.pushLog(fmt.Sprintf("inventory status: 0x%02X", frame.Status))
			}
		}

	case reader18.CmdInventorySingle:
		m.awaitingProbe = false
		if m.inventoryAutoAddr {
			m.inventoryAddress = frame.Address
			m.inventoryAutoAddr = false
			m.pushLog(fmt.Sprintf("inventory address detected: 0x%02X", frame.Address))
		}

		switch frame.Status {
		case reader18.StatusNoTag:
			result, err := reader18.ParseSingleInventoryResult(frame)
			if err != nil {
				if !m.inventoryRunning {
					m.pushLog("single inventory parse error: " + err.Error())
				}
				return
			}
			if result.TagCount > 0 {
				epcText := strings.ReplaceAll(formatHex(result.EPC, 96), " ", "")
				m.lastTagEPC = epcText
				m.lastTagAntenna = int(result.Antenna)
				m.lastTagRSSI = 0
				m.inventoryNoTagHit = 0
				if m.seenTagEPC == nil {
					m.seenTagEPC = make(map[string]struct{})
				}
				if _, exists := m.seenTagEPC[epcText]; !exists {
					m.seenTagEPC[epcText] = struct{}{}
					m.inventoryTagTotal++
					getBotSyncClient().onNewEPC(epcText)
					if m.activeScreen == screenControl {
						m.status = fmt.Sprintf("New tag: ant=%d epc=%s", result.Antenna, trimText(epcText, 28))
					}
					m.pushLog(fmt.Sprintf("new tag ant=%d epc=%s total=%d", result.Antenna, epcText, m.inventoryTagTotal))
				} else if m.activeScreen == screenControl && m.inventoryRunning && m.inventoryRounds%24 == 0 {
					m.status = fmt.Sprintf("Tag seen again: %s", trimText(epcText, 28))
				}
			} else {
				m.onNoTagObserved()
				if m.activeScreen == screenControl && m.inventoryRunning && m.inventoryRounds%24 == 0 {
					m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
				}
			}
		case reader18.StatusNoTagOrTimeout:
			m.onNoTagObserved()
			if m.activeScreen == screenControl && m.inventoryRunning && m.inventoryRounds%24 == 0 {
				m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusAntennaError:
			if m.inventoryRunning && m.inventoryRounds%20 == 0 {
				m.status = fmt.Sprintf("Reading... antenna check (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusCmdError:
			if !m.inventoryRunning {
				m.pushLog("single inventory status: command error (0xFE)")
			}
		case reader18.StatusCRCError:
			if !m.inventoryRunning {
				m.pushLog("single inventory status: parameter error (0xFF)")
			}
		default:
			if !m.inventoryRunning {
				m.pushLog(fmt.Sprintf("single inventory status: 0x%02X", frame.Status))
			}
		}

	case reader18.CmdGetReaderInfo:
		m.awaitingProbe = false
		if frame.Status == reader18.StatusSuccess {
			m.status = "Reader info received"
			m.pushLog("reader info: " + formatHex(frame.Data, 48))
		} else {
			m.pushLog(fmt.Sprintf("reader info status: 0x%02X", frame.Status))
		}

	default:
		if !m.inventoryRunning {
			m.pushLog(fmt.Sprintf("rx cmd=0x%02X status=0x%02X", frame.Command, frame.Status))
		}
	}
}

func (m *Model) onNoTagObserved() {
	if !m.inventoryRunning {
		return
	}
	m.inventoryNoTagHit++
	if m.inventorySession <= 1 || m.inventoryNoTagAB <= 0 {
		return
	}
	if m.inventoryNoTagHit >= m.inventoryNoTagAB {
		m.inventoryTarget ^= 0x01
		m.inventoryNoTagHit = 0
		m.pushLog("no-tag threshold reached, target switched to " + targetLabel(m.inventoryTarget))
	}
}
