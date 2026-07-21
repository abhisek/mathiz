package server

import (
	"net/http"
	"sort"
	"sync"

	"github.com/abhisek/mathiz/internal/skillgraph"
)

// Public curriculum API — the static skill graph, served unauthenticated so
// the marketing/parent surfaces can render "what Mathiz teaches" without an
// account. The payload is fixed per binary (the graph is a package-level
// singleton seeded at init), so it is built once and cached.

type curriculumSkillJSON struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Grade    int      `json:"grade"`
	Prereqs  []string `json:"prereqs"`  // always present, may be empty
	Keywords []string `json:"keywords"` // search aliases (e.g. "GCF" for HCF & LCM); always present, may be empty
}

type curriculumIslandJSON struct {
	ID     string                `json:"id"`
	Name   string                `json:"name"`
	Skills []curriculumSkillJSON `json:"skills"`
}

type curriculumJSON struct {
	Islands []curriculumIslandJSON `json:"islands"`
}

var (
	curriculumOnce   sync.Once
	curriculumCached curriculumJSON
)

// curriculumPayload builds the response once from the skill graph: islands in
// canonical strand order, skills within an island by grade then name.
func curriculumPayload() curriculumJSON {
	curriculumOnce.Do(func() {
		out := curriculumJSON{Islands: make([]curriculumIslandJSON, 0, len(skillgraph.AllStrands()))}
		for _, strand := range skillgraph.AllStrands() {
			skills := skillgraph.ByStrand(strand)
			sort.Slice(skills, func(i, j int) bool {
				if skills[i].GradeLevel != skills[j].GradeLevel {
					return skills[i].GradeLevel < skills[j].GradeLevel
				}
				return skills[i].Name < skills[j].Name
			})
			island := curriculumIslandJSON{
				ID:     string(strand),
				Name:   skillgraph.StrandDisplayName(strand),
				Skills: make([]curriculumSkillJSON, len(skills)),
			}
			for i, sk := range skills {
				island.Skills[i] = curriculumSkillJSON{
					ID:       sk.ID,
					Name:     sk.Name,
					Grade:    sk.GradeLevel,
					Prereqs:  append([]string{}, sk.Prerequisites...),
					Keywords: append([]string{}, sk.Keywords...),
				}
			}
			out.Islands = append(out.Islands, island)
		}
		curriculumCached = out
	})
	return curriculumCached
}

// handleCurriculum serves the public skill-graph curriculum. Registered
// unconditionally; the content is static, so clients may cache it.
func (s *Server) handleCurriculum(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, curriculumPayload())
}
