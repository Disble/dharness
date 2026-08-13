package mutation

import (
	"github.com/Disble/ditto/viruses"
	"github.com/Disble/ditto/viruses/arithmetic"
	"github.com/Disble/ditto/viruses/arithmeticassignment"
	"github.com/Disble/ditto/viruses/arithmeticassignmentinvert"
	"github.com/Disble/ditto/viruses/bitwise"
	"github.com/Disble/ditto/viruses/comparison"
	"github.com/Disble/ditto/viruses/comparisoninvert"
	"github.com/Disble/ditto/viruses/comparisonreplace"
	"github.com/Disble/ditto/viruses/floatdecrement"
	"github.com/Disble/ditto/viruses/floatincrement"
	"github.com/Disble/ditto/viruses/integerdecrement"
	"github.com/Disble/ditto/viruses/integerincrement"
	"github.com/Disble/ditto/viruses/loopbreak"
	"github.com/Disble/ditto/viruses/loopcondition"
	"github.com/Disble/ditto/viruses/rangebreak"
)

// DefaultViruses mirrors ditto v0.1.0's unexported default set. WithViruses
// replaces that set, so line scoping must name all fourteen mutators explicitly.
func DefaultViruses() []viruses.Virus {
	return []viruses.Virus{
		arithmetic.New(),
		arithmeticassignment.New(),
		arithmeticassignmentinvert.New(),
		bitwise.New(),
		comparison.New(),
		comparisoninvert.New(),
		comparisonreplace.New(),
		floatdecrement.New(),
		floatincrement.New(),
		integerdecrement.New(),
		integerincrement.New(),
		loopbreak.New(),
		loopcondition.New(),
		rangebreak.New(),
	}
}
