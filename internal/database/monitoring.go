package database

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"surge-controller/internal/dnsserver"
)

const MonitoringRetentionDays = 7
const MonitoringResolutionSeconds = 60

func encodeMonitoringBucket(bucket dnsserver.MonitoringBucket) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if err := json.NewEncoder(w).Encode(bucket); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeMonitoringBucket(raw []byte) (dnsserver.MonitoringBucket, error) {
	var bucket dnsserver.MonitoringBucket
	r, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return bucket, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, 16<<20))
	if err != nil {
		return bucket, err
	}
	if len(data) >= 16<<20 {
		return bucket, errors.New("monitoring bucket exceeds size limit")
	}
	return bucket, json.Unmarshal(data, &bucket)
}

// SaveDNSMonitoring commits the cumulative checkpoint and its history delta
// together. A retry or restart cannot count a persisted interval twice.
func (s *Store) SaveDNSMonitoring(ctx context.Context, snapshot dnsserver.DNSMonitoringSnapshot) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previous dnsserver.DNSMonitoringSnapshot
	var raw []byte
	err = tx.QueryRowContext(ctx, "SELECT document FROM statistics WHERE id='dns_monitoring_history_checkpoint'").Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	havePrevious := err == nil
	if havePrevious {
		if err = json.Unmarshal(raw, &previous); err != nil {
			return fmt.Errorf("decode monitoring checkpoint: %w", err)
		}
	}
	if havePrevious {
		delta, e := dnsserver.MonitoringDifference(snapshot, previous)
		if e != nil {
			return e
		}
		elapsed := snapshot.ObservedAt.Sub(previous.ObservedAt)
		if snapshot.SessionStartedAt == previous.SessionStartedAt && elapsed <= 0 {
			if delta.Received != 0 || delta.Completed != 0 || delta.Unconfirmed != 0 {
				return errors.New("monitoring timestamp did not advance")
			}
			return nil
		}
		from := previous.ObservedAt
		valid := snapshot.SessionStartedAt == previous.SessionStartedAt && elapsed > 0 && elapsed <= 15*time.Second
		seconds := elapsed.Seconds()
		if snapshot.SessionStartedAt != previous.SessionStartedAt {
			from = snapshot.ObservedAt
			seconds = 0
		}
		interval := dnsserver.MonitoringBucket{From: from, To: snapshot.ObservedAt, Seconds: seconds, RateValid: valid, Counts: delta}
		bucketKey := snapshot.ObservedAt.UTC().Truncate(time.Minute).Unix()
		var bucket dnsserver.MonitoringBucket
		err = tx.QueryRowContext(ctx, "SELECT document FROM dns_monitoring_history WHERE bucket=?", bucketKey).Scan(&raw)
		if err == nil {
			bucket, err = decodeMonitoringBucket(raw)
			if err != nil {
				return fmt.Errorf("decode monitoring history: %w", err)
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		dnsserver.MergeMonitoringBucket(&bucket, interval)
		raw, err = encodeMonitoringBucket(bucket)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO dns_monitoring_history(bucket,document) VALUES(?,?) ON CONFLICT(bucket) DO UPDATE SET document=excluded.document", bucketKey, raw); err != nil {
			return err
		}
	}
	raw, err = json.Marshal(snapshot)
	if err != nil {
		return err
	}
	for _, id := range []string{"dns_monitoring", "dns_monitoring_history_checkpoint"} {
		if _, err = tx.ExecContext(ctx, "INSERT INTO statistics(id,document) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET document=excluded.document", id, raw); err != nil {
			return err
		}
	}
	cutoff := snapshot.ObservedAt.Add(-MonitoringRetentionDays * 24 * time.Hour).UTC().Truncate(time.Minute).Unix()
	if _, err = tx.ExecContext(ctx, "DELETE FROM dns_monitoring_history WHERE bucket<?", cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DNSMonitoringHistory(ctx context.Context, from, to time.Time, source string) (dnsserver.MonitoringHistory, error) {
	result := dnsserver.MonitoringHistory{From: from, To: to, RetentionDays: MonitoringRetentionDays, ResolutionSeconds: MonitoringResolutionSeconds, Points: []dnsserver.MonitoringBucket{}, Summary: dnsserver.EmptyMonitoringCounts()}
	if !from.Equal(from.Truncate(time.Minute)) || !to.Equal(to.Truncate(time.Minute)) || !to.After(from) || to.Sub(from) > MonitoringRetentionDays*24*time.Hour {
		return result, errors.New("monitoring history range must use whole minutes and span at most 7 days")
	}
	minutes := int64(to.Sub(from) / time.Minute)
	// At most 720 trend points; counts and shares always aggregate every bucket.
	for _, step := range []int64{1, 5, 15, 30, 60} {
		if (minutes+step-1)/step <= 720 {
			result.ResolutionSeconds = step * 60
			break
		}
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT bucket,document FROM dns_monitoring_history WHERE bucket>=? AND bucket<? ORDER BY bucket", from.Unix(), to.Unix())
	if err != nil {
		return result, err
	}
	defer rows.Close()
	var lastGroup int64 = -1
	for rows.Next() {
		var key int64
		var raw []byte
		if err = rows.Scan(&key, &raw); err != nil {
			return result, err
		}
		bucket, e := decodeMonitoringBucket(raw)
		if e != nil {
			return result, fmt.Errorf("decode monitoring history: %w", e)
		}
		group := (key - from.Unix()) / result.ResolutionSeconds
		if group != lastGroup {
			result.Points = append(result.Points, dnsserver.MonitoringBucket{})
			lastGroup = group
		}
		dnsserver.AddMonitoringCounts(&result.Summary, bucket.Counts)
		if source != "" {
			counts := dnsserver.EmptyMonitoringCounts()
			for _, src := range bucket.Counts.Sources {
				if src.Source == source {
					counts = src.Counts
					break
				}
			}
			bucket.Counts = counts
		}
		bucket.Counts.Sources = nil
		dnsserver.MergeMonitoringBucket(&result.Points[len(result.Points)-1], bucket)
		if result.Summary.StartedAt.IsZero() {
			result.Summary.StartedAt = bucket.From
		}
		result.Summary.ObservedAt = bucket.To
	}

	result.TotalReceived = result.Summary.Received
	policySources := result.Summary.Sources
	if source != "" {
		policySources = nil
		for _, src := range result.Summary.Sources {
			if src.Source == source {
				policySources = append(policySources, src)
			}
		}
	}
	result.SourcePolicyFlows = dnsserver.SourcePolicyFlows(policySources)
	for _, src := range result.Summary.Sources {
		if src.Counts.Received == 0 && src.Counts.Completed == 0 && src.Counts.InFlight == 0 && src.Counts.Unconfirmed == 0 {
			continue
		}
		light := dnsserver.EmptyMonitoringCounts()
		light.Received = src.Counts.Received
		light.Completed = src.Counts.Completed
		light.InFlight = src.Counts.InFlight
		light.Unconfirmed = src.Counts.Unconfirmed
		light.LatencyMS = src.Counts.LatencyMS
		light.Flows = src.Counts.Flows
		result.Sources = append(result.Sources, dnsserver.DNSSourceCount{Source: src.Source, Counts: light})
	}
	if result.Sources == nil {
		result.Sources = []dnsserver.DNSSourceCount{}
	}
	if source != "" {
		summary := dnsserver.EmptyMonitoringCounts()
		summary.StartedAt = result.Summary.StartedAt
		summary.ObservedAt = result.Summary.ObservedAt
		for _, src := range result.Summary.Sources {
			if src.Source == source {
				start, at := summary.StartedAt, summary.ObservedAt
				summary = src.Counts
				summary.StartedAt, summary.ObservedAt = start, at
				break
			}
		}
		result.Summary = summary
	}
	result.Summary.Sources = nil
	return result, rows.Err()
}
