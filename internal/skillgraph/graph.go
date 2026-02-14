package skillgraph

import (
	"fmt"
	"slices"
	"sort"
)

// graph holds the skill DAG with precomputed indices.
type graph struct {
	skills     []Skill
	byID       map[string]*Skill
	byStrand   map[Strand][]Skill
	byGrade    map[int][]Skill
	roots      []Skill
	dependents map[string][]string
	topoOrder  []Skill
	topoIndex  map[string]int
}

// g is the package-level graph singleton, set by init() in seed.go.
var g *graph

// buildGraph constructs the graph from a slice of skills.
// It builds all indices including topological order (Kahn's algorithm).
func buildGraph(skills []Skill) *graph {
	gr := &graph{
		skills:     skills,
		byID:       make(map[string]*Skill, len(skills)),
		byStrand:   make(map[Strand][]Skill),
		byGrade:    make(map[int][]Skill),
		dependents: make(map[string][]string),
		topoIndex:  make(map[string]int, len(skills)),
	}

	// Build ID index
	for i := range gr.skills {
		gr.byID[gr.skills[i].ID] = &gr.skills[i]
	}

	// Build reverse edges (dependents)
	for i := range gr.skills {
		for _, prereqID := range gr.skills[i].Prerequisites {
			gr.dependents[prereqID] = append(gr.dependents[prereqID], gr.skills[i].ID)
		}
	}

	// Topological sort (Kahn's algorithm)
	inDegree := make(map[string]int, len(skills))
	for i := range skills {
		inDegree[skills[i].ID] = len(skills[i].Prerequisites)
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	// Sort initial queue for deterministic ordering
	sort.Strings(queue)

	var topoOrder []Skill
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		skill := gr.byID[id]
		topoOrder = append(topoOrder, *skill)

		deps := gr.dependents[id]
		// Sort dependents for deterministic ordering
		sorted := make([]string, len(deps))
		copy(sorted, deps)
		sort.Strings(sorted)
		for _, depID := range sorted {
			inDegree[depID]--
			if inDegree[depID] == 0 {
				queue = append(queue, depID)
			}
		}
	}

	gr.topoOrder = topoOrder
	for i, s := range gr.topoOrder {
		gr.topoIndex[s.ID] = i
	}

	// Identify roots
	for i := range gr.skills {
		if len(gr.skills[i].Prerequisites) == 0 {
			gr.roots = append(gr.roots, gr.skills[i])
		}
	}

	// Strand ordering: all strands in defined order
	strandOrder := AllStrands()
	strandIdx := make(map[Strand]int, len(strandOrder))
	for i, s := range strandOrder {
		strandIdx[s] = i
	}

	// Group by strand, sorted by grade asc then topo index
	strandGroups := make(map[Strand][]Skill)
	for i := range gr.skills {
		s := gr.skills[i]
		strandGroups[s.Strand] = append(strandGroups[s.Strand], s)
	}
	for strand, skills := range strandGroups {
		sorted := make([]Skill, len(skills))
		copy(sorted, skills)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].GradeLevel != sorted[j].GradeLevel {
				return sorted[i].GradeLevel < sorted[j].GradeLevel
			}
			return gr.topoIndex[sorted[i].ID] < gr.topoIndex[sorted[j].ID]
		})
		gr.byStrand[strand] = sorted
	}

	// Group by grade, sorted by strand order then topo index
	gradeGroups := make(map[int][]Skill)
	for i := range gr.skills {
		s := gr.skills[i]
		gradeGroups[s.GradeLevel] = append(gradeGroups[s.GradeLevel], s)
	}
	for grade, skills := range gradeGroups {
		sorted := make([]Skill, len(skills))
		copy(sorted, skills)
		sort.Slice(sorted, func(i, j int) bool {
			si := strandIdx[sorted[i].Strand]
			sj := strandIdx[sorted[j].Strand]
			if si != sj {
				return si < sj
			}
			return gr.topoIndex[sorted[i].ID] < gr.topoIndex[sorted[j].ID]
		})
		gr.byGrade[grade] = sorted
	}

	return gr
}

// GetSkill returns a skill by ID, or error if not found.
func GetSkill(id string) (Skill, error) {
	s, ok := g.byID[id]
	if !ok {
		return Skill{}, fmt.Errorf("skill not found: %q", id)
	}
	return *s, nil
}

// AllSkills returns all skills in the graph.
func AllSkills() []Skill {
	return slices.Clone(g.skills)
}

// ByStrand returns all skills in a given strand, ordered by grade then topological position.
func ByStrand(strand Strand) []Skill {
	return slices.Clone(g.byStrand[strand])
}

// ByGrade returns all skills for a given grade level, ordered by strand then topological position.
func ByGrade(grade int) []Skill {
	return slices.Clone(g.byGrade[grade])
}

// RootSkills returns all skills with no prerequisites.
func RootSkills() []Skill {
	return slices.Clone(g.roots)
}

// Prerequisites returns the direct prerequisite skills for a given skill ID.
func Prerequisites(id string) []Skill {
	s, ok := g.byID[id]
	if !ok {
		return nil
	}
	result := make([]Skill, 0, len(s.Prerequisites))
	for _, prereqID := range s.Prerequisites {
		if p, ok := g.byID[prereqID]; ok {
			result = append(result, *p)
		}
	}
	return result
}

// Dependents returns skills that directly depend on the given skill ID.
func Dependents(id string) []Skill {
	depIDs := g.dependents[id]
	result := make([]Skill, 0, len(depIDs))
	for _, depID := range depIDs {
		if s, ok := g.byID[depID]; ok {
			result = append(result, *s)
		}
	}
	return result
}

// IsUnlocked returns true if all prerequisites for the given skill are in the mastered set.
func IsUnlocked(id string, mastered map[string]bool) bool {
	s, ok := g.byID[id]
	if !ok {
		return false
	}
	for _, prereqID := range s.Prerequisites {
		if !mastered[prereqID] {
			return false
		}
	}
	return true
}

// AvailableSkills returns all skills that are unlocked but not yet mastered.
func AvailableSkills(mastered map[string]bool) []Skill {
	var result []Skill
	for _, s := range g.topoOrder {
		if !mastered[s.ID] && IsUnlocked(s.ID, mastered) {
			result = append(result, s)
		}
	}
	return result
}

// FrontierSkills returns the subset of available skills that are "next" for the learner —
// skills whose prerequisites were most recently mastered. Without recency data,
// this returns available skills that have prerequisites (non-root), in topological order.
// Falls back to root skills if no non-root skills are available.
func FrontierSkills(mastered map[string]bool) []Skill {
	available := AvailableSkills(mastered)
	if len(available) == 0 {
		return nil
	}

	// Prefer skills that have prerequisites (they represent forward progress)
	var frontier []Skill
	for _, s := range available {
		if len(s.Prerequisites) > 0 {
			frontier = append(frontier, s)
		}
	}

	// Fall back to roots if nothing else is available
	if len(frontier) == 0 {
		return available
	}
	return frontier
}

// BlockedSkills returns all skills that have at least one unmastered prerequisite.
func BlockedSkills(mastered map[string]bool) []Skill {
	var result []Skill
	for _, s := range g.topoOrder {
		if !IsUnlocked(s.ID, mastered) {
			result = append(result, s)
		}
	}
	return result
}

// TopologicalOrder returns all skills in a valid topological order.
func TopologicalOrder() []Skill {
	return slices.Clone(g.topoOrder)
}

// Validate checks the graph for structural issues.
// It delegates to validateSkills with the graph's skill set.
func Validate() error {
	return validateSkills(g.skills)
}
