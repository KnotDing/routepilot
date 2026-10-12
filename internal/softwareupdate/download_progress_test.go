package softwareupdate

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestDownloadProgressKnownAndUnknownLength(t *testing.T) {
	for _, total := range []int64{4, -1} {
		var reports []DownloadProgress
		transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
			r := httpResponse(200, "abcd", nil)
			r.ContentLength = total
			return r, nil
		})
		data, err := downloadWithTransportProgress(context.Background(), origin+"/a", 10, transport, func(p DownloadProgress) error { reports = append(reports, p); return nil })
		if err != nil || string(data) != "abcd" || len(reports) < 2 {
			t.Fatal(string(data), reports, err)
		}
		last := reports[len(reports)-1]
		if last.Bytes != 4 || last.Total != max(0, total) || reports[0].Bytes != 0 {
			t.Fatal(reports)
		}
	}
}

func TestDownloadProgressDoesNotMaskStatusPersistenceFailure(t *testing.T) {
	failure := errors.New("fixture status write failed")
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) { return httpResponse(200, "abcd", nil), nil })
	_, err := downloadWithTransportProgress(context.Background(), origin+"/a", 10, transport, func(DownloadProgress) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

func TestManagerPublishesDownloadProgressWithoutLaunchingAnotherUpdate(t *testing.T) {
	f := isolatedManager(t)
	m := f.manager
	reported, finish := make(chan struct{}), make(chan struct{})
	m.ops.prepare = func(ctx context.Context, r release, before func() error, progress func(DownloadProgress) error) error {
		if err := progress(DownloadProgress{2, 4}); err != nil {
			return err
		}
		close(reported)
		<-finish
		return nil
	}
	if _, err := m.Start(m.latest.Tag); err != nil {
		t.Fatal(err)
	}
	<-reported
	defer close(finish)
	status := m.Status()
	if status.DownloadBytes != 2 || status.DownloadTotal != 4 || status.Phase != "downloading" || status.Target != m.latest.Tag {
		t.Fatal(status)
	}
	if _, err := m.Start(m.latest.Tag); err == nil {
		t.Fatal("accepted duplicate install")
	}
}
