// Package dnsimple provides a thin wrapper around the DNSimple API
// for fetching and managing DNS records.
package dnsimple

import (
	"context"
	"fmt"

	"github.com/ags4no/dnsync/internal/config"
	"github.com/ags4no/dnsync/internal/diff"
	"github.com/dnsimple/dnsimple-go/dnsimple"
)

// Client wraps the DNSimple API client.
type Client struct {
	client    *dnsimple.Client
	accountID string
}

// NewClient creates a new DNSimple client with the given API token and account ID.
func NewClient(token, accountID string) *Client {
	ts := dnsimple.StaticTokenHTTPClient(context.Background(), token)
	client := dnsimple.NewClient(ts)
	return &Client{
		client:    client,
		accountID: accountID,
	}
}

// FetchRecords retrieves all DNS records for a zone.
func (c *Client) FetchRecords(ctx context.Context, zone string) ([]diff.LiveRecord, error) {
	var allRecords []diff.LiveRecord
	page := 1

	for {
		resp, err := c.client.Zones.ListRecords(ctx, c.accountID, zone, &dnsimple.ZoneRecordListOptions{
			ListOptions: dnsimple.ListOptions{Page: &page},
		})
		if err != nil {
			return nil, fmt.Errorf("fetching records for zone %s: %w", zone, err)
		}

		for _, r := range resp.Data {
			allRecords = append(allRecords, diff.LiveRecord{
				ID:       r.ID,
				Name:     r.Name,
				Type:     r.Type,
				Content:  r.Content,
				TTL:      r.TTL,
				Priority: r.Priority,
			})
		}

		if resp.Pagination.CurrentPage >= resp.Pagination.TotalPages {
			break
		}
		page++
	}

	return allRecords, nil
}

// ApplyChanges executes a set of DNS changes against the DNSimple API.
func (c *Client) ApplyChanges(ctx context.Context, changes []diff.Change) error {
	for _, ch := range changes {
		switch ch.Action {
		case diff.ActionCreate:
			err := c.createRecord(ctx, ch.Zone, ch.Record)
			if err != nil {
				return fmt.Errorf("creating record %s %s in %s: %w",
					ch.Record.NormalizedName(), ch.Record.Type, ch.Zone, err)
			}
		case diff.ActionUpdate:
			err := c.updateRecord(ctx, ch.Zone, ch.LiveID, ch.Record)
			if err != nil {
				return fmt.Errorf("updating record %s %s in %s: %w",
					ch.Record.NormalizedName(), ch.Record.Type, ch.Zone, err)
			}
		case diff.ActionDelete:
			err := c.deleteRecord(ctx, ch.Zone, ch.LiveID)
			if err != nil {
				return fmt.Errorf("deleting record ID %d in %s: %w",
					ch.LiveID, ch.Zone, err)
			}
		}
	}
	return nil
}

func (c *Client) createRecord(ctx context.Context, zone string, r config.Record) error {
	attrs := dnsimple.ZoneRecordAttributes{
		Name:    dnsimple.String(r.NormalizedName()),
		Type:    r.Type,
		Content: r.Content,
		TTL:     r.TTL,
	}
	if r.Priority > 0 {
		attrs.Priority = r.Priority
	}
	_, err := c.client.Zones.CreateRecord(ctx, c.accountID, zone, attrs)
	return err
}

func (c *Client) updateRecord(ctx context.Context, zone string, recordID int64, r config.Record) error {
	attrs := dnsimple.ZoneRecordAttributes{
		Name:    dnsimple.String(r.NormalizedName()),
		Type:    r.Type,
		Content: r.Content,
		TTL:     r.TTL,
	}
	if r.Priority > 0 {
		attrs.Priority = r.Priority
	}
	_, err := c.client.Zones.UpdateRecord(ctx, c.accountID, zone, recordID, attrs)
	return err
}

func (c *Client) deleteRecord(ctx context.Context, zone string, recordID int64) error {
	_, err := c.client.Zones.DeleteRecord(ctx, c.accountID, zone, recordID)
	return err
}
