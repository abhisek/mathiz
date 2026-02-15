package diagnosis

import "github.com/abhisek/mathiz/internal/skillgraph"

// Misconception defines a known misconception pattern.
type Misconception struct {
	ID          string
	Strand      skillgraph.Strand
	Label       string
	Description string
	Examples    []string
}

// registry is the package-level misconception registry, keyed by ID.
var registry map[string]*Misconception

// byStrand indexes misconceptions by strand.
var byStrand map[skillgraph.Strand][]*Misconception

func init() {
	registry = make(map[string]*Misconception, len(seedMisconceptions))
	byStrand = make(map[skillgraph.Strand][]*Misconception)
	for i := range seedMisconceptions {
		m := &seedMisconceptions[i]
		registry[m.ID] = m
		byStrand[m.Strand] = append(byStrand[m.Strand], m)
	}
}

// GetMisconception returns a misconception by ID, or nil if not found.
func GetMisconception(id string) *Misconception {
	return registry[id]
}

// MisconceptionsByStrand returns all misconceptions for a given strand.
func MisconceptionsByStrand(strand skillgraph.Strand) []*Misconception {
	return byStrand[strand]
}

// AllMisconceptions returns every misconception in the taxonomy.
func AllMisconceptions() []*Misconception {
	result := make([]*Misconception, 0, len(registry))
	for _, m := range registry {
		result = append(result, m)
	}
	return result
}
