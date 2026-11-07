package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
	"strings"
)

type directoryCursor struct {
	Identity string `json:"identity"`
	Revision string `json:"revision"`
	Last     []byte `json:"last"`
}

func pageDirectory(ctx context.Context, entries []fs.DirEntry, identity, cursor string, limit int, makeEntry func(fs.DirEntry) Entry) (Listing, error) {
	var prior directoryCursor
	if cursor != "" {
		if len(cursor) > 64<<10 || !strings.HasPrefix(cursor, "dir:") {
			return Listing{}, errors.New("staleRevision: invalid directory cursor; restart listing")
		}
		encoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(cursor, "dir:"))
		if err != nil || json.Unmarshal(encoded, &prior) != nil || prior.Identity != identity || prior.Revision == "" || len(prior.Last) == 0 {
			return Listing{}, errors.New("staleRevision: directory cursor belongs to another target; restart listing")
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	page := Listing{Entries: make([]Entry, 0, min(limit, len(entries))), Complete: true}
	digest := sha256.New()
	found := cursor == ""
	var last []byte
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return Listing{}, err
		}
		item := makeEntry(entry)
		encoded, err := json.Marshal(item)
		if err != nil {
			return Listing{}, err
		}
		_, _ = digest.Write(encoded)
		_, _ = digest.Write([]byte{0})
		if !found {
			if bytes.Equal([]byte(entry.Name()), prior.Last) {
				found = true
			}
			continue
		}
		if len(page.Entries) >= limit {
			page.Complete = false
			continue
		}
		page.Entries = append(page.Entries, item)
		last = []byte(entry.Name())
	}
	page.Revision = hex.EncodeToString(digest.Sum(nil))
	if cursor != "" && (!found || prior.Revision != page.Revision) {
		return Listing{}, errors.New("staleRevision: directory changed or cursor entry disappeared; restart listing")
	}
	if !page.Complete {
		encoded, err := json.Marshal(directoryCursor{Identity: identity, Revision: page.Revision, Last: last})
		if err != nil {
			return Listing{}, err
		}
		page.NextCursor = "dir:" + base64.RawURLEncoding.EncodeToString(encoded)
	}
	return page, nil
}
