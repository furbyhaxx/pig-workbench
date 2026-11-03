package jobs

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestLog(t *testing.T, perJob, perSession int64) *LogStore {
	t.Helper()
	store, err := NewLogStore(perJob, perSession)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close log store: %v", err)
		}
	})
	return store
}

func TestLogTailGapAndPaging(t *testing.T) {
	store := newTestLog(t, 8, 100)
	for _, chunk := range []string{"abcde", "fghij", "klmnop"} {
		if err := store.Append("job", []byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	var combined []byte
	cursor := int64(0)
	for i := 0; ; i++ {
		page, next, truncated, gap, err := store.Read("job", cursor, 3)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && (!truncated || gap != 8 || next != 11) {
			t.Fatalf("first page %q next=%d truncated=%v gap=%d", page, next, truncated, gap)
		}
		if i > 0 && truncated {
			t.Fatalf("unexpected gap at cursor %d", cursor)
		}
		combined = append(combined, page...)
		if next == cursor {
			break
		}
		cursor = next
	}
	if string(combined) != "ijklmnop" || cursor != 16 {
		t.Fatalf("collected %q through cursor %d", combined, cursor)
	}
	written, retained, truncated, err := store.Facts("job")
	if err != nil || written != 16 || retained != 8 || !truncated {
		t.Fatalf("facts: written=%d retained=%d truncated=%v err=%v", written, retained, truncated, err)
	}
}

func TestLogSessionEvictionAndRevival(t *testing.T) {
	store := newTestLog(t, 10, 10)
	if err := store.Append("old", []byte("123456")); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("new", []byte("abcdef")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := store.Read("old", 0, 10); !errors.Is(err, ErrLogEvicted) {
		t.Fatalf("old read = %v, want evicted", err)
	} else {
		var evicted *LogEvictedError
		if !errors.As(err, &evicted) {
			t.Fatalf("eviction error has wrong type: %v", err)
		}
	}
	written, retained, lost, err := store.Facts("old")
	if written != 6 || retained != 0 || !lost || !errors.Is(err, ErrLogEvicted) {
		t.Fatalf("evicted facts: %d %d %v %v", written, retained, lost, err)
	}
	data, next, truncated, _, err := store.Read("new", 0, 10)
	if err != nil || string(data) != "abcdef" || next != 6 || truncated || store.SessionBytes() != 6 {
		t.Fatalf("new read %q next=%d truncated=%v session=%d err=%v", data, next, truncated, store.SessionBytes(), err)
	}
	if err := store.Append("old", []byte("XYZ")); err != nil {
		t.Fatal(err)
	}
	data, next, truncated, gap, err := store.Read("old", 0, 10)
	if err != nil || string(data) != "XYZ" || next != 9 || !truncated || gap != 6 {
		t.Fatalf("revived read %q next=%d truncated=%v gap=%d err=%v", data, next, truncated, gap, err)
	}
	written, retained, truncated, err = store.Facts("old")
	if err != nil || written != 9 || retained != 3 || !truncated {
		t.Fatalf("revived facts: %d %d %v %v", written, retained, truncated, err)
	}
}

func TestLogCurrentJobLargerThanSession(t *testing.T) {
	store := newTestLog(t, 100, 5)
	if err := store.Append("old", []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("new", []byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := store.Read("old", 0, 5); !errors.Is(err, ErrLogEvicted) {
		t.Fatalf("old read = %v", err)
	}
	data, next, truncated, gap, err := store.Read("new", 0, 10)
	if err != nil || string(data) != "56789" || next != 10 || !truncated || gap != 5 || store.SessionBytes() != 5 {
		t.Fatalf("new read %q next=%d truncated=%v gap=%d session=%d err=%v", data, next, truncated, gap, store.SessionBytes(), err)
	}
}

func TestLogEvictsOldestOtherWindow(t *testing.T) {
	store := newTestLog(t, 9, 9)
	for _, item := range []struct{ id, data string }{{"a", "aa"}, {"b", "bbb"}, {"c", "ccc"}} {
		if err := store.Append(item.id, []byte(item.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Append("a", []byte("aa")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := store.Read("b", 0, 10); !errors.Is(err, ErrLogEvicted) {
		t.Fatalf("oldest other job = %v", err)
	}
	if data, _, _, _, err := store.Read("c", 0, 10); err != nil || string(data) != "ccc" {
		t.Fatalf("newer job = %q, %v", data, err)
	}
	if err := store.Append("a", []byte("aaa")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := store.Read("c", 0, 10); !errors.Is(err, ErrLogEvicted) {
		t.Fatalf("second oldest job = %v", err)
	}
	data, next, truncated, gap, err := store.Read("a", 0, 10)
	if err != nil || string(data) != "aaaaaaa" || next != 7 || truncated || gap != 0 {
		t.Fatalf("protected job %q next=%d truncated=%v gap=%d err=%v", data, next, truncated, gap, err)
	}
	if err := store.Append("a", []byte("aaaaa")); err != nil {
		t.Fatal(err)
	}
	data, next, truncated, gap, err = store.Read("a", 0, 10)
	if err != nil || string(data) != "aaaaaaaaa" || next != 12 || !truncated || gap != 3 {
		t.Fatalf("trimmed protected job %q next=%d truncated=%v gap=%d err=%v", data, next, truncated, gap, err)
	}
}

func TestLogStablePagesAndCallerBuffer(t *testing.T) {
	store := newTestLog(t, 64, 64)
	input := []byte("abcdefghij")
	if err := store.Append("job", input); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'
	for i := 0; i < 2; i++ {
		data, next, truncated, _, err := store.Read("job", 2, 3)
		if err != nil || string(data) != "cde" || next != 5 || truncated {
			t.Fatalf("page %q next=%d truncated=%v err=%v", data, next, truncated, err)
		}
		data[0] = 'X'
	}
	for _, cursor := range []int64{-1, 11} {
		if _, _, _, _, err := store.Read("job", cursor, 3); !errors.Is(err, ErrInvalidLogCursor) {
			t.Fatalf("cursor %d: %v", cursor, err)
		}
	}
}

func TestLogSegmentRotation(t *testing.T) {
	const capBytes = 70001
	store := newTestLog(t, capBytes, 200000)
	var expected []byte
	var written int64
	for _, size := range []int{10000, 40000, 70000, 55000} {
		chunk := make([]byte, size)
		for i := range chunk {
			chunk[i] = byte((int(written) + i) % 251)
		}
		written += int64(size)
		if err := store.Append("job", chunk); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, chunk...)
		if len(expected) > capBytes {
			expected = expected[len(expected)-capBytes:]
		}
		var got []byte
		var cursor int64
		for {
			page, next, truncated, gap, err := store.Read("job", cursor, 7777)
			if err != nil {
				t.Fatal(err)
			}
			if cursor == 0 && written > capBytes && (!truncated || gap != written-int64(len(expected))) {
				t.Fatalf("written=%d truncated=%v gap=%d", written, truncated, gap)
			}
			got = append(got, page...)
			cursor = next
			if cursor == written {
				break
			}
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("written=%d: got %d bytes, want %d", written, len(got), len(expected))
		}
	}
}

func TestLogPageSize(t *testing.T) {
	store := newTestLog(t, 200000, 200000)
	if err := store.Append("job", bytes.Repeat([]byte{'x'}, 100000)); err != nil {
		t.Fatal(err)
	}
	for _, max := range []int{0, -1, 200000} {
		data, next, _, _, err := store.Read("job", 0, max)
		if err != nil || len(data) != 64*1024 || next != int64(len(data)) {
			t.Fatalf("max=%d: got %d bytes, next=%d, err=%v", max, len(data), next, err)
		}
	}
}

func TestLogDropAndRevival(t *testing.T) {
	store := newTestLog(t, 20, 20)
	if err := store.Append("job", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := store.Drop("job"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := store.Read("job", 0, 0); !errors.Is(err, ErrLogDropped) {
		t.Fatalf("dropped read = %v", err)
	} else {
		var dropped *LogDroppedError
		if !errors.As(err, &dropped) {
			t.Fatalf("dropped error has wrong type: %v", err)
		}
	}
	if store.SessionBytes() != 0 {
		t.Fatalf("session bytes = %d", store.SessionBytes())
	}
	if err := store.Append("job", []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, next, truncated, gap, err := store.Read("job", 0, 10)
	if err != nil || string(data) != "new" || next != 8 || !truncated || gap != 5 {
		t.Fatalf("revived read %q next=%d truncated=%v gap=%d err=%v", data, next, truncated, gap, err)
	}
}

func TestLogValidationAndClose(t *testing.T) {
	for _, caps := range [][2]int64{{0, 1}, {-1, 1}, {1, 0}, {1, -1}} {
		if _, err := NewLogStore(caps[0], caps[1]); err == nil {
			t.Fatalf("accepted caps %v", caps)
		}
	}
	store := newTestLog(t, 10, 10)
	if err := store.Append("", nil); !errors.Is(err, ErrInvalidLogID) {
		t.Fatalf("empty ID error = %v", err)
	}
	if _, _, _, _, err := store.Read("missing", 0, 10); !errors.Is(err, ErrLogNotFound) {
		t.Fatalf("missing read = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"job", ""} {
		if err := store.Append(id, []byte("x")); !errors.Is(err, ErrLogClosed) {
			t.Fatalf("append %q after close = %v", id, err)
		}
	}
	if _, _, _, _, err := store.Read("job", 0, 10); !errors.Is(err, ErrLogClosed) {
		t.Fatalf("read after close = %v", err)
	} else {
		var closed *LogClosedError
		if !errors.As(err, &closed) {
			t.Fatalf("closed error has wrong type: %v", err)
		}
	}
}

func TestLogReadDuringClose(t *testing.T) {
	store := newTestLog(t, 100, 100)
	if err := store.Append("job", []byte("contents")); err != nil {
		t.Fatal(err)
	}
	const readers = 8
	ready := make(chan struct{}, readers)
	start := make(chan struct{})
	failures := make(chan error, readers)
	var wg sync.WaitGroup
	for range readers {
		wg.Go(func() {
			ready <- struct{}{}
			<-start
			for {
				data, next, _, _, err := store.Read("job", 0, 0)
				if errors.Is(err, ErrLogClosed) {
					return
				}
				if err != nil || string(data) != "contents" || next != 8 {
					failures <- fmt.Errorf("read %q next=%d: %v", data, next, err)
					return
				}
			}
		})
	}
	for range readers {
		<-ready
	}
	close(start)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
}

func TestLogConcurrentAppendAndRead(t *testing.T) {
	const writers, records = 4, 200
	store := newTestLog(t, 1<<20, 1<<20)
	if err := store.Append("job", nil); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Go(func() {
			for n := 0; n < records; n++ {
				if err := store.Append("job", []byte(fmt.Sprintf("%d:%04d;", i, n))); err != nil {
					failures <- err
					return
				}
			}
		})
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	var got bytes.Buffer
	var cursor int64
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("concurrent log reader did not reach the final offset")
		default:
		}
		page, next, truncated, gap, err := store.Read("job", cursor, 37)
		if err != nil || truncated || gap != 0 || next < cursor {
			t.Fatalf("concurrent read next=%d truncated=%v gap=%d err=%v", next, truncated, gap, err)
		}
		got.Write(page)
		cursor = next
		select {
		case <-done:
			written, _, _, err := store.Facts("job")
			if err != nil {
				t.Fatal(err)
			}
			if cursor == written {
				goto complete
			}
		default:
		}
		runtime.Gosched()
	}
complete:
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	seen := make(map[string]int)
	for _, item := range strings.Split(got.String(), ";") {
		if item != "" {
			seen[item]++
		}
	}
	if len(seen) != writers*records {
		t.Fatalf("received %d unique records, want %d", len(seen), writers*records)
	}
	for i := 0; i < writers; i++ {
		for n := 0; n < records; n++ {
			key := fmt.Sprintf("%d:%04d", i, n)
			if seen[key] != 1 {
				t.Fatalf("record %q occurred %d times", key, seen[key])
			}
		}
	}
}
