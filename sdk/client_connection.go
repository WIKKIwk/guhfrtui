package sdk

import (
	"context"
	"fmt"
	"time"

	reader18 "new_era_go/internal/protocol/reader18"
	"new_era_go/internal/reader"
)

func (c *Client) Connect(ctx context.Context, endpoint Endpoint, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	internalEndpoint := reader.Endpoint{Host: endpoint.Host, Port: endpoint.Port}
	if err := c.transport.Connect(ctx, internalEndpoint, timeout); err != nil {
		return err
	}
	c.emitStatus("connected: " + endpoint.Address())
	return nil
}

func (c *Client) Reconnect(ctx context.Context, endpoint Endpoint, timeout time.Duration) error {
	_ = c.StopInventory()
	_ = c.transport.Disconnect()
	return c.Connect(ctx, endpoint, timeout)
}

func (c *Client) Disconnect() error {
	_ = c.StopInventory()
	err := c.transport.Disconnect()
	if err == nil {
		c.emitStatus("disconnected")
	}
	return err
}

func (c *Client) Close() error {
	return c.Disconnect()
}

// ProbeInfo sends GetReaderInfo command.
func (c *Client) ProbeInfo() error {
	if !c.transport.IsConnected() {
		return fmt.Errorf("not connected")
	}
	addr := c.currentReaderAddress()
	return c.transport.SendRaw(reader18.GetReaderInfoCommand(addr), 2*time.Second)
}

// ApplyInventoryConfig sends inventory-related configuration commands to reader.
