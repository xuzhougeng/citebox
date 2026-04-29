package research

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// BasketRow is the storage view of a basket item.
type BasketRow struct {
	S2PaperID string
	Notes     string
	AddedAt   time.Time
}

// BasketRepo is the storage backend for the basket.
type BasketRepo interface {
	AddBasketItem(s2PaperID, notes string) error
	RemoveBasketItem(s2PaperID string) error
	ListBasketItems() ([]BasketRow, error)
}

// PaperLookup hydrates an item to a full Paper via the cache/service.
type PaperLookup interface {
	GetPaper(ctx context.Context, id string) (Paper, error)
}

// LibraryImporter persists a Paper into the local library and returns the new
// internal record id. Implemented by the library service.
type LibraryImporter interface {
	ImportPaperFromS2WithID(ctx context.Context, p Paper) (importedID int64, err error)
}

// Basket bundles storage + lookup + (optional) library importer.
type Basket struct {
	repo     BasketRepo
	lookup   PaperLookup
	importer LibraryImporter
}

// NewBasket constructs a Basket. importer may be nil if Import is unused (tests).
func NewBasket(repo BasketRepo, lookup PaperLookup, importer LibraryImporter) *Basket {
	return &Basket{repo: repo, lookup: lookup, importer: importer}
}

// Add inserts/updates a basket entry.
func (b *Basket) Add(ctx context.Context, s2PaperID, notes string) error {
	if strings.TrimSpace(s2PaperID) == "" {
		return fmt.Errorf("research: s2_paper_id required")
	}
	return b.repo.AddBasketItem(s2PaperID, notes)
}

// Remove deletes a basket entry by id.
func (b *Basket) Remove(ctx context.Context, s2PaperID string) error {
	return b.repo.RemoveBasketItem(s2PaperID)
}

// List returns all basket items hydrated with Paper metadata. Items whose
// metadata can't be fetched are skipped (logged at the caller level if needed).
func (b *Basket) List(ctx context.Context) ([]Paper, error) {
	rows, err := b.repo.ListBasketItems()
	if err != nil {
		return nil, err
	}
	out := make([]Paper, 0, len(rows))
	for _, row := range rows {
		paper, err := b.lookup.GetPaper(ctx, row.S2PaperID)
		if err != nil {
			continue
		}
		out = append(out, paper)
	}
	return out, nil
}

// ImportToLibrary creates papers in the library for the requested ids and
// removes successful ids from the basket. Returns the number imported.
func (b *Basket) ImportToLibrary(ctx context.Context, ids []string) (int, error) {
	if b.importer == nil {
		return 0, fmt.Errorf("research: importer not configured")
	}
	imported := 0
	for _, id := range ids {
		paper, err := b.lookup.GetPaper(ctx, id)
		if err != nil {
			continue
		}
		if _, err := b.importer.ImportPaperFromS2WithID(ctx, paper); err != nil {
			continue
		}
		_ = b.repo.RemoveBasketItem(id)
		imported++
	}
	return imported, nil
}

// ExportMarkdown formats basket contents as a markdown bullet list.
func (b *Basket) ExportMarkdown(ctx context.Context) (string, error) {
	papers, err := b.List(ctx)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	sb.WriteString("# 调研篮\n\n")
	for _, p := range papers {
		authors := make([]string, 0, len(p.Authors))
		for _, a := range p.Authors {
			authors = append(authors, a.Name)
		}
		fmt.Fprintf(&sb, "- **%s** — %s (%d)", p.Title, strings.Join(authors, ", "), p.Year)
		if p.ExternalIDs.DOI != "" {
			fmt.Fprintf(&sb, " · DOI: %s", p.ExternalIDs.DOI)
		}
		if p.TLDR != "" {
			fmt.Fprintf(&sb, "\n  > %s", p.TLDR)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
