package visibility

import (
	"fmt"
	"net/http"
)

// PublicStruct should be public (not private)
type PublicStruct struct {
	PublicField    string
	privateField   int
	XMLTag         string `xml:"xml_tag"`
	httpClient     *http.Client
	URLPath        string
	_hiddenField   string
	_PublicExport  string // Leading underscore but capitalized
}

// privateStruct should be private
type privateStruct struct {
	PublicField   string
	privateField  int
}

// Interface definitions
type PublicInterface interface {
	PublicMethod() string
	privateMethod() error // Interface methods follow same rules
}

type privateInterface interface {
	SomeMethod() bool
}

// Constants and variables
const (
	PublicConstant  = "visible"
	privateConstant = "hidden"
	MaxLimit        = 1000
	defaultTimeout  = 30
)

var (
	PublicVariable  = "visible"
	privateVariable = "hidden"
	HTTPSEnabled    = true
	xmlParser       = nil
	_global         = "underscore"
	_ExportGlobal   = "underscore but exported"
)

// Functions
func PublicFunction() string {
	return "public"
}

func privateFunction() string {
	return "private"
}

func HTTPHandler(w http.ResponseWriter, r *http.Request) {
	// Public function with acronyms
}

func jsonParser() interface{} {
	// Private function with acronym
	return nil
}

func XMLProcessor() *PublicStruct {
	// Public function with XML acronym
	return &PublicStruct{}
}

func urlBuilder() string {
	// Private function with url
	return ""
}

// Methods on public type
func (p *PublicStruct) PublicMethod() string {
	return p.PublicField
}

func (p *PublicStruct) privateMethod() error {
	return nil
}

func (p *PublicStruct) HTTPGet() (*http.Response, error) {
	// Public method with HTTP acronym
	return p.httpClient.Get(p.URLPath)
}

func (p *PublicStruct) xmlEncode() []byte {
	// Private method with xml
	return nil
}

// Methods on private type
func (p *privateStruct) PublicMethod() string {
	// Public method on private type - method itself is public, type is private
	return p.PublicField
}

func (p *privateStruct) privateMethod() error {
	// Private method on private type
	return nil
}

// Edge cases
func _privateUnderscore() {
	// Leading underscore - should be private
}

func _PublicUnderscore() {
	// Leading underscore but capitalized - should be public by Go rules
}

// Type aliases and definitions
type PublicAlias = string
type privateAlias = int

type HTTPClient PublicStruct    // Public type based on another type
type xmlDocument privateStruct  // Private type based on another type

// Receiver functions with different patterns
func (h HTTPClient) Process() error {
	return nil
}

func (x xmlDocument) parse() interface{} {
	return nil
}

// Init function - special case, should be considered public despite lowercase
func init() {
	// Special init function
	PublicVariable = "initialized"
}

// Main function - special case, should be considered public despite lowercase
func main() {
	fmt.Println("Main function")
}