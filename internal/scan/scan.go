// Package scan reads JSONL session transcripts with a per-file cache, so a
// refresh only re-parses files that changed since the last one.
package scan

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sdillen/token-usage/internal/usage"
)

// ParseFunc turns the lines of one transcript into records. path identifies
// the file (and is typically used as the session key).
type ParseFunc func(path string, lines *LineReader) []usage.Record

// LineReader yields the complete lines of a transcript.
type LineReader struct {
	r *bufio.Reader
}

// Next returns the next complete line, or false at the end of the input.
// A trailing line without newline is still being written and is skipped.
func (l *LineReader) Next() ([]byte, bool) {
	for {
		line, err := l.r.ReadBytes('\n')
		if err != nil {
			return nil, false
		}
		if len(line) > 1 {
			return line, true
		}
	}
}

type entry struct {
	size    int64
	modTime time.Time
	records []usage.Record
}

// Scanner parses all *.jsonl files below a set of roots.
type Scanner struct {
	Roots []string
	Parse ParseFunc

	mu    sync.Mutex
	cache map[string]entry
}

// Scan returns the records of all transcripts. Unreadable files are skipped;
// ctx cancellation aborts the scan.
func (s *Scanner) Scan(ctx context.Context) ([]usage.Record, error) {
	var files []string
	for _, root := range s.Roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fs.SkipDir
				}
				return nil // permission problems must not abort the scan
			}
			if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
				files = append(files, path)
			}
			return ctx.Err()
		})
		if err != nil {
			return nil, err
		}
	}

	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]entry{}
	}
	old := s.cache
	s.mu.Unlock()

	fresh := make(map[string]entry, len(files))
	var freshMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, path := range files {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			if ctx.Err() != nil {
				return
			}
			e, ok := s.load(path, old[path])
			if !ok {
				return
			}
			freshMu.Lock()
			fresh[path] = e
			freshMu.Unlock()
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache = fresh
	s.mu.Unlock()

	var records []usage.Record
	for _, path := range files {
		records = append(records, fresh[path].records...)
	}
	return records, nil
}

func (s *Scanner) load(path string, cached entry) (entry, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return entry{}, false
	}
	if cached.size == info.Size() && cached.modTime.Equal(info.ModTime()) {
		return cached, true
	}
	f, err := os.Open(path)
	if err != nil {
		return entry{}, false
	}
	defer f.Close()
	records := s.Parse(path, &LineReader{r: bufio.NewReaderSize(io.Reader(f), 256<<10)})
	return entry{size: info.Size(), modTime: info.ModTime(), records: records}, true
}
