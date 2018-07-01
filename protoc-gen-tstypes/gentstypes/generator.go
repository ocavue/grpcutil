package gentstypes

// TODO: add nested messages support
// TODO: add nested enum support

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/sprig"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	"github.com/jhump/protoreflect/desc"
)

const indent = "  "

type Parameters struct {
	AsyncIterators        bool
	DeclareNamespace      bool
	OutputNamePattern     string
	DumpRequestDescriptor bool
	EnumsAsInt            bool
	Verbose               int
	// TODO: allow template specification?
}

type Generator struct {
	*bytes.Buffer
	indent   string
	Request  *plugin.CodeGeneratorRequest
	Response *plugin.CodeGeneratorResponse
}

type OutputNameContext struct {
	Dir        string
	BaseName   string
	Descriptor *desc.FileDescriptor
	Request    *plugin.CodeGeneratorRequest
}

func New() *Generator {
	return &Generator{
		Buffer:   new(bytes.Buffer),
		Request:  new(plugin.CodeGeneratorRequest),
		Response: new(plugin.CodeGeneratorResponse),
	}
}

func (g *Generator) incIndent() {
	g.indent += indent
}

func (g *Generator) decIndent() {
	g.indent = string(g.indent[:len(g.indent)-len(indent)])
}

func (g *Generator) W(s string) {
	g.w(s)
	g.Buffer.WriteString("\n")
}

func (g *Generator) w(s string) {
	g.Buffer.WriteString(g.indent)
	g.Buffer.WriteString(s)
}

var s = &spew.ConfigState{
	Indent:                  " ",
	DisableMethods:          true,
	SortKeys:                true,
	SpewKeys:                true,
	MaxDepth:                12,
	DisablePointerAddresses: true,
	DisableCapacities:       true,
}

func genName(r *plugin.CodeGeneratorRequest, f *desc.FileDescriptor, outPattern string) string {
	// TODO: consider using go_package if present?

	n := filepath.Base(f.GetName())
	if strings.HasSuffix(n, ".proto") {
		n = n[:len(n)-len(".proto")]
	}
	ctx := &OutputNameContext{
		Dir:        filepath.Dir(f.GetName()),
		BaseName:   n,
		Descriptor: f,
		Request:    r,
	}
	var t = template.Must(template.New("gentstypes/generator.go:genName").Funcs(sprig.FuncMap()).Parse(outPattern))
	buf := new(bytes.Buffer)
	if err := t.Execute(buf, ctx); err != nil {
		log.Fatalln("issue rendering template:", err)
	}
	return buf.String()
}

func (g *Generator) GenerateAllFiles(params *Parameters) {
	g.W("// Code generated by protoc-gen-tstypes. DO NOT EDIT.\n")
	files, err := desc.CreateFileDescriptors(g.Request.ProtoFile)
	if params.DumpRequestDescriptor {
		s.Fdump(os.Stderr, g.Request)
	}
	if err != nil {
		log.Fatal(err)
	}
	names := []string{}
	for _, fname := range g.Request.FileToGenerate {
		names = append(names, fname)
	}
	sort.Strings(names)
	for _, n := range names {
		f := files[n]
		if params.DeclareNamespace {
			g.W(fmt.Sprintf("declare namespace %s {\n", f.GetPackage()))
			g.incIndent()
		}
		g.generate(f, params)
		if params.DeclareNamespace {
			g.decIndent()
			g.W("}\n")
		}
		n := genName(g.Request, f, params.OutputNamePattern)
		if params.Verbose > 0 {
			fmt.Fprintln(os.Stderr, "generating", n)
		}
		g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
			Name:    proto.String(n),
			Content: proto.String(g.String()),
		})
		g.Buffer.Reset()
	}
}

func (g *Generator) generate(f *desc.FileDescriptor, params *Parameters) {
	// TODO: consider best order
	g.generateEnums(f.GetEnumTypes(), params)
	g.generateMessages(f.GetMessageTypes(), params)
	g.generateServices(f.GetServices(), params)
}

func (g *Generator) generateMessages(messages []*desc.MessageDescriptor, params *Parameters) {
	for _, m := range messages {
		g.generateMessage(m, params)
	}
}
func (g *Generator) generateEnums(enums []*desc.EnumDescriptor, params *Parameters) {
	for _, e := range enums {
		g.generateEnum(e, params)
	}
}
func (g *Generator) generateServices(services []*desc.ServiceDescriptor, params *Parameters) {
	for _, e := range services {
		g.generateService(e, params)
	}
}

func (g *Generator) generateMessage(m *desc.MessageDescriptor, params *Parameters) {
	// TODO: namespace messages?
	for _, e := range m.GetNestedEnumTypes() {
		g.generateEnum(e, params)
	}
	g.W(fmt.Sprintf("export interface %s {", m.GetName()))
	for _, f := range m.GetFields() {
		g.W(fmt.Sprintf(indent+"%s?: %s;", f.GetName(), fieldType(f)))
	}
	g.W("}\n")
}

func fieldType(f *desc.FieldDescriptor) string {
	t := rawFieldType(f)
	if f.IsMap() {
		return fmt.Sprintf("{ [key: %s]: %s }", rawFieldType(f.GetMapKeyType()), rawFieldType(f.GetMapValueType()))
	}
	if f.IsRepeated() {
		return fmt.Sprintf("Array<%s>", t)
	}
	return t
}

func rawFieldType(f *desc.FieldDescriptor) string {
	switch f.GetType() {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_INT64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_UINT64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_INT32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_FIXED64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_FIXED32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_UINT32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		return "number"
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		return "boolean"
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		return "string"
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		return "Uint8Array"
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		return f.GetEnumType().GetName()
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		t := f.GetMessageType()
		if t.GetFile().GetPackage() != f.GetFile().GetPackage() {
			return t.GetFullyQualifiedName()
		}
		return t.GetName()
	}
	return "any /*unknown*/"
}

func (g *Generator) generateEnum(e *desc.EnumDescriptor, params *Parameters) {
	g.W(fmt.Sprintf("export enum %s {", e.GetName()))
	for _, v := range e.GetValues() {
		if params.EnumsAsInt {
			g.W(fmt.Sprintf("    %s = %v,", v.GetName(), v.GetNumber()))
		} else {
			g.W(fmt.Sprintf("    %s = \"%v\",", v.GetName(), v.GetName()))
		}
	}
	g.W("}")
}

func (g *Generator) generateService(service *desc.ServiceDescriptor, params *Parameters) {
	g.W(fmt.Sprintf("export interface %sService {", service.GetName()))
	g.incIndent()
	g.generateServiceMethods(service, params)
	g.decIndent()
	g.W(fmt.Sprintf("}"))
}

func (g *Generator) generateServiceMethods(service *desc.ServiceDescriptor, params *Parameters) {
	for _, m := range service.GetMethods() {
		g.generateServiceMethod(m, params)
	}
}
func (g *Generator) generateServiceMethod(method *desc.MethodDescriptor, params *Parameters) {
	i := method.GetInputType().GetName()
	o := method.GetOutputType().GetName()
	if params.AsyncIterators {
		if method.IsServerStreaming() {
			o = fmt.Sprintf("AsyncIterator<%s>", o)
		}
		if method.IsClientStreaming() {
			i = fmt.Sprintf("AsyncIterator<%s>", i)
		}
		g.W(fmt.Sprintf("%s: (r:%s) => %s;", method.GetName(), i, o))
	} else {
		ss, cs := method.IsServerStreaming(), method.IsClientStreaming()
		if !(ss || cs) {
			g.W(fmt.Sprintf("%s: (r:%s) => %s;", method.GetName(), i, o))
			return
		}
		if !cs {
			g.W(fmt.Sprintf("%s: (r:%s, cb:(a:{value: %s, done: boolean}) => void) => void;", method.GetName(), i, o))
			return
		}
		if !ss {
			g.W(fmt.Sprintf("%s: (r:() => {value: %s, done: boolean}) => %s;", method.GetName(), i, o))
			return
		}
		g.W(fmt.Sprintf("%s: (r:() => {value: %s, done: boolean}, cb:(a:{value: %s, done: boolean}) => void) => void;", method.GetName(), i, o))
	}
}
