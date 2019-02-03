// Package server provides an web-based interface for the model checker
package webui

import (
	"context"
	"encoding/json"
	"github.com/ningit/smcview/grapher"
	"github.com/ningit/smcview/maude"
	"github.com/ningit/smcview/smcdump"
	"github.com/ningit/smcview/util"
	"github.com/shurcooL/httpfs/html/vfstemplate"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// sessionStatus represents the status of a session in the web interface.
type sessionStatus int

const (
	blank sessionStatus = iota
	validModule
	fileLoaded // but invalid module
	waitingAnswer
	completed
)

type inputData struct {
	File        string
	Module      string
	InitialTerm string
	LtlFormula  string
	Strategy    string
	Opaques     string
	StartTime   time.Time
}

type mcSession struct {
	interpreter *maude.Client
	status      sessionStatus
	// Name of the generated model checker dump file
	dumpfile    string
	// Metadata to inform while waiting for the model checker
	inputData   inputData
	waitChannel chan struct{}
}

type WebUi struct {
	instance http.Server
	assets   http.FileSystem
	sessions mcSession
	viewTmpl *template.Template
	waitTmpl *template.Template
	// Temporary directory path for auxiliary files
	tempDir  string
	// Port is the listening port
	Port int
	// Address is the listening address
	Address string
	// RootDir is the base of all files and directories the server can access to
	RootDir string
	// InitialDir is the initial directory for finding source files
	InitialDir string
}

func InitWebUi(maudePath string, assets http.FileSystem) *WebUi {
	// Loads HTML templates
	viewTmpl, err := vfstemplate.ParseFiles(assets, nil, "result.htm")
	if viewTmpl == nil {
		log.Fatal(err)
	}

	waitTmpl, err := vfstemplate.ParseFiles(assets, nil, "wait.htm")
	if viewTmpl == nil {
		log.Fatal(err)
	}

	// Creates a temporary directory for auxiliary files
	tempDir, err := ioutil.TempDir("", "maude-smc")
	if err != nil {
		log.Fatal(err)
	}

	workingDir, _ := os.Getwd()

	// Inits Maude and sets the dump path inside the temporary directory
	var maudec = maude.InitMaude(maudePath)
	maudec.SetSmcOutput(filepath.Join(tempDir, "0"))

	var webui = &WebUi{
		assets:     assets,
		sessions:   mcSession{
			interpreter: maudec,
			status: blank,
		},
		viewTmpl:   viewTmpl,
		waitTmpl:   waitTmpl,
		tempDir:    tempDir,
		Port:       1234,
		RootDir:    "",
		InitialDir: workingDir,
	}

	webui.instance.Handler = webui

	return webui
}

func (s *WebUi) Start() {
	var portNumber = strconv.FormatInt(int64(s.Port), 10)
	s.instance.Addr = s.Address + ":" + portNumber

	// Opens a browser window
	time.AfterFunc(time.Second, func() {
		openBrowser("http://localhost:" + portNumber)
	})

	// Captures ^C for to shut down the server
	var stopChan = make(chan os.Signal)
	signal.Notify(stopChan, os.Interrupt)

	go func() {
		if err := s.instance.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal("Cannot start server: ", err)
		}
	}()

	// Waits from a signal in the stop channel to shutdown the server
	<-stopChan
	signal.Reset(os.Interrupt)
	println("\nShutting down server...")
	s.instance.Shutdown(context.Background())
	os.RemoveAll(s.tempDir)
}

// These structures are used to instante HTML templates
// that show the model checker results.
type resultData struct {
	Initial        string
	Formula        string
	NumberOfStates int
	Holds          bool
	Path           []int32
	Cycle          []int32
	States         map[int32]stateData
}

type stateData struct {
	Solution    bool
	Term        string
	Strategy    string
	Transitions []transitionData
}

type transitionData struct {
	Target int32
	Label  string
	Type   int
}

// collectStates collect all states occurring in given path in form
// of stateData in the stateMap table.
func collectStates(stateMap map[int32]stateData, path []int32, dump smcdump.SmcDump) {
	for _, stateNr := range path {
		if _, seen := stateMap[stateNr]; !seen {
			var state = dump.State(stateNr)
			var transitions = make([]transitionData, len(state.Successors))

			for i, tr := range state.Successors {
				transitions[i] = transitionData{
					tr.Target,
					dump.GetString(tr.Label),
					int(tr.TrType),
				}
			}

			stateMap[stateNr] = stateData{
				state.Solution,
				util.CleanString(dump.GetString(state.Term)),
				util.CleanString(dump.GetString(state.Strategy)),
				transitions,
			}
		}
	}
}

// translatePath translates a path from the web side to a path in the host
// machine.
func (s *WebUi) translatePath(url string) string {

	if url == "" {
		return ""
	} else if strings.HasPrefix(url, "tmp:") && !strings.ContainsAny(url, "\\/") {
		// URL for temporal files generated by server operations
		return filepath.Join(s.tempDir, url[4:])
	} else {
		nativeUrl, _ := s.web2NativeUrl(url)
		return  nativeUrl
	}
}

func (s *WebUi) handleView(writer http.ResponseWriter, request *http.Request) {

	var givendump = request.FormValue("dumpfile")
	var hostpath = s.translatePath(givendump)

	if hostpath == "" {
		http.Error(writer, "Not found", 404)
		return
	}

	s.sessions.dumpfile = hostpath

	dump, _ := smcdump.Read(hostpath)
	if dump == nil {
		http.Error(writer, "The given file \""+givendump+"\" is not a valid dump.", 400)
		return
	}

	var stateMap = make(map[int32]stateData)
	collectStates(stateMap, dump.Path(), dump)
	collectStates(stateMap, dump.Cycle(), dump)

	var resultdata = resultData{
		util.CleanString(dump.InitialTerm()),
		util.CleanString(dump.LtlFormula()),
		dump.NumberOfStates(),
		dump.PropertyHolds(),
		dump.Path(),
		dump.Cycle(),
		stateMap,
	}

	err := s.viewTmpl.Execute(writer, resultdata)

	if err != nil {
		log.Print(err)
	}
}

func (s *WebUi) handleLs(writer http.ResponseWriter, request *http.Request) {
	var (
		dir  = request.FormValue("url")
		mode = request.FormValue("mode")
	)

	// Are we looking for dumps or for source files?
	var dump = false
	if mode == "dump" {
		dump = true
	} else if mode != "source" {
		http.Error(writer, "Bad request", 400)
		return
	}

	// The path in the host machine
	var hostdir string
	// Special directories are handled in a custom OS-dependent function
	// (only used in Windows to allow access to all available volumes)
	var isSpecial bool

	// We return separate lists of directories and files
	var dirs, files []string

	// An empty directory means the initial directory fixed from the
	// command line or by default. Path pointing to files are admitted
	// and its directory is used instead.
	if dir == "" {
		hostdir = s.InitialDir
		dir = s.native2WebUrl(hostdir)
		isSpecial = false
	} else {
		hostdir, isSpecial = s.web2NativeUrl(dir)

		if hostdir == "" {
			http.Error(writer, "Bad request", 400)
			return
		}
	}

	if isSpecial {
		dirs, files = s.specialUrl(hostdir)
	} else {
		// Checks that the file exists and it is a directory
		stat, _ := os.Stat(hostdir)

		if stat == nil {
			http.Error(writer, "Bad request", 400)
			return
		} else if !stat.IsDir() {
			dir = path.Dir(dir)
			hostdir = filepath.Dir(hostdir)
		}

		// If the directory is a normal one, we simply read it
		fileList, err := ioutil.ReadDir(hostdir)

		if err != nil {
			http.Error(writer, "Bad request", 400)
			return
		}

		files = make([]string, 0)
		dirs = make([]string, 0)

		for _, file := range fileList {
			var name = file.Name()

			if file.IsDir() && name[0] != '.' {
				dirs = append(dirs, name)
			} else if !dump && filepath.Ext(name) == ".maude" || dump &&
				smcdump.HasSignature(filepath.Join(hostdir, name)) {
				files = append(files, name)
			}
		}
	}

	// The directory listing is passed as JSON to the browser
	writer.Header().Set("Content-Type", "application/json")

	var parentDir = path.Dir(dir)

	if parentDir == "." {
		parentDir = s.native2WebUrl(filepath.Dir(hostdir))
	}

	json.NewEncoder(writer).Encode(struct {
		Dirs   []string `json:"dirs"`
		Files  []string `json:"files"`
		Base   string   `json:"base"`
		Parent string   `json:"parent"`
	}{dirs,
		files,
		dir,
		parentDir,
	})
}

func (s *WebUi) handleSourceInfo(writer http.ResponseWriter, request *http.Request) {
	var givenfile = request.FormValue("url")
	var hostpath = s.translatePath(givenfile)

	if hostpath == "" {
		http.Error(writer, "Bad request", 400)
		return
	}

	s.sessions.interpreter.Start()
	s.sessions.interpreter.Load(hostpath)
	var modules = s.sessions.interpreter.Modules()
	// Source file already loaded, but we do not know if it is valid for model checking
	s.sessions.status = fileLoaded
	s.sessions.inputData.File = givenfile

	writer.Header().Set("Content-Type", "application/json")

	json.NewEncoder(writer).Encode(struct {
		Modules []maude.ModuleInfo `json:"modules"`
	}{modules})
}

// Structures regarding module information to be passed as JSON to the browser.
type maudeOp struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
}

type modInfo struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Params      []string  `json:"params"`
	Valid       bool      `json:"valid"`
	StateSorts  []string  `json:"stateSorts"`
	Strategies  []maudeOp `json:"strategies"`
	AtomicProps []maudeOp `json:"props"`
}

func (s *WebUi) handleModInfo(writer http.ResponseWriter, request *http.Request) {
	var module = request.FormValue("mod")

	if module == "" {
		http.Error(writer, "Bad request", 400)
		return
	}

	// Gets more information from the module signature
	var extModInfo = s.sessions.interpreter.GetModInfo(module)

	var modinfo = modInfo{
		Name:   module,
		Type:   extModInfo.Type,
		Params: extModInfo.Params,
		Valid:  true,
	}

	// Gets the subsorts of the model-checking State sort
	var stateSorts, _ = s.sessions.interpreter.Subsorts("State")

	if stateSorts == nil {
		modinfo.Valid = false
		// If the module is not valid, the list of all sorts is returned
		modinfo.StateSorts = s.sessions.interpreter.Sorts()
	} else {
		modinfo.StateSorts = stateSorts
	}

	// Gets all the strategies in the module
	var strats = s.sessions.interpreter.Strategies()

	modinfo.Strategies = make([]maudeOp, len(strats))

	for i, strat := range strats {
		modinfo.Strategies[i] = maudeOp{strat.Name, strat.Params}
	}

	// Gets all the atomic propositions in the module
	var atomicProps = s.sessions.interpreter.AtomicProps()

	if atomicProps == nil {
		modinfo.Valid = false
		modinfo.AtomicProps = make([]maudeOp, 0)
	} else {
		modinfo.AtomicProps = make([]maudeOp, len(atomicProps))

		for i, prop := range atomicProps {
			modinfo.AtomicProps[i] = maudeOp{prop.Name, prop.Params}
		}
	}

	// Updates the inner session status
	if modinfo.Valid {
		s.sessions.status = validModule
	} else {
		s.sessions.status = fileLoaded
	}

	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(modinfo)
}

// modelCheckResult is used to communicate whether the model checker input data
// is correct, and what was wrong in the negative case.
type modelCheckResult struct {
	// Ok (0) or where the failure is: in the state term (1), in the LTL
	// formula (2), in the strategy (3) or in the opaque strategy list (4).
	Status int `json:"status"`
	// The position of the parsing error.
	Pos    int `json:"pos"`
}

// checkModelInput checks that the model checker input is correct. The LTL formula
// is not checked since the LTL module may not be included when this function is called.
func checkModelInput(maudec *maude.Client, initial, strategy string, opaques []string) (modelCheckResult, bool) {
	// Initial term
	var parse = maudec.Parse(initial, "State")
	if parse.Type != maude.Ok {
		return modelCheckResult{1, parse.Pos}, false
	}

	// Strategy (can be a single name or an expression)
	var strategies = maudec.Strategies()
	var isName = false

	if !strings.Contains(strategy, " ") {
		for _, st := range strategies {
			if st.Name == strategy {
				isName = true
				break
			}
		}
	}

	if !isName {
		parse = maudec.StratParse(strategy)
		if parse.Type != maude.Ok {
			return modelCheckResult{3, parse.Pos}, false
		}
	}

	// Opaque strategies should be defined
	// It does not really matter if they are defined (because unknown
	// names will be ignored) but we still check it to prevent typos.

	opaquesLoop: for index, id := range opaques {
		for _, strat := range strategies {
			if id == strat.Name {
				continue opaquesLoop
			}
		}

		return modelCheckResult{4, index}, false
	}

	return modelCheckResult{0, -1}, isName
}

// removeEmptyString removes empty strings from a slice of strings.
func removeEmptyString(tokens []string) []string {
	var result = make([]string, 0)

	for _, value := range tokens {
		if value != "" {
			result = append(result, value)
		}
	}

	return result
}

func (s *WebUi) handleModelcheck(writer http.ResponseWriter, request *http.Request) {
	var (
		module        = request.FormValue("mod")
		initial       = request.FormValue("initial")
		formula       = request.FormValue("formula")
		strategy      = request.FormValue("strategy")
		opaquesRaw    = request.FormValue("opaques")
		namedStrategy = strategy
	)

	// Some parameters must be non-empty
	if module == "" || initial == "" || formula == "" {
		http.Error(writer, "Bad request", 400)
		return
	}

	// A JSON response will be provided
	writer.Header().Set("Content-Type", "application/json")
	var jsonEncoder = json.NewEncoder(writer)

	var opaques = removeEmptyString(strings.Split(opaquesRaw, " "))

	// Checks that the model cheker input is syntactically correct
	s.sessions.interpreter.Select(module)
	var result, isName = checkModelInput(s.sessions.interpreter, initial, strategy, opaques)

	if result.Status != 0 {
		jsonEncoder.Encode(result)
		return
	}

	// Prepare the opaques as a QidList term
	var opaqueQids = "nil"

	for _, id := range opaques {
		opaqueQids = opaqueQids + " '" + id
	}

	// The input module need not include the strategy model checker
	// module or the LTL module. To execute the model checker, we
	// need to create a new module including it.
	var hasSmc = s.sessions.interpreter.SmcAvailable()

	if !hasSmc || !isName {
		var tmpModule = `smod %SMCVIEW-MODULE is
	protecting ` + module + ` .
	including STRATEGY-MODEL-CHECKER .
`
		if !isName {
			tmpModule += `	strat %smcview-strat @ State .
	sd %smcview-strat := ` + strategy + ` .
`
		}

		tmpModule += "endsm"
		// Possible errors (unbounded variables in strategy expression,
		// for example) are not checked here.
		s.sessions.interpreter.RawInput(tmpModule)
		namedStrategy = "%smcview-strat"
	}

	// Checks the LTL formula (not done before because the input module
	// need not include the LTL module)
	if parse := s.sessions.interpreter.Parse(formula, "Formula"); parse.Type != maude.Ok {
		jsonEncoder.Encode(modelCheckResult{2, parse.Pos})
		return
	}

	// Puts the server in waiting state and stores the input data
	s.sessions.status = waitingAnswer
	s.sessions.inputData = inputData{
		s.sessions.inputData.File,
		module,
		initial,
		formula,
		strategy,
		opaquesRaw,
		time.Now(),
	}

	var mcmd = "modelCheck(" + initial + ", " + formula + ", '" + namedStrategy + ", " + opaqueQids + ")"

	s.sessions.waitChannel = make(chan struct{})

	go func() {
		s.sessions.interpreter.Reduce(mcmd)
		s.sessions.interpreter.Select(module)
		s.sessions.status = completed
		// Closing a channel awakes all its readers
		close(s.sessions.waitChannel)
	}()

	jsonEncoder.Encode(modelCheckResult{0, -1})
}

func (s *WebUi) handleWait(writer http.ResponseWriter, request *http.Request) {
	// If the interface is waiting for the model checker output, listen
	// at the wait channel
	if s.sessions.status == waitingAnswer {
		<-s.sessions.waitChannel
	}

	// For the moment, we do not need Maude after the model checking is done
	s.sessions.status = blank
	s.sessions.interpreter.QuitTimeout(250)

	http.Error(writer, "tmp:0", 200)
}

func (s *WebUi) handleAsk(writer http.ResponseWriter, request *http.Request) {
	var question = request.FormValue("question")

	switch question {
		case "ls"         : s.handleLs(writer, request)
		case "modinfo"    : s.handleModInfo(writer, request)
		case "sourceinfo" : s.handleSourceInfo(writer, request)
		case "modelcheck" : s.handleModelcheck(writer, request)
		case "wait"       : s.handleWait(writer, request)
		default           : http.Error(writer, "Not found", 404)
	}
}

func (s *WebUi) handleMain(writer http.ResponseWriter, request *http.Request) {
	var givendump = request.FormValue("dumpfile")

	// If the dumpfile parameter is given, we show that dumpfile
	if givendump != "" {
		s.handleView(writer, request)
		return
	}

	switch s.sessions.status {
	case waitingAnswer:
		err := s.waitTmpl.Execute(writer, s.sessions.inputData)

		if err != nil {
			log.Fatal(err)
		}
	case completed:
		s.handleView(writer, request)
	default:
		s.serveAsset(writer, request, "select.htm")
	}
}

func (s *WebUi) handleGet(writer http.ResponseWriter, request *http.Request) {
	var which = request.FormValue("file")

	// Gets files from the temporary directory
	switch which {
		case "dump" :
			writer.Header().Set("Content-Disposition", "attachment; filename=\"modelchecker.dump\"")

			if file, err := os.Open(s.sessions.dumpfile); err == nil {
				http.ServeContent(writer, request, "modelchecker.dump", time.Now(), file)
			} else {
				http.Error(writer, "Not found", 404)
			}
		case "autdot" :
			// Generates the automaton graph (it could be cached) in DOT format
			var grph = grapher.MakeGrapher(grapher.Legend)
			var dump, err = smcdump.Read(s.sessions.dumpfile)
			if err != nil {
				http.Error(writer, "Not found", 404) ; return
			}

			var dotfilename = filepath.Join(s.tempDir, "automaton.dot")

			file, err := os.Create(dotfilename)
			if err != nil {
				http.Error(writer, "Not found", 404) ; return
			}

			grph.GenerateDot(file, dump)
			dump.Close()
			file.Close()

			writer.Header().Set("Content-Disposition", "attachment; filename=\"automaton.dot\"")
			http.ServeFile(writer, request, dotfilename)

		default :
			http.Error(writer, "Bad request", 400)
	}
}

func (s *WebUi) handleCancel(writer http.ResponseWriter, request *http.Request) {
	s.sessions.status = blank
	s.sessions.interpreter.Kill()

	// Redirects to the initial screen
	http.Redirect(writer, request, "/", 302)
}

func (s *WebUi) serveAsset(writer http.ResponseWriter, request *http.Request, name string) {
	asset, _ := s.assets.Open(name)
	stat, _ := asset.Stat()

	http.ServeContent(writer, request, name, stat.ModTime(), asset)

	asset.Close()
}

func (s *WebUi) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch res := request.URL.Path; res {
		case "/smcview.css"	: s.serveAsset(writer, request, "smcview.css")
		case "/smcview.js"	: s.serveAsset(writer, request, "smcview.js")
		case "/smcgraph.js"	: s.serveAsset(writer, request, "smcgraph.js")
		case "/"		: s.handleMain(writer, request)
		case "/ask"		: s.handleAsk(writer, request)
		case "/cancel"		: s.handleCancel(writer, request)
		case "/get"		: s.handleGet(writer, request)
		default			: http.Error(writer, "File not found", 404)
	}
}