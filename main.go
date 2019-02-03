//go:generate go run assets_generate.go

package main

import (
	"flag"
	"fmt"
	"github.com/ningit/smcview/grapher"
	"github.com/ningit/smcview/maude"
	"github.com/ningit/smcview/smcdump"
	"github.com/ningit/smcview/webui"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Strings constants used by the command line interface
const (
	usageLine = "Strategy-aware model checker for Maude -- Graphical interface\nUsage: %s [options] [dumpfile]\n"
	badCommandLine = `Wrong command line syntax. The program must be called
 * without arguments, to starts the web interface, or
 * with a single argument, being the path of an existing model checker dump.
The argument must be provided after all flags. Use -help to get information about them.`
)

func underRoot(rootpath, otherpath string) bool {
	// Paths are assumed to be absolute and cleaned
	return strings.HasPrefix(otherpath, rootpath)
}

func processDump(fpath, graphMode string, toPdf bool) {
	var dump, err = smcdump.Read(fpath)
	if dump == nil {
		log.Fatal(err)
	}

	// Shows the basic information about the dump
	fmt.Printf("     LTL formula:  %s\n", dump.LtlFormula())
	fmt.Printf("    Initial term:  %s\n", dump.InitialTerm())
	fmt.Printf("Number of states:  %d\n", dump.NumberOfStates())
	fmt.Printf("           Holds:  %v\n", dump.PropertyHolds())
	if !dump.PropertyHolds() {
		fmt.Printf("            Path:  %v\n", dump.Path())
		fmt.Printf("           Cycle:  %v\n", dump.Cycle())
	}

	// Parses graph options and constructs a grapher with them
	var graphOpt grapher.GraphOpt

	switch graphMode {
		case "legend" : graphOpt = grapher.Legend
		case "term"   : graphOpt = grapher.Term
		case "strat"  : graphOpt = grapher.Strat
		case "short"  : graphOpt = grapher.Short
		default: fmt.Printf("Unknown graph option '%s'. Graph output will be skipped.\n", graphMode) ; return
	}

	var grph = grapher.MakeGrapher(graphOpt)

	// Path prefix for the generated DOT or PDF files that will be
	// written in the current directory
	currentDirectory, _ := os.Getwd()
	var prefix = filepath.Join(currentDirectory,
			strings.TrimSuffix(filepath.Base(fpath), filepath.Ext(fpath)))

	// If the DOT command is not available PDF will not be generated
	if toPdf {
		if _, err := exec.LookPath("dot"); err != nil {
			log.Println("GraphViz dot command is not available in the path. Source files will be generated instead of PDF.")
			toPdf = false
		}
	}

	// We reject generating PDF for huge graphs because DOT will probably
	// not be able to handle them
	var toPdfAutomaton = toPdf

	if toPdf && dump.NumberOfStates() > 200 {
		log.Println("The automaton graph may be too large for GraphViz. The source file will be generated instead of PDF")
		toPdfAutomaton = false
	}

	var file *os.File

	// Generates the system automaton graph
	if toPdfAutomaton {
		file, _ = os.Create(prefix + "-automaton.pdf")
	} else {
		file, _ = os.Create(prefix + "-automaton.dot")
	}

	if file != nil {
		if toPdfAutomaton {
			grapher.GeneratePdf(file, func(writer io.Writer) { grph.GenerateDot(writer, dump) })
		} else {
			grph.GenerateDot(file, dump)
		}
	}

	// Generates the counterexample trace in case the property does not hold
	if !dump.PropertyHolds() {
		if toPdf {
			file, _ = os.Create(prefix + "-counterexpl.pdf")
		} else {
			file, _ = os.Create(prefix + "-counterexpl.dot")
		}

		if file != nil {
			if toPdf {
				grapher.GeneratePdf(file, func(writer io.Writer) { grph.GenerateCounterDot(writer, dump) })
			} else {
				grph.GenerateCounterDot(file, dump)
			}
		}
	}

	dump.Close()
}

func startServer(port int, verbose bool, address, maudePath, sourcedir, rootdir string) {
	// Tries to find Maude (except if its path was provided) and checks
	// whether the found version supports strategies
	var maudeVersion string

	if maudePath == "" {
		maudePath, maudeVersion = maude.LocateMaude()

		if maudePath == "" {
			log.Print("no strategy-enabled version of Maude was found")
		}

	} else {
		maudeVersion = maude.MaudeVersion(maudePath)

		if !strings.Contains(maudeVersion, "+strat") {
			log.Print("the given Maude path does not point to Maude or to a wrong version '", maudeVersion, "'")
		}
	}

	if verbose {
		println("Maude:", maudePath)
		println("Maude version:", maudeVersion)
	}

	// Sets up the web interface by later fixing the port address and
	// relevant directories
	var srv = webui.InitWebUi(maudePath, assets)
	if srv == nil {
		log.Fatal("the web interface cannot be initializated")
	}

	srv.Port = port
	srv.Address = address

	// The interface access will be confined to this directory if non-empty
	if rootdir != "" {
		fileInfo, _ := os.Stat(rootdir)

		if fileInfo == nil || !fileInfo.IsDir() {
			log.Fatal("wrong root directory (it must be an existing directory)")
		}

		rootdir, _ = filepath.Abs(rootdir)
		srv.RootDir = rootdir
		srv.InitialDir = rootdir
	}

	// The source dir will be used as initial directory for source files.
	// It must be inside the root directory in case it was specified.
	if sourcedir != "" {
		fileInfo, _ := os.Stat(sourcedir)

		if fileInfo == nil || !fileInfo.IsDir() {
			log.Fatal("wrong source directory (it must be an existing directory)")
		}

		sourcedir, _ = filepath.Abs(sourcedir)

		if rootdir != "" && !underRoot(rootdir, sourcedir) {
			log.Fatal("source directory is outside root directory")
		}

		srv.InitialDir = sourcedir
	}

	var addressName = srv.Address

	// More user-friendly name
	if srv.Address == "127.0.0.1" {
		addressName = "localhost"
	}

	fmt.Printf("Listening at http://%s:%d/\n", addressName, srv.Port)

	srv.Start()
}

func main() {
	// Parses command line arguments
	var (
		verbose, graphPdf                                 bool
		port                                              int
		address, maudePath, sourcedir, rootdir, graphMode string
	)

	flag.IntVar(&port, "port", 1234, "server listening `port`")
	flag.StringVar(&address, "address", "127.0.0.1", "server listening `address`")
	flag.BoolVar(&verbose, "verbose", false, "show more information")
	flag.StringVar(&maudePath, "maudecmd", "", "maude executable `path`")
	flag.StringVar(&sourcedir, "sourcedir", "", "initial source `directory`")
	flag.StringVar(&rootdir, "rootdir", "", "restrict access to the filesystem to a given `directory`")
	flag.BoolVar(&graphPdf, "pdf", false, "generate PDF instead of DOT files (GraphViz is required)")
	flag.StringVar(&graphMode, "gopt", "legend", "choose how state labels are printed in DOT graphs (among legend, term, strat, short)")

	// Usage information when -help is requested
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), usageLine, os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if nargs := flag.NArg(); nargs > 1 {
		// There are too much command line parameter, an explanation is printed
		println(badCommandLine)
		return
	} else if nargs == 1 {
		// When a single argument is provided, it must be the path of a dump for
		// which the automaton and counterexample graph will be generated
		processDump(flag.Arg(0), graphMode, graphPdf)
		return
	}

	// Otherwise, we start the server
	startServer(port, verbose, address, maudePath, sourcedir, rootdir)
}
