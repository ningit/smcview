// Package util provides some utility functions
package util

import (
	"github.com/ningit/smcview/maude"
	"log"
	"os"
	"strings"
)

// TermSimplifier simplifies terms.
type TermSimplifier interface {
	// Simplify simplifies a term
	Simplify(string) string
}

type dummySimplifier struct {}

// CreateDummySimplifier creates an empty simplifier that simply returns
// the input term as output.
func CreateDummySimplifier() TermSimplifier {
	return &dummySimplifier{}
}

// Simplify is the identifity for the dummy simplifier.
func (vs *dummySimplifier) Simplify(term string) string {
	return term
}

type termReducer struct {
	maudec     *maude.Client
	simplifier string
}

// CreateSimplifier constructs a simplifier that uses Maude to simplify
// terms. The simplification function should be defined in a file named
// smcview-simp.maude in the current working directory and its name is given
// by opname.
//
// Be aware that the file smcview-simp.maude will be loaded and any command
// within it will be executed.
func CreateSimplifier(opname string, maudec *maude.Client) TermSimplifier {
	// Fallbacks to the dummy simplifier when the requirements
	// of the reduction simplifier are not met
	if opname == "" || maudec == nil {
		return &dummySimplifier{}
	}

	if _, err := os.Stat("smcview-simpl.maude"); err != nil {
		log.Println("the file 'smcview-simp.maude' required by the simplifier is not available in the working directory.")
		return &dummySimplifier{}
	}

	maudec.Start()
	maudec.Load("smcview-simpl.maude")

	return &termReducer{maudec, opname}
}

// Simplify reduces the given operator applied to the input term.
func (tr *termReducer) Simplify(term string) string {
	var result = tr.maudec.Reduce(tr.simplifier + "((" + term + "))")

	if result.Ok {
		// If the result is a string (or it seems to be),
		// its quotes are removed
		if result.Term[0] == '"' {
			return strings.TrimPrefix(strings.TrimSuffix(result.Term, "\""), "\"")
		}

		return result.Term
	} else {
		return term
	}
}
