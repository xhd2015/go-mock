package gen

import (
	"fmt"
	"reflect"
	"strings"
)

type EnvContext interface {
	Get(varName string) string
	Gets(varName string) (val string, ok bool)
}

type VarMap map[string]string

func (c VarMap) Get(varName string) string {
	return c[varName]
}
func (c VarMap) Gets(varName string) (val string, ok bool) {
	val, ok = c[varName]
	return
}

type Statement interface {
	Eval(varMap EnvContext) []string
}

type Statements struct {
	list []interface{}
}

type StringStatement string
type IndentStatment struct {
	Indent    string
	Statments Statements
}
type PreStatement struct {
	statments *Statements
}

// Eval implements pre
func (c *PreStatement) Eval(varMap EnvContext) []string {
	list := make([]string, 0, len(c.statments.list))
	for _, s := range c.statments.list {
		traverseStatements(s, varMap, func(e Statement) {
			list = append(list, e.Eval(varMap)...)
		})
	}
	return []string{strings.Join(list, "")}
}

var _ Statement = ((*IndentStatment)(nil))

var _ Statement = ((*PreStatement)(nil))

func (c *IndentStatment) Eval(varMap EnvContext) []string {
	if c.Indent == "" {
		return c.Statments.Eval(varMap)
	}
	list := c.Statments.Eval(varMap)
	for i := 0; i < len(list); i++ {
		list[i] = c.Indent + list[i]
	}
	return list
}

type IfStatement struct {
}

type IfBuilder struct {
	t              bool
	statements     Statements
	elseStatements Statements
}

type ElsePhase interface {
	Else(s ...interface{}) Statement
}

var _ Statement = ((*IfBuilder)(nil))
var _ Statement = ((*Statements)(nil))

func If(t bool) *IfBuilder {
	return &IfBuilder{
		t: t,
	}
}
func Pre(s ...interface{}) *PreStatement {
	st := &Statements{}
	st.Append(s...)
	return &PreStatement{
		statments: st,
	}
}
func Group(s ...interface{}) *PreStatement {
	return Pre(s...)
}
func (c *IfBuilder) Then(s ...interface{}) ElsePhase {
	c.statements.Append(s...)
	return c
}
func (c *IfBuilder) Eval(varMap EnvContext) []string {
	if c.t {
		return c.statements.Eval(varMap)
	}
	return c.elseStatements.Eval(varMap)
}
func (c *IfBuilder) Else(s ...interface{}) Statement {
	c.elseStatements.Append(s...)
	return c
}

// TODO: may detect dead loop
func (c *Statements) Append(s ...interface{}) {
	c.list = append(c.list, s...)
}

func (c *Statements) Eval(varMap EnvContext) []string {
	list := make([]string, 0, len(c.list))
	for _, x := range c.list {
		traverseStatements(x, varMap, func(e Statement) {
			list = append(list, e.Eval(varMap)...)
		})
	}
	return list
}

func traverseStatements(x interface{}, varMap EnvContext, fn func(e Statement)) {
	if x == nil {
		panic(fmt.Errorf("statement cannot be nil"))
	}
	switch x := x.(type) {
	case Statement:
		fn(x)
	case string:
		fn(StringStatement(x))
	case []string:
		for _, s := range x {
			fn(StringStatement(s))
		}
	case []interface{}:
		for _, s := range x {
			traverseStatements(s, varMap, fn)
		}
	case Statements:
		for _, s := range x.list {
			traverseStatements(s, varMap, fn)
		}
	case PreStatement:
		// make *PreStatement
		fn(&x)
	default:
		v := reflect.ValueOf(x)
		if v.Kind() == reflect.Slice {
			for i := 0; i < v.Len(); i++ {
				traverseStatements(v.Index(i).Interface(), varMap, fn)
			}
		} else if v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
			if !v.IsNil() {
				traverseStatements(v.Elem().Interface(), varMap, fn)
			}
		}
		panic(fmt.Errorf("unrecognized statment type:%T %v", x, x))
	}
}

func (c Statements) Format(varMap EnvContext, pretty bool, indent string) string {
	list := c.Eval(varMap)
	if !pretty {
		for i := 0; i < len(list); i++ {
			list[i] = strings.TrimSpace(list[i])
			if len(list[i]) > 0 {
				last := list[i][len(list[i])-1:]
				if last != "{" && last != "(" && last != "[" && last != "," && last != ";" {
					list[i] = list[i] + ";"
				}
			}
		}
	} else if indent != "" {
		for i := 0; i < len(list); i++ {
			list[i] = indent + list[i]
		}
	}
	joint := ""
	if pretty {
		joint = "\n"
	}
	return strings.Join(list, joint)
}

func (c StringStatement) Eval(varMap EnvContext) []string {
	if varMap == nil {
		return []string{string(c)}
	}
	s := string(c)
	var list []string
	for {
		const LSPLIT = "__"
		const RSPLIT = "__"
		i := strings.Index(s, LSPLIT)
		j := int(-1)
		if i >= 0 && len(s) > len(LSPLIT) {
			j = strings.Index(s[i+len(LSPLIT):], RSPLIT)
			if j >= 0 {
				j = j + i + len(LSPLIT) + len(RSPLIT)
			}
		}
		// fmt.Printf("search %q,i=%d,j=%d\n", s, i, j)
		if i < 0 || j < 0 {
			if len(list) == 0 {
				return []string{string(c)}
			}
			return []string{strings.Join(list, "") + s}
		}
		if i > 0 {
			list = append(list, s[:i])
		}
		varName := s[i:j]
		// fmt.Printf("varName:%v\n", varName)
		val, ok := varMap.Gets(varName)
		if !ok {
			val = varName
		}
		list = append(list, val)
		s = s[j:]
	}
}

type TemplateBuilder struct {
	pretty     bool
	indent     string
	statements Statements
}

func NewTemplateBuilder() *TemplateBuilder {
	return &TemplateBuilder{
		pretty: true,
	}
}

func (c *TemplateBuilder) Block(s ...interface{}) *TemplateBuilder {
	c.statements.Append(s...)
	return c
}

func Indent(indent string, s ...interface{}) Statement {
	idt := &IndentStatment{
		Indent: indent,
	}
	idt.Statments.Append(s...)
	return idt
}

func (c *TemplateBuilder) If(t bool) *IfBuilder {
	return If(t)
}

func (c *TemplateBuilder) Pretty(pretty bool) *TemplateBuilder {
	c.pretty = pretty
	return c
}

func (c *TemplateBuilder) Indent(indent string) *TemplateBuilder {
	c.indent = indent
	return c
}

func (c *TemplateBuilder) Format(varMap EnvContext) string {
	return c.statements.Format(varMap, c.pretty, c.indent)
}

func (c *TemplateBuilder) Eval(varMap EnvContext) []string {
	return c.statements.Eval(varMap)
}
