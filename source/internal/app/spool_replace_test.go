package app

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestConcurrentEmptyFeedMountOnlyCommitsOnce(t *testing.T) {
	s, u, d := setup(t)
	p, stock := fixture(t, s, u)
	stock.Mount = nil
	stock.Material = "PETG"
	stock.Color = "#fce300"
	if e := s.SaveSpool(u, stock); e != nil {
		t.Fatal(e)
	}
	spools, _ := readSpools(s.DB)
	stock = spools[0]
	now := time.Now()
	yes := true
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, Equipment: Equipment{Feeds: []FeedReading{{Slot: "1", Material: "PETG", Color: "FCE300FF", Present: &yes, Updated: now}}}}
	d.mu.Unlock()
	in := SpoolReplacement{Mount: &Mount{p.ID, "1"}, ExistingID: stock.ID, ExistingRevision: stock.Revision}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.ReplaceSpool(u, in); results <- e }()
	}
	wg.Wait()
	close(results)
	passed := 0
	for e := range results {
		if e == nil {
			passed++
		}
	}
	if passed != 1 {
		t.Fatalf("expected one commit, got %d", passed)
	}
	after, _ := readSpools(s.DB)
	if len(after) != 1 || after[0].ID != stock.ID || after[0].Remaining != stock.Remaining || after[0].Revision != stock.Revision+1 {
		t.Fatal("concurrent mount changed inventory")
	}
}

func TestEmptyFeedMountsOriginalStockAndRejectsRaces(t *testing.T) {
	for _, mode := range []string{"existing", "new", "occupied", "stale", "material", "reserved", "offline"} {
		t.Run(mode, func(t *testing.T) {
			s, u, d := setup(t)
			p, stock := fixture(t, s, u)
			stock.Mount = nil
			stock.Material = "PETG"
			stock.Color = "#fce300"
			stock.Remaining = 427
			if err := s.SaveSpool(u, stock); err != nil {
				t.Fatal(err)
			}
			spools, _ := readSpools(s.DB)
			stock = spools[0]
			if mode == "occupied" {
				other := stock
				other.ID = ""
				other.Revision = 0
				other.Mount = &Mount{p.ID, "1"}
				if err := s.SaveSpool(u, other); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "reserved" {
				newJob(t, s, u, p, stock, 10)
			}
			now := time.Now()
			yes := true
			material := "PETG"
			if mode == "material" {
				material = "PLA"
			}
			d.mu.Lock()
			d.statuses[p.ID] = Telemetry{Connected: mode != "offline", Updated: now, Equipment: Equipment{Feeds: []FeedReading{{Slot: "1", Material: material, Color: "F6DA5AFF", Present: &yes, Updated: now}}}}
			d.mu.Unlock()
			in := SpoolReplacement{Mount: &Mount{p.ID, "1"}, ExistingID: stock.ID, ExistingRevision: stock.Revision, ConfirmColorDifference: true, ReportedColor: "#f6da5a"}
			if mode == "stale" {
				in.ExistingRevision++
			}
			if mode == "new" {
				in.ExistingID = ""
				in.ExistingRevision = 0
				in.Spool = stock
				in.Spool.ID = ""
				in.Spool.Revision = 0
				in.Spool.Mount = in.Mount
				in.Spool.Color = "#f6da5a"
				in.Spool.Remaining = 900
			}
			before, _ := readSpools(s.DB)
			result, err := s.ReplaceSpool(u, in)
			after, _ := readSpools(s.DB)
			if mode != "existing" && mode != "new" {
				if err == nil {
					t.Fatal("invalid mount accepted")
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("failed mount changed inventory")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "existing" {
				want := stock
				want.Mount = in.Mount
				want.Revision++
				if len(after) != 1 || result["newId"] != stock.ID || !reflect.DeepEqual(after[0], want) {
					t.Fatal("returning stock rewritten or duplicated")
				}
			} else {
				if len(after) != 2 || result["newId"] == stock.ID {
					t.Fatal("new spool reused old identity")
				}
			}
			if _, err = s.ReplaceSpool(u, in); err == nil {
				t.Fatal("duplicate mount accepted")
			}
		})
	}
}

func TestReturningSpoolColorIsInventoryMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, material, color     string
		confirm, reserved, accept bool
	}{
		{"unconfirmed", "PETG", "#f6da5a", false, false, true},
		{"confirmed", "PETG", "#f6da5a", true, false, true},
		{"report_changed", "PETG", "#ffff00", true, false, true},
		{"material_conflict", "PLA", "#f6da5a", true, false, false},
		{"reserved", "PETG", "#f6da5a", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, u, d := setup(t)
			p, old := fixture(t, s, u)
			old.Mount = &Mount{p.ID, "1"}
			if err := s.SaveSpool(u, old); err != nil {
				t.Fatal(err)
			}
			target := old
			target.ID = ""
			target.Revision = 0
			target.Mount = nil
			target.Color = "#fce300"
			target.Material = tc.material
			target.Remaining = 427
			if err := s.SaveSpool(u, target); err != nil {
				t.Fatal(err)
			}
			spools, _ := readSpools(s.DB)
			for _, sp := range spools {
				if sp.ID == old.ID {
					old = sp
				} else {
					target = sp
				}
			}
			if tc.reserved {
				newJob(t, s, u, p, old, 10)
			}
			before, _ := readSpools(s.DB)
			now := time.Now()
			yes := true
			d.mu.Lock()
			d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, Equipment: Equipment{Feeds: []FeedReading{{Slot: "1", Material: "PETG", ProfileID: "GFG00", Color: "F6DA5AFF", Present: &yes, Updated: now}}}}
			d.mu.Unlock()
			result, err := s.ReplaceSpool(u, SpoolReplacement{OldID: old.ID, Revision: old.Revision, ExistingID: target.ID, ExistingRevision: target.Revision, ProfileID: "GFG00", ConfirmColorDifference: tc.confirm, ReportedColor: tc.color})
			after, _ := readSpools(s.DB)
			if !tc.accept {
				if err == nil {
					t.Fatal("unsafe replacement accepted")
				}
				if !reflect.DeepEqual(before, after) {
					t.Fatal("rejected replacement changed stock")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result["newId"] != target.ID || len(after) != 2 {
				t.Fatal("identity changed or duplicate created")
			}
			for _, sp := range after {
				if sp.ID == target.ID {
					want := target
					want.Mount = old.Mount
					want.Revision++
					if !reflect.DeepEqual(want, sp) {
						t.Fatal("stock details overwritten")
					}
				} else {
					want := old
					want.Mount = nil
					want.Revision++
					if !reflect.DeepEqual(want, sp) {
						t.Fatal("old spool altered")
					}
				}
			}
		})
	}
}

func TestReplacementReusesExistingStockWithoutCreatingOrRewritingIt(t *testing.T) {
	s, u, d := setup(t)
	p, old := fixture(t, s, u)
	old.Mount = &Mount{p.ID, "1"}
	if e := s.SaveSpool(u, old); e != nil {
		t.Fatal(e)
	}
	target := old
	target.ID = ""
	target.Revision = 0
	target.Mount = nil
	target.Material = "PETG"
	target.Label = "My original grey roll"
	target.Color = "#dcdcdc"
	target.Remaining = 427
	if e := s.SaveSpool(u, target); e != nil {
		t.Fatal(e)
	}
	spools, _ := readSpools(s.DB)
	for _, sp := range spools {
		if sp.ID == old.ID {
			old = sp
		} else {
			target = sp
		}
	}
	now := time.Now()
	yes := true
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, Equipment: Equipment{Feeds: []FeedReading{{Slot: "1", Material: "PETG", ProfileID: "GFG02", Color: "DCDCDCFF", Present: &yes, Updated: now}}}}
	d.mu.Unlock()
	in := SpoolReplacement{OldID: old.ID, Revision: old.Revision, ExistingID: target.ID, ExistingRevision: target.Revision, ProfileID: "GFG02"}
	bad := in
	bad.ExistingRevision++
	if _, e := s.ReplaceSpool(u, bad); e == nil {
		t.Fatal("stale target version accepted")
	}
	bad = in
	bad.Spool = target
	if _, e := s.ReplaceSpool(u, bad); e == nil {
		t.Fatal("stock record override accepted")
	}
	result, e := s.ReplaceSpool(u, in)
	if e != nil {
		t.Fatal(e)
	}
	if result["newId"] != target.ID {
		t.Fatal("existing ID replaced")
	}
	spools, _ = readSpools(s.DB)
	if len(spools) != 2 {
		t.Fatal("duplicate spool created")
	}
	for _, sp := range spools {
		if sp.ID == old.ID {
			want := old
			want.Mount = nil
			want.Revision++
			if !reflect.DeepEqual(sp, want) {
				t.Fatal("old roll changed")
			}
		} else {
			want := target
			want.Mount = old.Mount
			want.Revision++
			if !reflect.DeepEqual(sp, want) {
				t.Fatal("returning roll identity or weight changed", sp, want)
			}
		}
	}
	if _, e = s.ReplaceSpool(u, in); e == nil {
		t.Fatal("duplicate swap accepted")
	}
}

func TestSpoolReplacementIsAtomicAndPreservesOldIdentity(t *testing.T) {
	s, u, d := setup(t)
	p, old := fixture(t, s, u)
	old.Mount = &Mount{p.ID, "1"}
	old.Color = "#0000ff"
	if e := s.SaveSpool(u, old); e != nil {
		t.Fatal(e)
	}
	spools, _ := readSpools(s.DB)
	old = spools[0]
	before := old
	now := time.Now()
	yes := true
	telemetry := Telemetry{Connected: true, Updated: now, State: "idle", Equipment: Equipment{Feeds: []FeedReading{{Slot: "1", Material: "PETG", ProfileID: "GFG02", Color: "DCDCDCFF", Present: &yes, Updated: now}}}}
	d.mu.Lock()
	d.statuses[p.ID] = telemetry
	d.mu.Unlock()
	next := old
	next.ID = ""
	next.Revision = 0
	next.Material = "PETG"
	next.Label = "Grey PETG HF"
	next.Color = "#dcdcdc"
	next.ColorName = "Grey"
	next.Capacity = 1000
	next.Remaining = 900
	in := SpoolReplacement{OldID: old.ID, Revision: old.Revision, ProfileID: "GFG02", Spool: next}
	bad := in
	bad.Spool.Brand = ""
	if _, e := s.ReplaceSpool(u, bad); e == nil {
		t.Fatal("invalid new spool accepted")
	}
	spools, _ = readSpools(s.DB)
	if len(spools) != 1 || !reflect.DeepEqual(spools[0], before) {
		t.Fatal("failed replacement altered old record")
	}
	bad = in
	bad.ProfileID = "GFG00"
	if _, e := s.ReplaceSpool(u, bad); e == nil {
		t.Fatal("wrong report accepted")
	}
	other := User{ID: 999}
	if _, e := s.ReplaceSpool(other, in); e == nil {
		t.Fatal("unknown user accepted")
	}
	result, e := s.ReplaceSpool(u, in)
	if e != nil {
		t.Fatal(e)
	}
	if result["newId"] == old.ID {
		t.Fatal("old identity reused")
	}
	spools, _ = readSpools(s.DB)
	if len(spools) != 2 {
		t.Fatal("new spool missing")
	}
	for _, sp := range spools {
		if sp.ID == old.ID {
			want := before
			want.Mount = nil
			want.Revision++
			if !reflect.DeepEqual(sp, want) {
				t.Fatal("old material, name, stock or history changed", sp, want)
			}
		} else {
			if sp.Mount == nil || *sp.Mount != *before.Mount || sp.Remaining != 900 || sp.Material != "PETG" || sp.Label != "Grey PETG HF" {
				t.Fatal("new spool incorrect", sp)
			}
		}
	}
	if _, e = s.ReplaceSpool(u, in); e == nil {
		t.Fatal("duplicate request accepted")
	}
	spools, _ = readSpools(s.DB)
	if len(spools) != 2 {
		t.Fatal("duplicate new record")
	}
}

func TestSpoolReplacementBlocksReservedOldSpool(t *testing.T) {
	s, u, d := setup(t)
	p, old := fixture(t, s, u)
	old.Mount = &Mount{p.ID, "1"}
	s.SaveSpool(u, old)
	spools, _ := readSpools(s.DB)
	old = spools[0]
	newJob(t, s, u, p, old, 10)
	now := time.Now()
	yes := true
	d.mu.Lock()
	d.statuses[p.ID] = Telemetry{Connected: true, Updated: now, Equipment: Equipment{Feeds: []FeedReading{{Slot: "1", Material: "PETG", ProfileID: "GFG02", Color: "DCDCDCFF", Present: &yes, Updated: now}}}}
	d.mu.Unlock()
	next := old
	next.ID = ""
	next.Revision = 0
	next.Material = "PETG"
	next.Color = "#dcdcdc"
	if _, e := s.ReplaceSpool(u, SpoolReplacement{OldID: old.ID, Revision: old.Revision, ProfileID: "GFG02", Spool: next}); e == nil {
		t.Fatal("reserved spool replaced")
	}
	spools, _ = readSpools(s.DB)
	if len(spools) != 1 || spools[0].Mount == nil {
		t.Fatal("old mount altered")
	}
}
