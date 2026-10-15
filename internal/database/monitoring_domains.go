package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"surge-controller/internal/dnsserver"
)

type domainCheckpoint struct {
	Session   time.Time `json:"session"`
	Sequence  uint64    `json:"sequence"`
	StartedAt time.Time `json:"started_at"`
}

func saveMonitoringDomains(ctx context.Context, tx *sql.Tx, batch *dnsserver.DomainBatch) error {
	if batch == nil {
		return nil
	}
	var old domainCheckpoint
	var raw []byte
	err := tx.QueryRowContext(ctx, "SELECT document FROM statistics WHERE id='dns_domain_checkpoint'").Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if err = json.Unmarshal(raw, &old); err != nil {
			return fmt.Errorf("decode domain checkpoint: %w", err)
		}
		if old.Session.Equal(batch.Session) {
			if batch.Sequence == old.Sequence {
				return nil
			}
			if batch.Sequence != old.Sequence+1 {
				return errors.New("domain checkpoint sequence is not consecutive")
			}
		}
	}
	if batch.Session.IsZero() || batch.StartedAt.IsZero() || batch.ObservedAt.IsZero() || batch.Sequence == 0 {
		return errors.New("invalid domain checkpoint")
	}
	started := old.StartedAt
	if started.IsZero() {
		started = batch.StartedAt
	}
	for _, table := range []string{"dns_domain_counts", "dns_domain_history"} {
		query := "INSERT INTO " + table + "(source,domain,other,queries) VALUES(?,?,?,?) ON CONFLICT(source,domain,other) DO UPDATE SET queries=queries+excluded.queries"
		if table == "dns_domain_history" {
			query = "INSERT INTO dns_domain_history(bucket,source,domain,other,queries) VALUES(?,?,?,?,?) ON CONFLICT(bucket,source,domain,other) DO UPDATE SET queries=queries+excluded.queries"
		}
		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return err
		}
		for _, row := range batch.Rows {
			args := []any{row.Source, row.Domain, row.Other, row.Queries}
			if table == "dns_domain_history" {
				args = append([]any{batch.ObservedAt.UTC().Truncate(time.Minute).Unix()}, args...)
			}
			if _, err = stmt.ExecContext(ctx, args...); err != nil {
				stmt.Close()
				return err
			}
		}
		if err = stmt.Close(); err != nil {
			return err
		}
	}
	raw, err = json.Marshal(domainCheckpoint{batch.Session, batch.Sequence, started})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO statistics(id,document) VALUES('dns_domain_checkpoint',?) ON CONFLICT(id) DO UPDATE SET document=excluded.document", raw)
	return err
}

// SQL retains exact identities on disk; only the top 100 and their remainder
// cross the API. Buffered increments are merged before sorting, so a newly hot
// domain can enter the ranking after the old combination budget is exhausted.
func (s *Store) DNSMonitoringDomains(ctx context.Context, from, to time.Time, source string, buffer *dnsserver.DomainBatch) ([]dnsserver.DNSDomainCount, *dnsserver.DNSDomainStatistics, error) {
	var checkpoint domainCheckpoint
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT document FROM statistics WHERE id='dns_domain_checkpoint'").Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}
	if err == nil {
		if err = json.Unmarshal(raw, &checkpoint); err != nil {
			return nil, nil, fmt.Errorf("decode domain checkpoint: %w", err)
		}
	}
	started := checkpoint.StartedAt
	pending := []dnsserver.DomainCount{}
	if buffer != nil {
		if buffer.Rows != nil {
			pending = buffer.Rows
		}
		if started.IsZero() {
			started = buffer.StartedAt
		}
	}
	if started.IsZero() || !to.IsZero() && !to.After(started) {
		return nil, nil, nil
	}
	encoded, err := json.Marshal(pending)
	if err != nil {
		return nil, nil, err
	}
	query := `WITH counts AS (
 SELECT domain,other,queries FROM dns_domain_counts WHERE (?='' OR source=?)
 UNION ALL
 SELECT json_extract(value,'$.domain'),COALESCE(json_extract(value,'$.other'),0),json_extract(value,'$.queries') FROM json_each(?) WHERE (?='' OR json_extract(value,'$.source')=?)
 ), aggregated AS (SELECT domain,other,SUM(queries) AS queries FROM counts GROUP BY domain,other)
 SELECT domain,other,queries,SUM(queries) OVER() FROM aggregated ORDER BY other DESC,queries DESC,domain LIMIT 101`
	args := []any{source, source, string(encoded), source, source}
	if !from.IsZero() {
		query = `WITH aggregated AS (SELECT domain,other,SUM(queries) AS queries FROM dns_domain_history WHERE bucket>=? AND bucket<? AND (?='' OR source=?) GROUP BY domain,other)
  SELECT domain,other,queries,SUM(queries) OVER() FROM aggregated ORDER BY other DESC,queries DESC,domain LIMIT 101`
		args = []any{from.Unix(), to.Unix(), source, source}
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	result := []dnsserver.DNSDomainCount{}
	stats := &dnsserver.DNSDomainStatistics{StartedAt: started}
	var shown uint64
	var unknown *dnsserver.DNSDomainCount
	for rows.Next() {
		var row dnsserver.DNSDomainCount
		if err = rows.Scan(&row.Domain, &row.Other, &row.Queries, &stats.Completed); err != nil {
			return nil, nil, err
		}
		if row.Other {
			unknown = &row
			stats.Unrecorded += row.Queries
			continue
		}
		if len(result) < 100 {
			result = append(result, row)
			shown += row.Queries
		}
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	if rest := stats.Completed - shown - stats.Unrecorded; rest > 0 {
		result = append(result, dnsserver.DNSDomainCount{Rest: true, Queries: rest})
	}
	if unknown != nil {
		result = append(result, *unknown)
	}
	return result, stats, nil
}
