package softwareupdate

import (
	"io"
	"time"
)

// Total is zero when the server does not provide a content length. Never
// infer a percentage from elapsed time or pretend an unknown size is known.
type DownloadProgress struct{ Bytes, Total int64 }

type downloadProgressReader struct {
	reader       io.Reader
	bytes, total int64
	last         time.Time
	report       func(DownloadProgress) error
}

func (r *downloadProgressReader) publish() error {
	if r.report == nil {
		return nil
	}
	r.last = time.Now()
	return r.report(DownloadProgress{r.bytes, r.total})
}

func (r *downloadProgressReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	r.bytes += int64(n)
	if n > 0 && time.Since(r.last) >= 200*time.Millisecond {
		if e := r.publish(); e != nil {
			return n, e
		}
	}
	return n, err
}
