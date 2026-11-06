// Package wgsl writes the fragment library of a shaderir program as WGSL.
// It uses the Metal uniform layout and reads uniforms from the byte-addressable
// shader_uniforms storage buffer declared by the renderer's effect prelude.
package wgsl

import (
	"fmt"
	"go/constant"
	"math"
	"strings"

	"github.com/egoist/mygo/internal/shaderir"
	"github.com/egoist/mygo/internal/shaderir/msl"
)

type compileContext struct {
	uniformOffsets []int
}

const prelude = `
fn wgsl_mod(x: f32, y: f32) -> f32 {
	return x - y * floor(x / y);
}

fn wgsl_faceforward(n: vec3<f32>, i: vec3<f32>, nref: vec3<f32>) -> vec3<f32> {
	return select(n, -n, dot(nref, i) < 0.0);
}

fn wgsl_reflect(i: vec3<f32>, n: vec3<f32>) -> vec3<f32> {
	return i - 2.0 * dot(n, i) * n;
}
`

// Fragment returns p's fragment library as a WGSL function named name:
//
//	fn name(position: vec4<f32>, ...varyings) -> vec4<f32>
//
// The effect prelude declares shader_uniforms, T0 to T3 and the effect
// sampler. The fragment's position counts its y from the top of the target.
func Fragment(p *shaderir.Program, name string) string {
	lib, entry, funcs := p.FragmentLibrary()
	if entry == nil {
		return ""
	}
	offsets, _ := msl.UniformLayout(lib)
	c := &compileContext{uniformOffsets: offsets}
	var lines []string
	lines = append(lines, strings.Split(Prelude(), "\n")...)
	lines = append(lines, "")
	for _, f := range funcs {
		lines = append(lines, c.function(lib, f)...)
		lines = append(lines, "")
	}
	var params, args []string
	for i, t := range entry.InParams {
		n := "position"
		if i > 0 {
			n = fmt.Sprintf("v%d", i-1)
		}
		params = append(params, fmt.Sprintf("%s: %s", n, c.typ(lib, &t)))
		args = append(args, n)
	}
	lines = append(lines,
		fmt.Sprintf("fn %s(%s) -> vec4<f32> {", name, strings.Join(params, ", ")),
		fmt.Sprintf("\treturn F%d(%s);", entry.Index, strings.Join(args, ", ")),
		"}",
	)
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

// Prelude returns the WGSL helpers a library may use.
func Prelude() string { return prelude }

func (c *compileContext) typ(p *shaderir.Program, t *shaderir.Type) string {
	switch t.Main {
	case shaderir.Bool:
		return "bool"
	case shaderir.Int:
		return "i32"
	case shaderir.Uint:
		return "u32"
	case shaderir.Float:
		return "f32"
	case shaderir.Vec2:
		return "vec2<f32>"
	case shaderir.Vec3:
		return "vec3<f32>"
	case shaderir.Vec4:
		return "vec4<f32>"
	case shaderir.IVec2:
		return "vec2<i32>"
	case shaderir.IVec3:
		return "vec3<i32>"
	case shaderir.IVec4:
		return "vec4<i32>"
	case shaderir.UVec2:
		return "vec2<u32>"
	case shaderir.UVec3:
		return "vec3<u32>"
	case shaderir.UVec4:
		return "vec4<u32>"
	case shaderir.Mat2:
		return "mat2x2<f32>"
	case shaderir.Mat3:
		return "mat3x3<f32>"
	case shaderir.Mat4:
		return "mat4x4<f32>"
	case shaderir.Array:
		return fmt.Sprintf("array<%s, %d>", c.typ(p, &t.Sub[0]), t.Length)
	case shaderir.Struct:
		return fmt.Sprintf("S%d", structIndex(p, t))
	case shaderir.None:
		return "void"
	default:
		panic(fmt.Sprintf("wgsl: unexpected type %s", t.String()))
	}
}

func structIndex(p *shaderir.Program, t *shaderir.Type) int {
	i := 0
	for _, u := range p.Uniforms {
		if u.Main != shaderir.Struct {
			continue
		}
		if u.Equal(t) {
			return i
		}
		i++
	}
	panic("wgsl: struct type outside uniforms is not implemented")
}

func (c *compileContext) varDecl(p *shaderir.Program, t *shaderir.Type, name string) string {
	return fmt.Sprintf("%s: %s", name, c.typ(p, t))
}

func (c *compileContext) zero(p *shaderir.Program, t *shaderir.Type) string {
	switch t.Main {
	case shaderir.Bool:
		return "false"
	case shaderir.Int:
		return "i32(0)"
	case shaderir.Uint:
		return "u32(0)"
	case shaderir.Float:
		return "f32(0)"
	case shaderir.Vec2, shaderir.Vec3, shaderir.Vec4,
		shaderir.IVec2, shaderir.IVec3, shaderir.IVec4,
		shaderir.UVec2, shaderir.UVec3, shaderir.UVec4,
		shaderir.Mat2, shaderir.Mat3, shaderir.Mat4:
		return fmt.Sprintf("%s(0)", c.typ(p, t))
	case shaderir.Array:
		parts := make([]string, t.Length)
		for i := range parts {
			parts[i] = c.zero(p, &t.Sub[0])
		}
		return fmt.Sprintf("%s(%s)", c.typ(p, t), strings.Join(parts, ", "))
	case shaderir.Struct:
		return fmt.Sprintf("%s()", c.typ(p, t))
	default:
		panic(fmt.Sprintf("wgsl: no zero for %s", t.String()))
	}
}

func (c *compileContext) uniform(p *shaderir.Program, index int) string {
	t := &p.Uniforms[index]
	base := c.uniformOffsets[index]
	return c.uniformValue(p, t, base)
}

func (c *compileContext) load(offset int, typ string) string {
	return fmt.Sprintf("bitcast<%s>(shader_uniforms[%d])", typ, offset/4)
}

func (c *compileContext) uniformValue(p *shaderir.Program, t *shaderir.Type, base int) string {
	switch t.Main {
	case shaderir.Bool:
		return fmt.Sprintf("((shader_uniforms[%d] >> %du) & 0xffu) != 0u", base/4, (base%4)*8)
	case shaderir.Int:
		return c.load(base, "i32")
	case shaderir.Uint:
		return c.load(base, "u32")
	case shaderir.Float:
		return c.load(base, "f32")
	case shaderir.Vec2, shaderir.Vec3, shaderir.Vec4:
		return c.vectorValue(base, t.VectorElementCount(), "f32")
	case shaderir.IVec2, shaderir.IVec3, shaderir.IVec4:
		return c.vectorValue(base, t.VectorElementCount(), "i32")
	case shaderir.UVec2, shaderir.UVec3, shaderir.UVec4:
		return c.vectorValue(base, t.VectorElementCount(), "u32")
	case shaderir.Mat2, shaderir.Mat3, shaderir.Mat4:
		n := t.MatrixSize()
		columnStride := 16
		if n == 2 {
			// Metal aligns a float2 column to 8 bytes.
			columnStride = 8
		}
		cols := make([]string, n)
		for i := range cols {
			cols[i] = c.vectorValue(base+i*columnStride, n, "f32")
		}
		return fmt.Sprintf("mat%dx%d<f32>(%s)", n, n, strings.Join(cols, ", "))
	case shaderir.Array:
		stride := 0
		lanes := msl.TypeLanes(&t.Sub[0])
		if len(lanes) > 0 {
			stride = roundUp(lanes[len(lanes)-1]+4, 4)
		}
		parts := make([]string, t.Length)
		for i := range parts {
			parts[i] = c.uniformValue(p, &t.Sub[0], base+i*max(stride, 4))
		}
		return fmt.Sprintf("%s(%s)", c.typ(p, t), strings.Join(parts, ", "))
	default:
		panic(fmt.Sprintf("wgsl: uniform type %s is not supported", t.String()))
	}
}

func (c *compileContext) vectorValue(base, n int, typ string) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = c.load(base+i*4, typ)
	}
	return fmt.Sprintf("vec%d<%s>(%s)", n, typ, strings.Join(parts, ", "))
}

func (c *compileContext) function(p *shaderir.Program, f *shaderir.Func) []string {
	var params []string
	for i, t := range f.InParams {
		params = append(params, c.varDecl(p, &t, fmt.Sprintf("l%d", i)))
	}
	if len(f.OutParams) > 0 {
		panic("wgsl: out parameters are not implemented")
	}
	ret := ""
	if f.Return.Main != shaderir.None {
		ret = " -> " + c.typ(p, &f.Return)
	}
	lines := []string{fmt.Sprintf("fn F%d(%s)%s {", f.Index, strings.Join(params, ", "), ret)}
	lines = append(lines, c.block(p, f.Block, f.Block, 0)...)
	lines = append(lines, "}")
	return lines
}

func (c *compileContext) localName(p *shaderir.Program, topBlock *shaderir.Block, idx int) string {
	return fmt.Sprintf("l%d", idx)
}

func (c *compileContext) initVariable(p *shaderir.Program, topBlock, block *shaderir.Block, index int, decl bool, level int) []string {
	name := c.localName(p, topBlock, index)
	t := p.LocalVariableType(topBlock, block, index)
	idt := strings.Repeat("\t", level+1)
	if t.Main == shaderir.None {
		return nil
	}
	if t.Main == shaderir.Array && decl {
		lines := []string{fmt.Sprintf("%svar %s = %s;", idt, c.varDecl(p, &t, name), c.zero(p, &t))}
		return lines
	}
	if decl {
		return []string{fmt.Sprintf("%svar %s = %s;", idt, c.varDecl(p, &t, name), c.zero(p, &t))}
	}
	return []string{fmt.Sprintf("%s%s = %s;", idt, name, c.zero(p, &t))}
}

func (c *compileContext) block(p *shaderir.Program, topBlock, block *shaderir.Block, level int) []string {
	if block == nil {
		return nil
	}
	var lines []string
	for i := range block.LocalVars {
		lines = append(lines, c.initVariable(p, topBlock, block, block.LocalVarIndexOffset+i, true, level)...)
	}
	var expr func(e *shaderir.Expr) string
	expr = func(e *shaderir.Expr) string {
		switch e.Type {
		case shaderir.NumberExpr:
			return numberLiteral(e.Const, e.Unsigned)
		case shaderir.UniformVariable:
			return c.uniform(p, e.Index)
		case shaderir.TextureVariable:
			return fmt.Sprintf("T%d", e.Index)
		case shaderir.LocalVariable:
			return c.localName(p, topBlock, e.Index)
		case shaderir.BuiltinFuncExpr:
			return builtin(e.BuiltinFunc)
		case shaderir.SwizzlingExpr:
			return e.Swizzling
		case shaderir.FunctionExpr:
			return fmt.Sprintf("F%d", e.Index)
		case shaderir.Unary:
			return fmt.Sprintf("(%s(%s))", unaryOp(e.Op), expr(&e.Exprs[0]))
		case shaderir.Binary:
			if e.Op == shaderir.AndNot {
				return fmt.Sprintf("((%s) & ~(%s))", expr(&e.Exprs[0]), expr(&e.Exprs[1]))
			}
			if e.Op == shaderir.ModOp && isFloat(e.Exprs[0].Const) {
				return fmt.Sprintf("wgsl_mod((%s), (%s))", expr(&e.Exprs[0]), expr(&e.Exprs[1]))
			}
			return fmt.Sprintf("((%s) %s (%s))", expr(&e.Exprs[0]), binaryOp(e.Op), expr(&e.Exprs[1]))
		case shaderir.Selection:
			return fmt.Sprintf("select((%s), (%s), (%s))", expr(&e.Exprs[2]), expr(&e.Exprs[1]), expr(&e.Exprs[0]))
		case shaderir.Call:
			var args []string
			for _, arg := range e.Exprs[1:] {
				args = append(args, expr(&arg))
			}
			callee := e.Exprs[0]
			if callee.Type == shaderir.BuiltinFuncExpr {
				if callee.BuiltinFunc == shaderir.TexelAt {
					return fmt.Sprintf("textureLoad(%s, vec2<i32>(floor(%s)), 0)", expr(&e.Exprs[1]), args[1])
				}
				if callee.BuiltinFunc == shaderir.Min || callee.BuiltinFunc == shaderir.Max {
					result := args[0]
					for i := 1; i < len(args); i++ {
						result = fmt.Sprintf("%s(%s, %s)", builtin(callee.BuiltinFunc), result, args[i])
					}
					return result
				}
				if callee.BuiltinFunc == shaderir.FrontFacing {
					return "true"
				}
				if callee.BuiltinFunc == shaderir.Mod {
					return fmt.Sprintf("wgsl_mod(%s, %s)", args[0], args[1])
				}
				if s, ok := constructor(callee.BuiltinFunc); ok {
					return fmt.Sprintf("%s(%s)", s, strings.Join(args, ", "))
				}
			}
			return fmt.Sprintf("%s(%s)", expr(&callee), strings.Join(args, ", "))
		case shaderir.FieldSelector:
			return fmt.Sprintf("(%s).%s", expr(&e.Exprs[0]), expr(&e.Exprs[1]))
		case shaderir.Index:
			return fmt.Sprintf("(%s)[%s]", expr(&e.Exprs[0]), expr(&e.Exprs[1]))
		default:
			return fmt.Sprintf("/* unexpected expr %d */", e.Type)
		}
	}
	idt := strings.Repeat("\t", level+1)
	for _, stmt := range block.Stmts {
		switch stmt.Type {
		case shaderir.ExprStmt:
			lines = append(lines, idt+expr(&stmt.Exprs[0])+";")
		case shaderir.BlockStmt:
			lines = append(lines, idt+"{")
			lines = append(lines, c.block(p, topBlock, stmt.Blocks[0], level+1)...)
			lines = append(lines, idt+"}")
		case shaderir.Assign:
			lines = append(lines, idt+expr(&stmt.Exprs[0])+" = "+expr(&stmt.Exprs[1])+";")
		case shaderir.Init:
			lines = append(lines, c.initVariable(p, topBlock, block, stmt.InitIndex, false, level)...)
		case shaderir.If:
			lines = append(lines, fmt.Sprintf("%sif (%s) {", idt, expr(&stmt.Exprs[0])))
			lines = append(lines, c.block(p, topBlock, stmt.Blocks[0], level+1)...)
			if len(stmt.Blocks) > 1 {
				lines = append(lines, idt+"} else {")
				lines = append(lines, c.block(p, topBlock, stmt.Blocks[1], level+1)...)
			}
			lines = append(lines, idt+"}")
		case shaderir.For:
			v := c.localName(p, topBlock, stmt.ForVarIndex)
			init, end := constantToNumberLiteral(stmt.ForInit), constantToNumberLiteral(stmt.ForEnd)
			if len(stmt.Exprs) == 2 {
				init, end = expr(&stmt.Exprs[0]), expr(&stmt.Exprs[1])
			}
			delta := numberLiteral(stmt.ForDelta, stmt.ForVarType.Scalar() == shaderir.Uint)
			lines = append(lines, fmt.Sprintf("%sfor (var %s: %s = %s; %s %s %s; %s = %s + %s) {",
				idt, v, c.typ(p, &stmt.ForVarType), init, v, binaryOp(stmt.ForOp), end, v, v, delta))
			lines = append(lines, c.block(p, topBlock, stmt.Blocks[0], level+1)...)
			lines = append(lines, idt+"}")
		case shaderir.Continue:
			lines = append(lines, idt+"continue;")
		case shaderir.Break:
			lines = append(lines, idt+"break;")
		case shaderir.Return:
			if len(stmt.Exprs) == 0 {
				lines = append(lines, idt+"return;")
			} else {
				lines = append(lines, idt+"return "+expr(&stmt.Exprs[0])+";")
			}
		case shaderir.Discard:
			lines = append(lines, idt+"discard;", idt+"return vec4<f32>(0.0);")
		default:
			lines = append(lines, fmt.Sprintf("%s/* unexpected stmt %d */", idt, stmt.Type))
		}
	}
	return lines
}

func isFloat(v constant.Value) bool {
	return v != nil && v.Kind() == constant.Float
}

func builtin(f shaderir.BuiltinFunc) string {
	switch f {
	case shaderir.Atan2:
		return "atan2"
	case shaderir.Inversesqrt:
		return "inverseSqrt"
	case shaderir.Dfdx:
		return "dpdx"
	case shaderir.Dfdy:
		return "dpdy"
	case shaderir.Faceforward:
		return "wgsl_faceforward"
	case shaderir.Reflect:
		return "wgsl_reflect"
	case shaderir.Refract:
		return "refract"
	default:
		return string(f)
	}
}

func constructor(f shaderir.BuiltinFunc) (string, bool) {
	switch f {
	case shaderir.BoolF:
		return "bool", true
	case shaderir.IntF:
		return "i32", true
	case shaderir.UintF:
		return "u32", true
	case shaderir.FloatF:
		return "f32", true
	case shaderir.Vec2F:
		return "vec2<f32>", true
	case shaderir.Vec3F:
		return "vec3<f32>", true
	case shaderir.Vec4F:
		return "vec4<f32>", true
	case shaderir.IVec2F:
		return "vec2<i32>", true
	case shaderir.IVec3F:
		return "vec3<i32>", true
	case shaderir.IVec4F:
		return "vec4<i32>", true
	case shaderir.UVec2F:
		return "vec2<u32>", true
	case shaderir.UVec3F:
		return "vec3<u32>", true
	case shaderir.UVec4F:
		return "vec4<u32>", true
	case shaderir.Mat2F:
		return "mat2x2<f32>", true
	case shaderir.Mat3F:
		return "mat3x3<f32>", true
	case shaderir.Mat4F:
		return "mat4x4<f32>", true
	}
	return "", false
}

func unaryOp(op shaderir.Op) string {
	switch op {
	case shaderir.Add:
		return "+"
	case shaderir.Sub:
		return "-"
	case shaderir.NotOp:
		return "!"
	case shaderir.ComplementOp:
		return "~"
	}
	return "?"
}

func binaryOp(op shaderir.Op) string {
	switch op {
	case shaderir.Add:
		return "+"
	case shaderir.Sub:
		return "-"
	case shaderir.ComponentWiseMul, shaderir.MatrixMul:
		return "*"
	case shaderir.Div:
		return "/"
	case shaderir.ModOp:
		return "%"
	case shaderir.LeftShift:
		return "<<"
	case shaderir.RightShift:
		return ">>"
	case shaderir.LessThanOp:
		return "<"
	case shaderir.LessThanEqualOp:
		return "<="
	case shaderir.GreaterThanOp:
		return ">"
	case shaderir.GreaterThanEqualOp:
		return ">="
	case shaderir.EqualOp, shaderir.VectorEqualOp:
		return "=="
	case shaderir.NotEqualOp, shaderir.VectorNotEqualOp:
		return "!="
	case shaderir.And:
		return "&"
	case shaderir.Xor:
		return "^"
	case shaderir.Or:
		return "|"
	case shaderir.AndAnd:
		return "&&"
	case shaderir.OrOr:
		return "||"
	}
	return "?"
}

func constantToNumberLiteral(v constant.Value) string {
	return numberLiteral(v, false)
}

func numberLiteral(v constant.Value, unsigned bool) string {
	if v == nil {
		return "0"
	}
	switch v.Kind() {
	case constant.Bool:
		if constant.BoolVal(v) {
			return "true"
		}
		return "false"
	case constant.Int:
		x, _ := constant.Int64Val(v)
		if unsigned {
			return fmt.Sprintf("%du", uint32(x))
		}
		return fmt.Sprintf("%d", x)
	case constant.Float:
		x, _ := constant.Float64Val(v)
		if math.Trunc(x) == x && math.Abs(x) < 1<<63 {
			return fmt.Sprintf("%d.0", int64(x))
		}
		return fmt.Sprintf("%.10e", x)
	}
	return "0"
}

func roundUp(x, a int) int { return (x + a - 1) / a * a }
