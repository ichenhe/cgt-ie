package classify

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// fakeQuerier answers from a fixed table and records what it was asked.
type fakeQuerier struct {
	name  string
	table map[string]Info
	err   error
	asked [][]string
}

func (f *fakeQuerier) Name() string { return f.name }
func (f *fakeQuerier) Query(_ context.Context, isins []string) (map[string]Info, error) {
	f.asked = append(f.asked, append([]string(nil), isins...))
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]Info{}
	for _, i := range isins {
		if v, ok := f.table[i]; ok {
			out[i] = v
		}
	}
	return out, nil
}

func TestChainAsksLaterQueriersOnlyForUnknowns(t *testing.T) {
	a := &fakeQuerier{name: "a", table: map[string]Info{"X1": {CFI: "ESVUFR"}}}
	b := &fakeQuerier{name: "b", table: map[string]Info{"X2": {Issuer: IssuerCompany, IssuerName: "Co"}}}
	got, err := Chain(a, b).Query(context.Background(), []string{"X1", "X2", "X3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.asked) != 1 || len(a.asked[0]) != 3 {
		t.Errorf("a asked %v, want one call with 3 ISINs", a.asked)
	}
	if len(b.asked) != 1 || len(b.asked[0]) != 2 {
		t.Errorf("b asked %v, want one call with X2 and X3 only", b.asked)
	}
	if got["X1"].CFI != "ESVUFR" || got["X2"].Issuer != IssuerCompany {
		t.Errorf("got %+v", got)
	}
	if _, ok := got["X3"]; ok {
		t.Error("X3 should be absent")
	}
}

func TestChainReportsFailureButReturnsWhatOthersFound(t *testing.T) {
	down := &fakeQuerier{name: "down", err: errors.New("timeout")}
	ok := &fakeQuerier{name: "ok", table: map[string]Info{"X1": {CFI: "CEOGES"}}}
	got, err := Chain(down, ok).Query(context.Background(), []string{"X1"})
	if err == nil || got["X1"].CFI != "CEOGES" {
		t.Errorf("got %+v err=%v", got, err)
	}
	if Chain(down, ok).Name() != "down>ok" {
		t.Errorf("name = %q", Chain(down, ok).Name())
	}
}

func TestCachedServesHitsAndRemembersMisses(t *testing.T) {
	inner := &fakeQuerier{name: "s", table: map[string]Info{"X1": {CFI: "ESVUFR"}}}
	q := Cached("", inner)

	got, err := q.Query(context.Background(), []string{"x1", "X9", "X1"})
	if err != nil {
		t.Fatal(err)
	}
	if got["X1"].CFI != "ESVUFR" || len(got) != 1 {
		t.Errorf("got %+v", got)
	}
	if len(inner.asked) != 1 || len(inner.asked[0]) != 2 {
		t.Errorf("inner asked %v, want one deduplicated call", inner.asked)
	}

	inner.asked = nil
	if _, err := q.Query(context.Background(), []string{"X1", "X9"}); err != nil {
		t.Fatal(err)
	}
	if len(inner.asked) != 0 {
		t.Errorf("hit and remembered miss should not reach inner: %v", inner.asked)
	}
}

func TestCachedDoesNotRememberMissesDuringOutage(t *testing.T) {
	inner := &fakeQuerier{name: "s", err: errors.New("timeout")}
	q := Cached("", inner)
	if _, err := q.Query(context.Background(), []string{"X2"}); err == nil {
		t.Fatal("expected outage error")
	}
	inner.err, inner.table = nil, map[string]Info{"X2": {CFI: "DAZSPR"}}
	got, err := q.Query(context.Background(), []string{"X2"})
	if err != nil || got["X2"].CFI != "DAZSPR" {
		t.Errorf("X2 not retried after outage: %+v %v", got, err)
	}
}

func TestCachedPersistsToDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	src := &fakeQuerier{name: "s", table: map[string]Info{"X1": {CFI: "ESVUFR"}}}
	if _, err := Cached(path, src).Query(context.Background(), []string{"X1", "X9"}); err != nil {
		t.Fatal(err)
	}
	q2 := Cached(path, &fakeQuerier{name: "s", err: errors.New("should not be called")})
	got, err := q2.Query(context.Background(), []string{"X1", "X9"})
	if err != nil {
		t.Fatalf("cache not used: %v", err)
	}
	if got["X1"].CFI != "ESVUFR" {
		t.Errorf("got %+v", got)
	}
}
