package sdk

import (
	"context"
	"fmt"
	"time"

	"new_era_go/internal/discovery"
)

func DefaultScanOptions() ScanOptions {
	opts := discovery.DefaultOptions()
	return ScanOptions{
		Ports:                 append([]int{}, opts.Ports...),
		Timeout:               opts.Timeout,
		Concurrency:           opts.Concurrency,
		HostLimitPerInterface: opts.HostLimitPerInterface,
	}
}

// Discover scans LAN for probable reader endpoints.
func (c *Client) Discover(ctx context.Context, opts ScanOptions) ([]Candidate, error) {
	internalOpts := toInternalScanOptions(opts)
	internalCandidates, err := discovery.Scan(ctx, internalOpts)
	candidates := make([]Candidate, 0, len(internalCandidates))
	for _, candidate := range internalCandidates {
		candidates = append(candidates, fromInternalCandidate(candidate))
	}
	return candidates, err
}

// QuickConnect scans and connects to best candidate (verified-first).
func (c *Client) QuickConnect(ctx context.Context, opts ScanOptions) (Candidate, error) {
	candidates, err := c.Discover(ctx, opts)
	if err != nil && len(candidates) == 0 {
		return Candidate{}, err
	}
	if len(candidates) == 0 {
		return Candidate{}, fmt.Errorf("no reader candidate found")
	}

	index := 0
	for i, candidate := range candidates {
		if candidate.Verified {
			index = i
			break
		}
	}
	chosen := candidates[index]

	timeout := 3 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	if err := c.Reconnect(ctx, Endpoint{Host: chosen.Host, Port: chosen.Port}, timeout); err != nil {
		return chosen, err
	}

	if chosen.Verified {
		c.mu.Lock()
		c.readerAddr = chosen.ReaderAddress
		c.mu.Unlock()
	}
	c.emitStatus(fmt.Sprintf("connected to %s:%d", chosen.Host, chosen.Port))
	return chosen, nil
}

func toInternalScanOptions(opts ScanOptions) discovery.ScanOptions {
	defaults := discovery.DefaultOptions()
	if len(opts.Ports) == 0 {
		opts.Ports = defaults.Ports
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaults.Timeout
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaults.Concurrency
	}
	if opts.HostLimitPerInterface <= 0 {
		opts.HostLimitPerInterface = defaults.HostLimitPerInterface
	}
	return discovery.ScanOptions{
		Ports:                 append([]int{}, opts.Ports...),
		Timeout:               opts.Timeout,
		Concurrency:           opts.Concurrency,
		HostLimitPerInterface: opts.HostLimitPerInterface,
	}
}

func fromInternalCandidate(candidate discovery.Candidate) Candidate {
	return Candidate{
		Host:          candidate.Host,
		Port:          candidate.Port,
		Score:         candidate.Score,
		Banner:        candidate.Banner,
		Reason:        candidate.Reason,
		Verified:      candidate.Verified,
		ReaderAddress: candidate.ReaderAddress,
		Protocol:      candidate.Protocol,
	}
}
