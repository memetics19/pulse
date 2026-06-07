package web

import (
	"context"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestResolvePage(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	ctx := context.Background()

	// unknown host -> default page, all groups (nil), serve=true
	rp, serve := ResolvePage(ctx, q, "whatever.example:8090")
	if !serve || rp.Page.IsDefault != 1 || rp.GroupIDs != nil {
		t.Fatalf("default fallback wrong: serve=%v isDefault=%d groups=%v", serve, rp.Page.IsDefault, rp.GroupIDs)
	}

	// published custom page with group 5
	p, _ := q.CreateStatusPage(ctx, generated.CreateStatusPageParams{Domain: "acme.com", Title: "Acme", Published: 1})
	_ = q.AddPageGroup(ctx, generated.AddPageGroupParams{StatusPageID: p.ID, GroupID: 5})
	rp, serve = ResolvePage(ctx, q, "acme.com")
	if !serve || rp.Page.ID != p.ID || len(rp.GroupIDs) != 1 || rp.GroupIDs[0] != 5 {
		t.Fatalf("custom page wrong: serve=%v page=%d groups=%v", serve, rp.Page.ID, rp.GroupIDs)
	}

	// unpublished custom page -> serve=false
	up, _ := q.CreateStatusPage(ctx, generated.CreateStatusPageParams{Domain: "hidden.com", Title: "H", Published: 0})
	_ = up
	_, serve = ResolvePage(ctx, q, "hidden.com")
	if serve {
		t.Fatal("unpublished page must not serve")
	}
}
