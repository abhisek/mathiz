package server

import (
	"slices"
	"testing"
)

type curriculumPageJSON struct {
	Islands []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Skills []struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			Grade   int      `json:"grade"`
			Prereqs []string `json:"prereqs"`
		} `json:"skills"`
	} `json:"islands"`
}

func TestCurriculumIsPublic(t *testing.T) {
	e := newTestEnv(t)

	var cur curriculumPageJSON
	resp := e.call(t, "GET", "/api/v1/curriculum", "", nil, &cur)
	expectStatus(t, resp, 200, "curriculum")
	if cc := resp.Header.Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q, want %q", cc, "public, max-age=3600")
	}

	if len(cur.Islands) == 0 {
		t.Fatal("islands empty")
	}
	if cur.Islands[0].ID != "number-and-place-value" || cur.Islands[0].Name != "Number & Place Value" {
		t.Errorf("first island = %s (%s), want number-and-place-value (Number & Place Value)",
			cur.Islands[0].ID, cur.Islands[0].Name)
	}

	// One known skill lands in its island with the right grade and prereq.
	var found bool
	for _, island := range cur.Islands {
		for _, sk := range island.Skills {
			if sk.Prereqs == nil {
				t.Errorf("skill %s: prereqs is null, want an array (possibly empty)", sk.ID)
			}
			if sk.ID != "round-nearest-10-100" {
				continue
			}
			found = true
			if island.ID != "number-and-place-value" {
				t.Errorf("round-nearest-10-100 in island %s, want number-and-place-value", island.ID)
			}
			if sk.Name != "Round to Nearest 10 or 100" || sk.Grade != 3 {
				t.Errorf("round-nearest-10-100 = %+v", sk)
			}
			if !slices.Contains(sk.Prereqs, "pv-hundreds") {
				t.Errorf("round-nearest-10-100 prereqs = %v, want to contain pv-hundreds", sk.Prereqs)
			}
		}
	}
	if !found {
		t.Error("skill round-nearest-10-100 not present in curriculum")
	}

	// Skills within an island are ordered by grade, then name.
	for _, island := range cur.Islands {
		for i := 1; i < len(island.Skills); i++ {
			prev, cur := island.Skills[i-1], island.Skills[i]
			if prev.Grade > cur.Grade || (prev.Grade == cur.Grade && prev.Name > cur.Name) {
				t.Errorf("island %s: skills out of order at %d: %s (g%d) then %s (g%d)",
					island.ID, i, prev.Name, prev.Grade, cur.Name, cur.Grade)
			}
		}
	}
}
