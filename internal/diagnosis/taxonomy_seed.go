package diagnosis

import "github.com/abhisek/mathiz/internal/skillgraph"

// seedMisconceptions defines the MVP misconception taxonomy.
// 19 misconceptions across 5 strands (3–4 each).
var seedMisconceptions = []Misconception{
	// Number & Place Value (4)
	{
		ID:          "npv-place-swap",
		Strand:      skillgraph.StrandNumberPlace,
		Label:       "Place value swap",
		Description: "Confuses ones/tens/hundreds positions; e.g., reads 305 as 350",
		Examples:    []string{"305 read as 350", "1023 written as 1032"},
	},
	{
		ID:          "npv-zero-placeholder",
		Strand:      skillgraph.StrandNumberPlace,
		Label:       "Zero placeholder ignored",
		Description: "Drops or ignores zero in place value; e.g., 407 becomes 47",
		Examples:    []string{"407 written as 47", "3006 written as 36"},
	},
	{
		ID:          "npv-compare-digits",
		Strand:      skillgraph.StrandNumberPlace,
		Label:       "Digit-count comparison",
		Description: "Compares numbers by digit count alone; thinks 99 > 100 because 9 > 1",
		Examples:    []string{"99 > 100", "8 > 12"},
	},
	{
		ID:          "npv-rounding-direction",
		Strand:      skillgraph.StrandNumberPlace,
		Label:       "Rounding direction",
		Description: "Rounds the wrong way; e.g., rounds 45 down to 40 instead of up to 50",
		Examples:    []string{"45 rounded to 40", "350 rounded to 300"},
	},

	// Addition & Subtraction (4)
	{
		ID:          "add-no-carry",
		Strand:      skillgraph.StrandAddSub,
		Label:       "Forgot to carry/regroup",
		Description: "Adds columns independently without carrying; e.g., 47 + 38 = 715",
		Examples:    []string{"47 + 38 = 715", "256 + 178 = 3214"},
	},
	{
		ID:          "add-no-borrow",
		Strand:      skillgraph.StrandAddSub,
		Label:       "Forgot to borrow/regroup",
		Description: "Subtracts smaller from larger in each column regardless of position; e.g., 42 - 17 = 35",
		Examples:    []string{"42 - 17 = 35", "503 - 267 = 344"},
	},
	{
		ID:          "add-sign-confusion",
		Strand:      skillgraph.StrandAddSub,
		Label:       "Sign confusion",
		Description: "Adds when should subtract or vice versa",
		Examples:    []string{"5 - 3 = 8", "12 + 7 = 5"},
	},
	{
		ID:          "add-left-to-right",
		Strand:      skillgraph.StrandAddSub,
		Label:       "Left-to-right processing",
		Description: "Processes digits left to right incorrectly, mishandling carries",
		Examples:    []string{"345 + 278 = 5113"},
	},

	// Multiplication & Division (4)
	{
		ID:          "mul-add-confusion",
		Strand:      skillgraph.StrandMultDiv,
		Label:       "Multiplied instead of added (or vice versa)",
		Description: "Confuses multiplication with repeated addition; e.g., 4 × 3 = 7",
		Examples:    []string{"4 × 3 = 7", "6 × 2 = 8"},
	},
	{
		ID:          "mul-partial-product",
		Strand:      skillgraph.StrandMultDiv,
		Label:       "Partial product error",
		Description: "Forgets to add partial products in multi-digit multiplication",
		Examples:    []string{"23 × 4 = 82 (forgot tens partial product)"},
	},
	{
		ID:          "div-remainder-ignore",
		Strand:      skillgraph.StrandMultDiv,
		Label:       "Remainder ignored",
		Description: "Drops the remainder entirely; e.g., 17 ÷ 5 = 3 instead of 3 R2",
		Examples:    []string{"17 ÷ 5 = 3", "25 ÷ 4 = 6"},
	},
	{
		ID:          "div-dividend-divisor-swap",
		Strand:      skillgraph.StrandMultDiv,
		Label:       "Dividend/divisor swap",
		Description: "Divides the smaller number by the larger; e.g., 6 ÷ 18 instead of 18 ÷ 6",
		Examples:    []string{"computes 6 ÷ 18 when asked 18 ÷ 6"},
	},

	// Fractions (4)
	{
		ID:          "frac-add-straight",
		Strand:      skillgraph.StrandFractions,
		Label:       "Straight-across addition",
		Description: "Adds numerators and denominators separately; e.g., 1/2 + 1/3 = 2/5",
		Examples:    []string{"1/2 + 1/3 = 2/5", "3/4 + 1/2 = 4/6"},
	},
	{
		ID:          "frac-larger-denom",
		Strand:      skillgraph.StrandFractions,
		Label:       "Larger denominator = larger fraction",
		Description: "Thinks 1/8 > 1/4 because 8 > 4",
		Examples:    []string{"1/8 > 1/4", "1/10 > 1/5"},
	},
	{
		ID:          "frac-whole-number-compare",
		Strand:      skillgraph.StrandFractions,
		Label:       "Whole number comparison",
		Description: "Compares fractions as if they were whole numbers; e.g., 3/4 < 5/8 because 3 < 5",
		Examples:    []string{"3/4 < 5/8", "2/3 < 4/7"},
	},
	{
		ID:          "frac-simplify-error",
		Strand:      skillgraph.StrandFractions,
		Label:       "Simplification error",
		Description: "Reduces fractions incorrectly; e.g., simplifies 4/6 to 2/4 instead of 2/3",
		Examples:    []string{"4/6 simplified to 2/4", "6/9 simplified to 3/6"},
	},

	// Measurement (3)
	{
		ID:          "meas-unit-confusion",
		Strand:      skillgraph.StrandMeasurement,
		Label:       "Unit confusion",
		Description: "Mixes up units; e.g., uses centimeters where meters are needed",
		Examples:    []string{"200cm instead of 2m", "3kg instead of 3000g"},
	},
	{
		ID:          "meas-conversion-direction",
		Strand:      skillgraph.StrandMeasurement,
		Label:       "Conversion direction",
		Description: "Multiplies when should divide (or vice versa) during unit conversion",
		Examples:    []string{"2m = 200cm but says 0.02cm"},
	},
	{
		ID:          "meas-perimeter-area",
		Strand:      skillgraph.StrandMeasurement,
		Label:       "Perimeter/area confusion",
		Description: "Computes perimeter when area is asked, or vice versa",
		Examples:    []string{"area of 3×4 rectangle = 14 (perimeter)", "perimeter = 12 (area)"},
	},
}
