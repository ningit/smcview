// Package smcdump allows reading reports from the Maude strategy-aware model checker.
package smcdump

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// A dump from the Maude strategy-aware model checker.
type SmcDump interface {
	// PropertyHolds indicates whether the model-checked property holds.
	PropertyHolds() bool
	// NumberOfStates is the total number of explored states in the system automaton.
	NumberOfStates() int
	// InitialTerm is the initial term.
	InitialTerm() string
	// LtlFormula is the LTL formula.
	LtlFormula() string

	// Path is the counterexample path.
	Path() []int32
	// Cycle is the counterexample cycle.
	Cycle() []int32

	// State let obtain detailed information of a given state.
	State(int32) State
	// GetString returns the string identified by the given number.
	GetString(int32) string

	// Close closes and frees the SmcDump resources.
	Close()
}

// TransitionType indicates the type of a system-automaton transition.
type TransitionType int

const (
	Idle TransitionType = iota
	Rule
	Opaque
)

// Transition represents a transition of the system automaton.
type Transition struct {
	Target int32
	Label  int32

	TrType TransitionType
}


var (
	// Starting bytes of any dump file
	header       = []byte("msmc-output")
	// HasSignature is assumed to be executed many times, and this will
	// mitigate the cost of memory management
	headerBuffer = make([]byte, 11)
)


// HasSignature tries to detect, by reading its first bytes, if the given
// file is a dump produced by the model checker.
func HasSignature(path string) bool {
	file, err := os.Open(path)

	if err != nil {
		return false
	}

	defer file.Close()

	file.Read(headerBuffer)

	return bytes.Equal(headerBuffer, header)
}

// States describes a system automaton state.
type State struct {
	Successors []Transition
	Solution   bool
	Term       int32
	Strategy   int32
}

// Actual implementation of the SmcDump that reads the states and
// strings directly from the file.
type smcdump struct {
	path  []int32
	cycle []int32

	initialTerm string
	ltlFormula  string

	statesIndex  []int32
	stringsIndex []int32

	file *os.File
}

func (d *smcdump) LtlFormula() string {
	return d.ltlFormula
}

func (d *smcdump) InitialTerm() string {
	return d.initialTerm
}

func (d *smcdump) Path() []int32 {
	return d.path
}

func (d *smcdump) Cycle() []int32 {
	return d.cycle
}

func (d *smcdump) GetString(stringNr int32) string {
	var stringLength = d.stringsIndex[stringNr+1] - d.stringsIndex[stringNr]
	var text = make([]byte, stringLength)

	d.file.ReadAt(text, int64(d.stringsIndex[stringNr]))

	return string(text)
}

func (d *smcdump) NumberOfStates() int {
	return len(d.statesIndex)
}

func (d *smcdump) PropertyHolds() bool {
	return len(d.cycle) == 0
}

func (d *smcdump) State(stateNr int32) State {
	var state = State{}
	var tmp = make([]byte, 1)

	// Term and strategy indices
	d.file.Seek(int64(d.statesIndex[stateNr]), 0)
	binary.Read(d.file, binary.LittleEndian, &state.Term)
	binary.Read(d.file, binary.LittleEndian, &state.Strategy)

	// Whether the state contains a solution
	d.file.Read(tmp)
	state.Solution = tmp[0] != 0

	var nrSuccessors int32
	binary.Read(d.file, binary.LittleEndian, &nrSuccessors)

	state.Successors = make([]Transition, nrSuccessors)

	for i := int32(0); i < nrSuccessors; i++ {
		binary.Read(d.file, binary.LittleEndian, &state.Successors[i].Target)

		d.file.Read(tmp)
		state.Successors[i].TrType = TransitionType(tmp[0])

		if state.Successors[i].TrType == Rule || state.Successors[i].TrType == Opaque {
			binary.Read(d.file, binary.LittleEndian, &state.Successors[i].Label)
		}
	}

	return state
}

func (d *smcdump) Close() {
	d.file.Close()
}

func readArray(array []int32, reader io.Reader) {

	for i := 0; i < len(array); i++ {
		binary.Read(reader, binary.LittleEndian, &array[i])
	}
}

// Read reads a Maude strategy-aware model checker dump from the file in path.
func Read(path string) (SmcDump, error) {
	var dump smcdump

	file, err := os.Open(path)

	if file == nil {
		return nil, err
	}

	var reader = bufio.NewReaderSize(file, 1024)

	// Checks that the initial mark is present
	var initialMark = make([]byte, 11)
	reader.Read(initialMark)

	if !bytes.Equal(initialMark, header) {
		return nil, errors.New("bad format (no initial mark)")
	}

	// Checks that the version is correct
	version, _ := reader.ReadByte()

	if version != 0 {
		return nil, errors.New("bad format (bad version)")
	}

	dump.initialTerm, _ = reader.ReadString(0)
	dump.ltlFormula, _ = reader.ReadString(0)

	// Removes the zero at the end of the strings
	dump.initialTerm = dump.initialTerm[:len(dump.initialTerm)-1]
	dump.ltlFormula = dump.ltlFormula[:len(dump.ltlFormula)-1]

	// Reads the byte that indicates whether the property holds
	version, _ = reader.ReadByte()
	var propertyHolds = version == 0

	// The number of states
	var numberOfStates int32
	binary.Read(reader, binary.LittleEndian, &numberOfStates)

	// Only if the property does not hold, the path and the cycle are
	// written in the dump
	if !propertyHolds {
		var listSize int32

		// Reads the path
		binary.Read(reader, binary.LittleEndian, &listSize)
		dump.path = make([]int32, listSize)
		readArray(dump.path, reader)

		// Reads the cycle
		binary.Read(reader, binary.LittleEndian, &listSize)
		dump.cycle = make([]int32, listSize)
		readArray(dump.cycle, reader)
	}

	// The table that translates state indices to file offsets
	// where they are described in the dump.
	dump.statesIndex = make([]int32, numberOfStates)
	readArray(dump.statesIndex, reader)

	// The strings table is just after the states enumeration and
	// it is also copied in memory.
	var stringsTableOffset int32
	binary.Read(reader, binary.LittleEndian, &stringsTableOffset)
	// This breaks the reader, but we do not need it yet
	file.Seek(int64(stringsTableOffset), 0)
	var stringsTableSize int32
	binary.Read(file, binary.LittleEndian, &stringsTableSize)

	dump.stringsIndex = make([]int32, stringsTableSize+1)
	readArray(dump.stringsIndex, file)

	// The file remains open because states and strings are directly
	// read from it
	dump.file = file

	return &dump, nil
}
