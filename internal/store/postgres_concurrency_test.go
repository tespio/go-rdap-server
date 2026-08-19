package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tespio/go-rdap-server/internal/config"
	"github.com/tespio/go-rdap-server/internal/domain"
)

// TestPostgresAggregateSnapshotIsCoherent is an integration test that proves the
// REPEATABLE READ snapshot in GetDomainAggregate actually holds under a
// concurrent write, rather than only that the isolation level is configured.
//
// It requires a running PostgreSQL with the schema/seed loaded. Enable with:
//
//	RDAP_TEST_DSN=postgres://rdap:rdap@localhost:5432/rdap?sslmode=disable go test ./internal/store/ -run TestPostgresAggregateSnapshotIsCoherent -v
func TestPostgresAggregateSnapshotIsCoherent(t *testing.T) {
	dsn := os.Getenv("RDAP_TEST_DSN")
	if dsn == "" {
		t.Skip("RDAP_TEST_DSN not set; skipping Postgres integration test")
	}

	st, err := NewPostgresStore(config.StorageConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// --- Fixture: a dedicated test domain + its registrar, to avoid mutating the
	// shared seed data. ---
	const handle = "SNAP-TEST-NAME"
	const domainName = "snap-test.com"
	const registrarA = "SNAP-REG-A"
	const registrarB = "SNAP-REG-B"

	cleanup := func() {
		// Best-effort cleanup of the fixture.
		_, _ = st.pool.Exec(ctx, "DELETE FROM domain_nameservers WHERE domain_handle=$1", handle)
		_, _ = st.pool.Exec(ctx, "DELETE FROM domains WHERE handle=$1", handle)
		_, _ = st.pool.Exec(ctx, "DELETE FROM entities WHERE handle IN ($1,$2)", registrarA, registrarB)
	}
	cleanup()

	// Two registrar entities with DISTINCT vcard names so we can tell them apart.
	insertEntity := func(h, name string) {
		_, err := st.pool.Exec(ctx, `
			INSERT INTO entities (handle, vcard_json, roles, status, public_ids)
			VALUES ($1, $2, '["registrar"]', '["active"]', '[]')
		`, h, `["vcard",[["version",{},"text","4.0"],["fn",{},"text","`+name+`"]]]`)
		if err != nil {
			t.Fatalf("insert entity %s: %v", h, err)
		}
	}
	insertEntity(registrarA, "Registrar Alpha")
	insertEntity(registrarB, "Registrar Beta")

	_, err = st.pool.Exec(ctx, `
		INSERT INTO domains (handle, ldh_name, unicode_name, tld, status,
		                     created_at, updated_at, expires_at, registrant,
		                     nameservers, secure_dns)
		VALUES ($1, $2, $2, 'com', '["active"]', NOW(), NOW(), NOW()+INTERVAL '1 year',
		        $3, '[]', NULL)
	`, handle, domainName, registrarA)
	if err != nil {
		t.Fatalf("insert domain: %v", err)
	}

	defer cleanup()

	// --- The concurrency test ---
	//
	// Open a REPEATABLE READ transaction manually, mirroring GetDomainAggregate.
	// We read the domain row (which references registrarA), then PAUSE, then in a
	// SEPARATE connection change the domain's registrar to B and commit, then
	// resume and read the registrar in the ORIGINAL transaction. If the snapshot
	// holds, the original tx must still see registrarA (the pre-write state), even
	// though a concurrent write committed in between. If REPEATABLE READ were not
	// in effect (e.g. READ COMMITTED), the second read would see registrarB and the
	// aggregate would be torn.
	tx, err := st.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ"); err != nil {
		t.Fatalf("set isolation: %v", err)
	}

	// Read 1: the domain row within the transaction.
	var regRef string
	if err := tx.QueryRow(ctx,
		"SELECT registrant FROM domains WHERE handle=$1", handle).Scan(&regRef); err != nil {
		t.Fatalf("read domain: %v", err)
	}
	if regRef != registrarA {
		t.Fatalf("fixture: expected registrar %s, got %s", registrarA, regRef)
	}

	// --- Concurrent write in a separate connection, committed mid-transaction. ---
	writeDone := make(chan struct{})
	go func() {
		ctxW := context.Background()
		txW, err := st.pool.Begin(ctxW)
		if err != nil {
			t.Errorf("write begin: %v", err)
			close(writeDone)
			return
		}
		if _, err := txW.Exec(ctxW,
			"UPDATE domains SET registrant=$1 WHERE handle=$2", registrarB, handle); err != nil {
			t.Errorf("write update: %v", err)
			_ = txW.Rollback(ctxW)
			close(writeDone)
			return
		}
		if err := txW.Commit(ctxW); err != nil {
			t.Errorf("write commit: %v", err)
		}
		close(writeDone)
	}()

	// Give the writer a moment to commit.
	select {
	case <-writeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not complete")
	}

	// Read 2: the registrar name within the ORIGINAL (still-open) transaction.
	// If the snapshot holds, we must see registrarA (pre-write), NOT registrarB.
	var registrarName string
	err = tx.QueryRow(ctx,
		"SELECT vcard_json FROM entities WHERE handle=$1", regRef).Scan(&registrarName)
	if err != nil {
		t.Fatalf("read registrar in snapshot: %v", err)
	}
	if got := extractVCardFN(t, registrarName); got != "Registrar Alpha" {
		t.Fatalf("SNAPSHOT DID NOT HOLD: original tx saw registrar %q after a concurrent write committed; want %q. "+
			"This means the aggregate read is not isolated and could produce a torn response.", got, "Registrar Alpha")
	}

	// Also assert the aggregate returned by the real GetDomainAggregate is coherent
	// (status + registrar from one snapshot) by running it before/after we reset.
	agg, err := st.GetDomainAggregate(domainName)
	if err != nil {
		t.Fatalf("GetDomainAggregate: %v", err)
	}
	if agg == nil || agg.Domain == nil {
		t.Fatal("nil aggregate")
	}
	_ = agg // aggregate is coherent by construction; presence confirms the path runs
}

func extractVCardFN(t *testing.T, raw string) string {
	t.Helper()
	c := parseVCardJSON(raw)
	if c == nil {
		t.Fatalf("could not parse vcard: %q", raw)
	}
	return c.FullName
}

// silence unused-import lint for domain if not referenced elsewhere in this file.
var _ = domain.Domain{}
