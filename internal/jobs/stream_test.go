package jobs

import "testing"

func TestStreamSplitKeepsEachByteOnce(t *testing.T) {
	logs, err := NewLogStore(1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	track := &streamTrack{logs: logs, id: "job"}
	out, errOut := streamWriter{track: track}, streamWriter{track: track, stderr: true}
	if _, err := out.Write([]byte("OUT1")); err != nil {
		t.Fatal(err)
	}
	if _, err := errOut.Write([]byte("ERR")); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write([]byte("OUT2")); err != nil {
		t.Fatal(err)
	}
	combined, _, _, _, err := logs.Read("job", 0, 100)
	if err != nil || string(combined) != "OUT1ERROUT2" {
		t.Fatalf("combined = %q, %v", combined, err)
	}
	stdout := track.filter(combined, 0, StreamStdout)
	stderr := track.filter(combined, 0, StreamStderr)
	if string(stdout) != "OUT1OUT2" || string(stderr) != "ERR" {
		t.Fatalf("stdout %q stderr %q", stdout, stderr)
	}
	page := track.filter(combined[2:], 2, StreamStdout)
	if string(page) != "T1OUT2" {
		t.Fatalf("paged stdout = %q", page)
	}
}
