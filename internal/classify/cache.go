package classify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type cached struct {
	delegate Querier
	filePath string

	entries map[string]cacheEntry
	dirty   bool
}

type cacheEntry struct {
	ISIN string `json:"isin"`
	Info
}

// Cached wraps a Querier with a JSON file cache at path ("" keeps the cache
// in memory only).
func Cached(file string, delegate Querier) Querier {
	return &cached{delegate: delegate, filePath: file}
}

func (c *cached) Name() string { return "cached(" + c.delegate.Name() + ")" }

func (c *cached) Query(ctx context.Context, isins []string) (map[string]Info, error) {
	if err := c.load(); err != nil {
		return nil, err
	}
	out := map[string]Info{}
	var pending []string
	seen := map[string]struct{}{}
	for _, isin := range isins {
		isin = strings.ToUpper(strings.TrimSpace(isin))
		if _, dup := seen[isin]; isin == "" || dup {
			continue
		}
		seen[isin] = struct{}{}
		if e, ok := c.entries[isin]; ok {
			if e.known() {
				out[isin] = e.Info
			}
			continue
		}
		pending = append(pending, isin)
	}
	if len(pending) == 0 {
		return out, nil
	}

	res, err := c.delegate.Query(ctx, pending)
	for _, isin := range pending {
		info, ok := res[isin]
		if ok && info.known() {
			out[isin] = info
			c.entries[isin] = cacheEntry{ISIN: isin, Info: info}
			c.dirty = true
		} else if err == nil {
			c.entries[isin] = cacheEntry{ISIN: isin} // remembered miss
			c.dirty = true
		}
	}
	if serr := c.save(); serr != nil && err == nil {
		err = serr
	}
	return out, err
}

func (c *cached) load() error {
	if c.entries != nil {
		return nil
	}
	c.entries = map[string]cacheEntry{}
	if c.filePath == "" {
		return nil
	}
	b, err := os.ReadFile(c.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var list []cacheEntry
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("%s: %w", c.filePath, err)
	}
	for _, e := range list {
		c.entries[e.ISIN] = e
	}
	return nil
}

func (c *cached) save() error {
	if !c.dirty || c.filePath == "" {
		return nil
	}
	list := make([]cacheEntry, 0, len(c.entries))
	for _, e := range c.entries {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ISIN < list[j].ISIN })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.filePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(c.filePath, b, 0o644); err != nil {
		return err
	}
	c.dirty = false
	return nil
}
