package web

import (
	"context"
	"net"

	"github.com/memetics19/pulse/api/internal/generated"
)

// ResolvedPage is the status page selected for a request, plus the monitor
// group IDs it shows (nil means "all groups" — the default page).
type ResolvedPage struct {
	Page     generated.StatusPage
	GroupIDs []int64
}

// ResolvePage maps a request Host to a status page. Unknown hosts fall back to
// the default page (which shows all groups). The bool is false when the
// resolved non-default page is unpublished (caller should 404).
func ResolvePage(ctx context.Context, q *generated.Queries, host string) (ResolvedPage, bool) {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	page, err := q.GetPageByDomain(ctx, host)
	if err != nil || page.IsDefault == 1 {
		def, derr := q.GetDefaultPage(ctx)
		if derr != nil {
			return ResolvedPage{}, false
		}
		return ResolvedPage{Page: def, GroupIDs: nil}, true
	}
	if page.Published != 1 {
		return ResolvedPage{Page: page}, false
	}
	ids, _ := q.ListPageGroupIDs(ctx, page.ID)
	return ResolvedPage{Page: page, GroupIDs: ids}, true
}
