// Package grapher allows generating graphs from model checker dumps.
package grapher

import (
	"fmt"
	"github.com/ningit/smcview/smcdump"
	"github.com/ningit/smcview/util"
	"io"
	"log"
	"os/exec"
)

// String constants
const (
	legendBegin = "[shape=plaintext, label=< <table cellspacing=\"0\" border =\"0\" cellborder=\"1\">\n"
	legendElem  = "\t\t<tr><td>%d</td><td>%s</td></tr>\n"
	legendEnd   = "\t</table> >];\n"
)

// GraphOpt is a configuration flag for the grapher. It allows selecting how node labels are printed.
type GraphOpt int

const (
	Legend GraphOpt = iota
	Term
	Strat
	Short
)

// Grapher generates graphs in GraphViz dot format from a dump
type Grapher struct {
	gopt       GraphOpt
	seenTerms  map[int32]struct{}
	seenStrats map[int32]struct{}
}

// MakeGrapher initializes a grapher.
func MakeGrapher(gopt GraphOpt) Grapher {
	return Grapher{gopt, make(map[int32]struct{}), make(map[int32]struct{})}
}

// Clean removes the grapher cache and returns the grapher to its original state.
func (g *Grapher) Clean() {
	g.seenTerms = make(map[int32]struct{})
	g.seenStrats = make(map[int32]struct{})
}

func (g *Grapher) generateLegend(writer io.Writer, dump smcdump.SmcDump) {
	io.WriteString(writer, "\n\tlegendTerms "+legendBegin)

	for key, _ := range g.seenTerms {
		fmt.Fprintf(writer, legendElem, key, util.CleanHtmlString(dump.GetString(key)))
	}

	io.WriteString(writer, legendEnd+"\n\tlegendStrats "+legendBegin)

	for key, _ := range g.seenStrats {
		fmt.Fprintf(writer, legendElem, key, util.CleanHtmlString(dump.GetString(key)))
	}

	io.WriteString(writer, legendEnd)
}

// GenerateDot generates a graph in dot format for the system automaton.
func (g *Grapher) GenerateDot(writer io.Writer, dump smcdump.SmcDump) {
	g.Clean()

	io.WriteString(writer, "digraph {\n")

	var nrStates = dump.NumberOfStates()

	for i := 0; i < nrStates; i++ {
		g.graphState(writer, dump, int32(i), -1)
	}

	if g.gopt == Legend {
		g.generateLegend(writer, dump)
	}

	io.WriteString(writer, "}\n")
}

// GenerateCounterDot generates a graph in dot format for the counterexample.
func (g *Grapher) GenerateCounterDot(writer io.Writer, dump smcdump.SmcDump) {
	g.Clean()

	io.WriteString(writer, "digraph {\n")

	var path = dump.Path()
	var cycle = dump.Cycle()

	var pathLength = len(path)
	var cycleLength = len(cycle)

	for index, nodeNr := range dump.Path() {
		var targetNr int32

		if index+1 == pathLength {
			targetNr = cycle[0]
		} else {
			targetNr = path[index+1]
		}

		g.graphState(writer, dump, nodeNr, targetNr)
	}

	for index, nodeNr := range dump.Cycle() {
		var targetNr int32

		if index+1 == cycleLength {
			targetNr = cycle[0]
		} else {
			targetNr = cycle[index+1]
		}

		g.graphState(writer, dump, nodeNr, targetNr)
	}

	if g.gopt == Legend {
		g.generateLegend(writer, dump)
	}

	io.WriteString(writer, "}\n")
}

func (g *Grapher) graphState(writer io.Writer, dump smcdump.SmcDump, stateNr, targetNr int32) {
	var state = dump.State(stateNr)

	fmt.Fprintf(writer, "\t%d [label=\"", stateNr)

	switch g.gopt {
	case Legend:
		g.seenTerms[state.Term] = struct{}{}
		g.seenStrats[state.Strategy] = struct{}{}
		fallthrough
	case Short:
		fmt.Fprintf(writer, "(%d, %d)\"", state.Term, state.Strategy)
	case Term:
		io.WriteString(writer, util.CleanEscapeString(dump.GetString(state.Term))+"\"")
	case Strat:
		io.WriteString(writer, util.CleanEscapeString(dump.GetString(state.Strategy))+"\"")
	}

	if state.Solution {
		io.WriteString(writer, ", style = filled")
	}

	io.WriteString(writer, "];\n")

	for _, tr := range state.Successors {
		if targetNr < 0 || tr.Target == targetNr {
			var label string

			switch tr.TrType {
				case smcdump.Idle     : label = "idle"
				case smcdump.Rule     : label = dump.GetString(tr.Label)
				case smcdump.Opaque   : label = "opaque(" + dump.GetString(tr.Label) + ")"
			}

			if len(label) > 20 {
				label = label[0:20] + "..."
			}

			fmt.Fprintf(writer, "\t%d -> %d [label=\"%s\"];\n", stateNr, tr.Target, label)
		}
	}
}

// GeneratePdf is a utility function to directly generate a PDF from
// a graph description using the dot command.
func GeneratePdf(writer io.WriteCloser, dotGenerator func(w io.Writer)) {
	var cmd = exec.Command("dot", "-Tpdf")

	if cmd == nil {
		log.Fatal("Exec")
	}

	// The resulting PDF will be written in writer
	cmd.Stdout = writer
	stdin, err := cmd.StdinPipe()

	if stdin == nil {
		log.Fatal(err)
	}

	cmd.Start()

	// Writes the graph spec to the dot program standard input
	dotGenerator(stdin)
	stdin.Close()

	cmd.Wait()

	writer.Close()
}
