package sdk

import "time"

func (c *Client) emitTag(event TagEvent) {
	select {
	case c.tags <- event:
	default:
	}
}

func (c *Client) emitStatus(message string) {
	select {
	case c.statuses <- StatusEvent{When: time.Now(), Message: message}:
	default:
	}
}

func (c *Client) emitErr(err error) {
	if err == nil {
		return
	}
	select {
	case c.errs <- err:
	default:
	}
}
