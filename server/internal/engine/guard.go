package engine

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// EvalContext provides the data a guard expression resolves against.
type EvalContext struct {
	Vars     map[string]any
	Nodes    map[string]map[string]any // nodeID -> outputs
	Extra    map[string]any            // e.g. {"action": "approve"}
	Artifact func(name string) (string, bool)
}

// guardPasses reports whether a transition guard is satisfied. Empty guards
// and the literal "default" always pass; expressions that cannot be parsed
// or resolved evaluate to false (fail-closed routing).
func guardPasses(when string, ctx EvalContext) bool {
	w := strings.TrimSpace(when)
	if w == "" || w == "default" {
		return true
	}
	v, err := evalExpr(w, ctx)
	if err != nil {
		return false
	}
	return truthy(v)
}

func truthy(v any) bool {
	switch n := v.(type) {
	case bool:
		return n
	case float64:
		return n != 0
	case int:
		return n != 0
	case string:
		return n != "" && n != "false"
	case nil:
		return false
	default:
		return true
	}
}

// --- expression evaluator -------------------------------------------------
//
// Grammar (lowest to highest precedence):
//   or   := and ('||' and)*
//   and  := cmp ('&&' cmp)*
//   cmp  := add (('=='|'!='|'<='|'>='|'<'|'>') add)?
//   add  := unary (('+'|'-') unary)*
//   unary:= primary
//   primary := number | string | bool | func | ref | '(' or ')'

func evalExpr(s string, ctx EvalContext) (any, error) {
	p := &parser{toks: tokenize(s), ctx: ctx}
	v, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, fmt.Errorf("unexpected token %q", p.peek().val)
	}
	return v, nil
}

type tokKind int

const (
	tNum tokKind = iota
	tStr
	tIdent
	tOp
	tLParen
	tRParen
	tComma
	tEOF
)

type token struct {
	kind tokKind
	val  string
}

func tokenize(s string) []token {
	var out []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c == '(':
			out = append(out, token{tLParen, "("})
			i++
		case c == ')':
			out = append(out, token{tRParen, ")"})
			i++
		case c == ',':
			out = append(out, token{tComma, ","})
			i++
		case c == '\'' || c == '"':
			q := c
			i++
			start := i
			for i < len(s) && s[i] != q {
				i++
			}
			out = append(out, token{tStr, s[start:i]})
			if i < len(s) {
				i++
			}
		case strings.ContainsRune("=!<>&|+-*/", rune(c)):
			// multi-char operators
			if i+1 < len(s) {
				two := s[i : i+2]
				if two == "==" || two == "!=" || two == "<=" || two == ">=" || two == "&&" || two == "||" {
					out = append(out, token{tOp, two})
					i += 2
					continue
				}
			}
			out = append(out, token{tOp, string(c)})
			i++
		case c >= '0' && c <= '9':
			start := i
			for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
				i++
			}
			out = append(out, token{tNum, s[start:i]})
		default:
			// identifier: letters, digits, _, ., (for refs like nodes.x.outputs.y)
			start := i
			for i < len(s) && isIdentChar(s[i]) {
				i++
			}
			if i == start {
				i++ // skip unknown char
				continue
			}
			out = append(out, token{tIdent, s[start:i]})
		}
	}
	out = append(out, token{tEOF, ""})
	return out
}

func isIdentChar(c byte) bool {
	return c == '_' || c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

type parser struct {
	toks []token
	pos  int
	ctx  EvalContext
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) parseOr() (any, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && p.peek().val == "||" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = truthy(left) || truthy(right)
	}
	return left, nil
}

func (p *parser) parseAnd() (any, error) {
	left, err := p.parseCmp()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && p.peek().val == "&&" {
		p.next()
		right, err := p.parseCmp()
		if err != nil {
			return nil, err
		}
		left = truthy(left) && truthy(right)
	}
	return left, nil
}

func (p *parser) parseCmp() (any, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	if p.peek().kind == tOp {
		switch p.peek().val {
		case "==", "!=", "<", ">", "<=", ">=":
			op := p.next().val
			right, err := p.parseAdd()
			if err != nil {
				return nil, err
			}
			return compare(op, left, right), nil
		}
	}
	return left, nil
}

func (p *parser) parseAdd() (any, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOp && (p.peek().val == "+" || p.peek().val == "-") {
		op := p.next().val
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		lf, lok := toFloat(left)
		rf, rok := toFloat(right)
		if !lok || !rok {
			return nil, fmt.Errorf("non-numeric arithmetic")
		}
		if op == "+" {
			left = lf + rf
		} else {
			left = lf - rf
		}
	}
	return left, nil
}

func (p *parser) parsePrimary() (any, error) {
	t := p.peek()
	switch t.kind {
	case tNum:
		p.next()
		f, _ := strconv.ParseFloat(t.val, 64)
		return f, nil
	case tStr:
		p.next()
		return t.val, nil
	case tLParen:
		p.next()
		v, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tRParen {
			return nil, fmt.Errorf("expected )")
		}
		p.next()
		return v, nil
	case tIdent:
		p.next()
		// function call?
		if p.peek().kind == tLParen {
			return p.parseCall(t.val)
		}
		return p.resolveRef(t.val)
	}
	return nil, fmt.Errorf("unexpected token %q", t.val)
}

func (p *parser) parseCall(name string) (any, error) {
	p.next() // (
	var args []any
	for p.peek().kind != tRParen && p.peek().kind != tEOF {
		v, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		args = append(args, v)
		if p.peek().kind == tComma {
			p.next()
		}
	}
	if p.peek().kind != tRParen {
		return nil, fmt.Errorf("expected ) in call")
	}
	p.next()
	// json("file").path  — capture trailing .path attached as next ident? The
	// tokenizer keeps ".path" separate only if it followed without spaces; for
	// json() we accept an optional following ident chain via the parser caller.
	switch name {
	case "exists":
		if len(args) != 1 {
			return false, nil
		}
		fname, _ := args[0].(string)
		if p.ctx.Artifact == nil {
			return false, nil
		}
		_, ok := p.ctx.Artifact(fname)
		return ok, nil
	case "artifact":
		// artifact("name") → the raw content of a run artifact (empty when
		// absent). Used e.g. by a human_gate body to display a produced file.
		if len(args) != 1 || p.ctx.Artifact == nil {
			return "", nil
		}
		fname, _ := args[0].(string)
		content, _ := p.ctx.Artifact(fname)
		return content, nil
	case "json":
		if len(args) != 1 || p.ctx.Artifact == nil {
			return nil, nil
		}
		fname, _ := args[0].(string)
		content, ok := p.ctx.Artifact(fname)
		if !ok {
			return nil, nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			return nil, nil
		}
		// optional .path access: if next token is ident starting with '.'
		if p.peek().kind == tIdent && strings.HasPrefix(p.peek().val, ".") {
			path := strings.TrimPrefix(p.next().val, ".")
			return digPath(parsed, path), nil
		}
		return parsed, nil
	}
	return nil, fmt.Errorf("unknown function %q", name)
}

func (p *parser) resolveRef(ref string) (any, error) {
	parts := strings.Split(ref, ".")
	switch parts[0] {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "vars":
		if len(parts) >= 2 {
			return p.ctx.Vars[parts[1]], nil
		}
	case "nodes":
		// nodes.<id>.outputs.<field>
		if len(parts) >= 4 && parts[2] == "outputs" {
			if outs, ok := p.ctx.Nodes[parts[1]]; ok {
				return digPath(outs, strings.Join(parts[3:], ".")), nil
			}
		}
		return nil, nil
	default:
		// bare identifier: prefer routing Extra (e.g. action), then fall back
		// to a global variable of the same name for convenience in guards.
		if v, ok := p.ctx.Extra[ref]; ok {
			return v, nil
		}
		if v, ok := p.ctx.Vars[ref]; ok {
			return v, nil
		}
	}
	return nil, nil
}

func digPath(m map[string]any, path string) any {
	cur := any(m)
	for _, seg := range strings.Split(path, ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[seg]
	}
	return cur
}

func compare(op string, l, r any) bool {
	if lf, lok := toFloat(l); lok {
		if rf, rok := toFloat(r); rok {
			switch op {
			case "==":
				return lf == rf
			case "!=":
				return lf != rf
			case "<":
				return lf < rf
			case ">":
				return lf > rf
			case "<=":
				return lf <= rf
			case ">=":
				return lf >= rf
			}
		}
	}
	ls, rs := fmt.Sprint(l), fmt.Sprint(r)
	switch op {
	case "==":
		return ls == rs
	case "!=":
		return ls != rs
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case bool:
		if n {
			return 1, true
		}
		return 0, true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}
