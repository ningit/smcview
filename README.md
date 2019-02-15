## Graphical interface and graph generator for the Maude strategy model checker

For model checking using the graphical user interface, the Maude interpreter version with full strategy support and including the strategy model checker is required. It should be made available to *smcview* as a program called `maude` in the same directory, in the current working directory or in the system path, but its location can also be specified by a program option. All options are described when calling `smcview -help`.

Compiled binaries of this program are available at the *[Releases](https://github.com/ningit/smcview/releases)* section, and the Maude interpreter version can be downloaded from [maude.sip.ucm.es/strategies](http://maude.sip.ucm.es/strategies/#downloads).

### Build

Execute the commands `go generate` and `go build`. The static resources in the `data` directory are packed in the binary, but `go build -tags dev` can be used to read them from disk instead.
