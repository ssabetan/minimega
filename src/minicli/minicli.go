// minicli is a command line interface backend for minimega. It allows
// registering handlers and function callbacks for command line arguments that
// match defined patterns.
//
// minicli also supports multiple output rendering modes and stream and tabular
// compression.
package minicli

// Output modes
const (
	MODE_NORMAL = iota
	MODE_JSON
	MODE_QUIET
)

var (
	compress bool // compress output
	tabular  bool // tabularize output
	mode     int  // output mode
)

type Command struct {
	Original string            // original raw input
	Args     map[string]string // map of arguments
}

type Responses []*Response

// A response as populated by handler functions.
type Response struct {
	Host     string      // Host this response was created on
	Response string      // Simple response
	Header   []string    // Optional header. If set, will be used for both Response and Tabular data.
	Tabular  [][]string  // Optional tabular data. If set, Response will be ignored
	Error    string      // Because you can't gob/json encode an error type
	Data     interface{} // Optional user data
}

// Register a new API based on pattern. Patterns consist of required text, required and optional fields, multiple choice arguments, and variable number of arguments. The pattern syntax is as follows:
// <foo> 	a required string, returned in the arg map with key "foo"
// <foo bar> 	a required string, still returned in the arg map with key "foo".
//	 	The extra is just documentation
// foo bar	literal required text, as in "capture netflow <foo>"
// [foo]	optional string, returned in the arg map with key "foo". There can
// 		be only one optional arg and it must be at the end of the pattern.
// <foo,bar>	a required multiple choice argument. Returned as whichever
// 		choice is made in the argmap (the argmap key is simply created).
// [foo,bar]	an optional multiple choice argument.
// <foo>...	a required list of strings, one or more, with the key "foo" in
// 		the argmap
// [foo]...	an optional list of strings, zero or more, with the key "foo" in
// 		the argmap. This is the only way to support multiple optional fields.
func Register(pattern string, handler func(*Command) *Responses) {

}

// Process raw input text. An error is returned if parsing the input text
// failed.
func ProcessString(input string) (*Responses, error) {
	c, err := CompileCommand(input)
	if err != nil {
		return nil, err
	}
	return ProcessCommand(c), nil
}

// Process a prepopulated Command
func ProcessCommand(c *Command) *Responses {

}

// Create a command from raw input text. An error is returned if parsing the
// input text failed.
func CompileCommand(input string) (*Command, error) {

}

// List installed patterns and handlers
func Handlers() string {

}

// Enable or disable response compression
func CompressOutput(compress bool) {

}

// Enable or disable tabular aggregation
func TabularOutput(tabular bool) {

}

// Return a string representation using the current output mode
// using the %v verb in pkg fmt
func (r Responses) String() {

}

// Return a verbose output representation for use with the %#v verb in pkg fmt
func (r Responses) GoString() {

}

// Return any errors contained in the responses, or nil. If any responses have
// errors, the returned slice will be padded with nil errors to align the error
// with the response.
func (r Responses) Errors() []error {

}

// Set the output mode for String()
func OutputMode(mode int) {

}