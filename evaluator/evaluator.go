package evaluator

import (
	"artemis/ast"
	"artemis/lexer"
	"artemis/object"
	"artemis/parser"
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

//go:embed all:std
var embeddedStd embed.FS

const StdBltinsFallback = `
set help = fn() {
    out "Artemis Language - Core Primitives Only";
    out "Standard library (std/core.artms) not found.";
};
`

func InitEnv(env *object.Environment) {
	// Try to load core library from disk first, then from embedded FS
	stdPath := "std/core.artms"
	content, err := ioutil.ReadFile(stdPath)
	var source string
	if err == nil {
		source = string(content)
	} else {
		// Fallback to embedded
		embeddedContent, err := embeddedStd.ReadFile(stdPath)
		if err == nil {
			source = string(embeddedContent)
		} else {
			source = StdBltinsFallback
		}
	}

	l := lexer.New(source)
	p := parser.New(l)
	prog := p.ParseProgram()
	Eval(prog, env)
}

var (
	NULL         = &object.Null{}
	TRUE         = &object.Boolean{Value: true}
	FALSE        = &object.Boolean{Value: false}
	stdinScanner = bufio.NewScanner(os.Stdin)
)

var builtins = map[string]*object.Builtin{
	"type": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return &object.String{Value: string(args[0].Type())}
		},
	},
	"len": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			switch arg := args[0].(type) {
			case *object.Array:
				return &object.Integer{Value: int64(len(arg.Elements))}
			case *object.String:
				return &object.Integer{Value: int64(len(arg.Value))}
			default:
				return &object.Error{Message: fmt.Sprintf("argument to `len` not supported, got %s", args[0].Type())}
			}
		},
	},
	"push": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != object.ARRAY_OBJ {
				return &object.Error{Message: fmt.Sprintf("argument to `push` must be ARRAY, got %s", args[0].Type())}
			}

			arr := args[0].(*object.Array)
			length := len(arr.Elements)

			newElements := make([]object.Object, length+1)
			copy(newElements, arr.Elements)
			newElements[length] = args[1]

			return &object.Array{Elements: newElements}
		},
	},
	"readFile": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != object.STRING_OBJ {
				return &object.Error{Message: fmt.Sprintf("argument to `readFile` must be STRING, got %s", args[0].Type())}
			}

			path := args[0].(*object.String).Value
			content, err := ioutil.ReadFile(path)
			if err != nil {
				return &object.Error{Message: fmt.Sprintf("could not read file %s: %s", path, err.Error())}
			}
			return &object.String{Value: string(content)}
		},
	},
	"writeFile": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			if args[0].Type() != object.STRING_OBJ || args[1].Type() != object.STRING_OBJ {
				return &object.Error{Message: "arguments to `writeFile` must be STRING, STRING"}
			}

			path := args[0].(*object.String).Value
			data := args[1].(*object.String).Value

			err := ioutil.WriteFile(path, []byte(data), 0644)
			if err != nil {
				return &object.Error{Message: fmt.Sprintf("could not write file %s: %s", path, err.Error())}
			}
			return NULL
		},
	},
	"first": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != object.ARRAY_OBJ {
				return &object.Error{Message: fmt.Sprintf("argument to `first` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*object.Array)
			if len(arr.Elements) > 0 {
				return arr.Elements[0]
			}
			return NULL
		},
	},
	"last": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != object.ARRAY_OBJ {
				return &object.Error{Message: fmt.Sprintf("argument to `last` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*object.Array)
			length := len(arr.Elements)
			if length > 0 {
				return arr.Elements[length-1]
			}
			return NULL
		},
	},
	"pop": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != object.ARRAY_OBJ {
				return &object.Error{Message: fmt.Sprintf("argument to `pop` must be ARRAY, got %s", args[0].Type())}
			}
			arr := args[0].(*object.Array)
			length := len(arr.Elements)
			if length > 0 {
				newElements := make([]object.Object, length-1)
				copy(newElements, arr.Elements[0:length-1])
				return &object.Array{Elements: newElements}
			}
			return &object.Array{Elements: []object.Object{}}
		},
	},
	"toUpperCase": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != object.STRING_OBJ {
				return &object.Error{Message: fmt.Sprintf("argument to `toUpperCase` must be STRING, got %s", args[0].Type())}
			}
			return &object.String{Value: strings.ToUpper(args[0].(*object.String).Value)}
		},
	},
	"toLowerCase": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != object.STRING_OBJ {
				return &object.Error{Message: fmt.Sprintf("argument to `toLowerCase` must be STRING, got %s", args[0].Type())}
			}
			return &object.String{Value: strings.ToLower(args[0].(*object.String).Value)}
		},
	},
	"now": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			return &object.Integer{Value: time.Now().UnixNano() / int64(time.Millisecond)}
		},
	},
	"sleep": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			if args[0].Type() != object.INTEGER_OBJ {
				return &object.Error{Message: fmt.Sprintf("argument to `sleep` must be INTEGER (ms), got %s", args[0].Type())}
			}
			ms := args[0].(*object.Integer).Value
			time.Sleep(time.Duration(ms) * time.Millisecond)
			return NULL
		},
	},
	"json_encode": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=1"}
			}
			data := objToRaw(args[0])
			res, err := json.Marshal(data)
			if err != nil {
				return &object.Error{Message: "json encoding error: " + err.Error()}
			}
			return &object.String{Value: string(res)}
		},
	},
	"json_decode": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=1"}
			}
			str, ok := args[0].(*object.String)
			if !ok {
				return &object.Error{Message: "argument to json_decode must be STRING"}
			}
			var data interface{}
			err := json.Unmarshal([]byte(str.Value), &data)
			if err != nil {
				return &object.Error{Message: "json decoding error: " + err.Error()}
			}
			return rawToObj(data)
		},
	},
	"fs_remove": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return &object.Error{Message: "argument to fs_remove must be STRING"}
			}
			err := os.Remove(path.Value)
			if err != nil {
				return &object.Error{Message: err.Error()}
			}
			return NULL
		},
	},
	"fs_exists": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			path, ok := args[0].(*object.String)
			if !ok {
				return &object.Error{Message: "argument to fs_exists must be STRING"}
			}
			_, err := os.Stat(path.Value)
			if os.IsNotExist(err) {
				return FALSE
			}
			return TRUE
		},
	},
	"os_mouse_move": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=2", len(args))}
			}
			x, ok1 := args[0].(*object.Integer)
			y, ok2 := args[1].(*object.Integer)
			if !ok1 || !ok2 {
				return &object.Error{Message: "arguments to mouse_move must be INTEGER"}
			}
			setCursorPos.Call(uintptr(x.Value), uintptr(y.Value))
			return NULL
		},
	},
	"os_mouse_click": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// Basic left click
			mouseEvent.Call(uintptr(0x0002), 0, 0, 0, 0) // MOUSEEVENTF_LEFTDOWN
			mouseEvent.Call(uintptr(0x0004), 0, 0, 0, 0) // MOUSEEVENTF_LEFTUP
			return NULL
		},
	},
	"os_key_tap": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=1"}
			}
			key, ok := args[0].(*object.Integer)
			if !ok {
				return &object.Error{Message: "argument to key_tap must be INTEGER (VK code)"}
			}
			keybdEvent.Call(uintptr(key.Value), 0, 0, 0)               // Key down
			keybdEvent.Call(uintptr(key.Value), 0, uintptr(0x0002), 0) // Key up (KEYEVENTF_KEYUP = 0x0002)
			return NULL
		},
	},
	"os_exec": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=1"}
			}
			input, ok := args[0].(*object.String)
			if !ok {
				return &object.Error{Message: "argument to os_exec must be STRING"}
			}
			out, err := exec.Command("cmd", "/C", input.Value).CombinedOutput()
			if err != nil {
				return &object.Error{Message: string(out) + " " + err.Error()}
			}
			return &object.String{Value: string(out)}
		},
	},
	"os_mouse_get_pos": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			var pt POINT
			getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
			return &object.Hash{Pairs: map[object.HashKey]object.HashPair{
				(&object.String{Value: "x"}).HashKey(): {Key: &object.String{Value: "x"}, Value: &object.Integer{Value: int64(pt.X)}},
				(&object.String{Value: "y"}).HashKey(): {Key: &object.String{Value: "y"}, Value: &object.Integer{Value: int64(pt.Y)}},
			}}
		},
	},
	"math_random": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=1"}
			}
			max, ok := args[0].(*object.Integer)
			if !ok {
				return &object.Error{Message: "argument to random must be INTEGER"}
			}
			if max.Value <= 0 {
				return &object.Integer{Value: 0}
			}
			return &object.Integer{Value: int64(rand.Intn(int(max.Value)))}
		},
	},
	"http_get": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=1"}
			}
			url, ok := args[0].(*object.String)
			if !ok {
				return &object.Error{Message: "argument to http_get must be STRING"}
			}
			resp, err := http.Get(url.Value)
			if err != nil {
				return &object.Error{Message: err.Error()}
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return &object.Error{Message: err.Error()}
			}
			return &object.String{Value: string(body)}
		},
	},
	"os_alert": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=2"}
			}
			title, ok1 := args[0].(*object.String)
			msg, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return &object.Error{Message: "arguments to alert must be STRING"}
			}
			tPtr, _ := syscall.UTF16PtrFromString(title.Value)
			mPtr, _ := syscall.UTF16PtrFromString(msg.Value)
			messageBox.Call(0, uintptr(unsafe.Pointer(mPtr)), uintptr(unsafe.Pointer(tPtr)), 0)
			return NULL
		},
	},
	"input": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) == 1 {
				prompt, ok := args[0].(*object.String)
				if ok {
					fmt.Print(prompt.Value)
				}
			}
			if stdinScanner.Scan() {
				return &object.String{Value: stdinScanner.Text()}
			}
			return &object.String{Value: ""}
		},
	},
	"int": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			switch arg := args[0].(type) {
			case *object.Integer:
				return arg
			case *object.String:
				cleanVal := strings.TrimSpace(arg.Value)
				val, err := strconv.ParseInt(cleanVal, 0, 64)
				if err != nil {
					return &object.Error{Message: fmt.Sprintf("could not parse string '%s' as integer: %v", cleanVal, err)}
				}
				return &object.Integer{Value: val}
			default:
				return &object.Error{Message: "cannot convert to integer"}
			}
		},
	},
	"str": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return NULL
			}
			return &object.String{Value: args[0].Inspect()}
		},
	},
	"copy": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			text, ok := args[0].(*object.String)
			if !ok {
				return &object.Error{Message: "argument to copy must be STRING"}
			}
			setClipboard(text.Value)
			return NULL
		},
	},
	"paste": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			return &object.String{Value: getClipboard()}
		},
	},
	"os_keyboard_type": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			text, ok := args[0].(*object.String)
			if !ok {
				return &object.Error{Message: "argument to type must be STRING"}
			}
			for _, char := range text.Value {
				// Simplified typing logic for common chars
				vk := charToVK(char)
				if vk != 0 {
					keybdEvent.Call(uintptr(vk), 0, 0, 0)
					keybdEvent.Call(uintptr(vk), 0, uintptr(0x0002), 0)
				}
			}
			return NULL
		},
	},
}

type POINT struct {
	X, Y int32
}

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	setCursorPos     = user32.NewProc("SetCursorPos")
	getCursorPos     = user32.NewProc("GetCursorPos")
	mouseEvent       = user32.NewProc("mouse_event")
	keybdEvent       = user32.NewProc("keybd_event")
	messageBox       = user32.NewProc("MessageBoxW")
	openClipboard    = user32.NewProc("OpenClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	setClipboardData = user32.NewProc("SetClipboardData")
	getClipboardData = user32.NewProc("GetClipboardData")
	closeClipboard   = user32.NewProc("CloseClipboard")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	lstrcpy          = kernel32.NewProc("lstrcpyW")
)

func setClipboard(text string) {
	opened, _, _ := openClipboard.Call(0)
	if opened == 0 {
		return
	}
	defer closeClipboard.Call()
	emptyClipboard.Call()

	utf16 := utf16.Encode([]rune(text + "\x00"))
	size := uintptr(len(utf16) * 2)
	hMem, _, _ := globalAlloc.Call(uintptr(0x0042), size) // GHND = 0x0042
	ptr, _, _ := globalLock.Call(hMem)
	lstrcpy.Call(ptr, uintptr(unsafe.Pointer(&utf16[0])))
	globalUnlock.Call(hMem)

	setClipboardData.Call(uintptr(13), hMem) // CF_UNICODETEXT = 13
}

func getClipboard() string {
	opened, _, _ := openClipboard.Call(0)
	if opened == 0 {
		return ""
	}
	defer closeClipboard.Call()

	hMem, _, _ := getClipboardData.Call(uintptr(13))
	if hMem == 0 {
		return ""
	}

	ptr, _, _ := globalLock.Call(hMem)
	defer globalUnlock.Call(hMem)

	var res []uint16
	for i := 0; ; i++ {
		char := *(*uint16)(unsafe.Pointer(ptr + uintptr(i*2)))
		if char == 0 {
			break
		}
		res = append(res, char)
	}
	return string(utf16.Decode(res))
}

func charToVK(r rune) byte {
	// Very basic mapping for demo/automation purposes
	if r >= 'a' && r <= 'z' {
		return byte(r - 'a' + 0x41)
	}
	if r >= 'A' && r <= 'Z' {
		return byte(r - 'A' + 0x41)
	}
	if r >= '0' && r <= '9' {
		return byte(r - '0' + 0x30)
	}
	if r == ' ' {
		return 0x20
	}
	return 0
}

func Eval(node ast.Node, env *object.Environment) object.Object {
	switch node := node.(type) {
	case *ast.Program:
		return evalProgram(node, env)
	case *ast.ExpressionStatement:
		return Eval(node.Expression, env)
	case *ast.BlockStatement:
		return evalBlockStatement(node, env)
	case *ast.ImportStatement:
		return evalImportStatement(node, env)
	case *ast.SetStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		env.Set(node.Name.Value, val)
	case *ast.AssignStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		if !env.Update(node.Name.Value, val) {
			return &object.Error{Message: fmt.Sprintf("cannot assign to undefined variable: %s", node.Name.Value)}
		}
		return val
	case *ast.ThrowStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		return &object.Error{Message: val.Inspect()}
	case *ast.TryExpression:
		return evalTryExpression(node, env)
	case *ast.OutStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		if val != nil {
			fmt.Println(val.Inspect())
		}
	case *ast.ReturnStatement:
		val := Eval(node.Value, env)
		if isError(val) {
			return val
		}
		return &object.ReturnValue{Value: val}
	case *ast.MatchExpression:
		return evalMatchExpression(node, env)
	case *ast.SpawnStatement:
		return evalSpawnStatement(node, env)
	case *ast.IfStatement:
		return evalIfStatement(node, env)
	case *ast.WhileStatement:
		return evalWhileStatement(node, env)
	case *ast.ForStatement:
		return evalForStatement(node, env)
	case *ast.IntegerLiteral:
		return &object.Integer{Value: node.Value}
	case *ast.FloatLiteral:
		return &object.Float{Value: node.Value}
	case *ast.Boolean:
		return nativeBoolToBooleanObject(node.Value)
	case *ast.StringLiteral:
		return &object.String{Value: node.Value}
	case *ast.InterpolatedString:
		return evalInterpolatedString(node, env)
	case *ast.ArrayLiteral:
		elements := evalExpressions(node.Elements, env)
		if len(elements) == 1 && isError(elements[0]) {
			return elements[0]
		}
		return &object.Array{Elements: elements}
	case *ast.HashLiteral:
		return evalHashLiteral(node, env)
	case *ast.IndexExpression:
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		index := Eval(node.Index, env)
		if isError(index) {
			return index
		}
		return evalIndexExpression(left, index)
	case *ast.Identifier:
		return evalIdentifier(node, env)
	case *ast.PrefixExpression:
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalPrefixExpression(node.Operator, right)
	case *ast.InfixExpression:
		if node.Operator == "&&" || node.Operator == "||" {
			left := Eval(node.Left, env)
			if isError(left) {
				return left
			}
			if node.Operator == "&&" && !isTruthy(left) {
				return FALSE
			}
			if node.Operator == "||" && isTruthy(left) {
				return TRUE
			}
			right := Eval(node.Right, env)
			if isError(right) {
				return right
			}
			return nativeBoolToBooleanObject(isTruthy(right))
		}
		left := Eval(node.Left, env)
		if isError(left) {
			return left
		}
		right := Eval(node.Right, env)
		if isError(right) {
			return right
		}
		return evalInfixExpression(node.Operator, left, right)
	case *ast.PostfixExpression:
		return evalPostfixExpression(node, env)
	case *ast.MemberExpression:
		return evalMemberExpression(node, env)
	case *ast.PipeExpression:
		return evalPipeExpression(node, env)
	case *ast.FunctionLiteral:
		params := node.Parameters
		body := node.Body
		return &object.Function{Parameters: params, Env: env, Body: body}
	case *ast.CallExpression:
		function := Eval(node.Function, env)
		if isError(function) {
			return function
		}
		args := evalExpressions(node.Arguments, env)
		if len(args) == 1 && isError(args[0]) {
			return args[0]
		}
		return applyFunction(function, args)
	}
	return nil
}

func evalProgram(program *ast.Program, env *object.Environment) object.Object {
	var result object.Object
	for _, statement := range program.Statements {
		result = Eval(statement, env)
		switch result := result.(type) {
		case *object.ReturnValue:
			return result.Value
		case *object.Error:
			return result
		}
	}
	return result
}

func evalBlockStatement(block *ast.BlockStatement, env *object.Environment) object.Object {
	var result object.Object
	for _, statement := range block.Statements {
		result = Eval(statement, env)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
		}
	}
	return result
}

func nativeBoolToBooleanObject(input bool) *object.Boolean {
	if input {
		return TRUE
	}
	return FALSE
}

func evalIfStatement(ie *ast.IfStatement, env *object.Environment) object.Object {
	condition := Eval(ie.Condition, env)
	if isError(condition) {
		return condition
	}
	if isTruthy(condition) {
		return Eval(ie.Consequence, env)
	} else if ie.Alternative != nil {
		return Eval(ie.Alternative, env)
	} else {
		return NULL
	}
}

func evalWhileStatement(ws *ast.WhileStatement, env *object.Environment) object.Object {
	for {
		condition := Eval(ws.Condition, env)
		if isError(condition) {
			return condition
		}
		if !isTruthy(condition) {
			break
		}
		result := Eval(ws.Body, env)
		if result != nil && (result.Type() == object.RETURN_VALUE_OBJ || result.Type() == object.ERROR_OBJ) {
			return result
		}
	}
	return NULL
}

func evalImportStatement(node *ast.ImportStatement, env *object.Environment) object.Object {
	pathVal := Eval(node.Path, env)
	if isError(pathVal) {
		return pathVal
	}
	str, ok := pathVal.(*object.String)
	if !ok {
		return &object.Error{Message: "import path must be string"}
	}

	path := str.Value
	var content []byte
	var err error

	// 1. Try disk path as-is
	content, err = ioutil.ReadFile(path)
	if err != nil {
		// 2. Try std/ path on disk
		stdPath := "std/" + path
		content, err = ioutil.ReadFile(stdPath)
		if err != nil {
			// 3. Try embedded FS
			content, err = embeddedStd.ReadFile(stdPath)
			if err != nil {
				return &object.Error{Message: fmt.Sprintf("could not find module %s on disk or in standard library", path)}
			}
		}
	}

	l := lexer.New(string(content))
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors) > 0 {
		return &object.Error{Message: fmt.Sprintf("parse errors in %s: %s", str.Value, p.Errors[0])}
	}

	if node.Alias != nil {
		// Namespaced import
		moduleEnv := object.NewEnvironment()
		Eval(prog, moduleEnv)
		module := &object.Module{Name: node.Alias.Value, Env: moduleEnv}
		env.Set(node.Alias.Value, module)
		return NULL
	}

	return Eval(prog, env) // execute in current env (standard import)
}

func evalInfixExpression(operator string, left, right object.Object) object.Object {
	switch {
	case left.Type() == object.INTEGER_OBJ && right.Type() == object.INTEGER_OBJ:
		return evalIntegerInfixExpression(operator, left, right)
	case left.Type() == object.FLOAT_OBJ || right.Type() == object.FLOAT_OBJ:
		return evalFloatInfixExpression(operator, left, right)
	case left.Type() == object.STRING_OBJ && right.Type() == object.STRING_OBJ:
		return evalStringInfixExpression(operator, left, right)
	case operator == "==":
		return nativeBoolToBooleanObject(objectsEqual(left, right))
	case operator == "!=":
		return nativeBoolToBooleanObject(!objectsEqual(left, right))
	case left.Type() != right.Type():
		return &object.Error{Message: fmt.Sprintf("type mismatch: %s %s %s", left.Type(), operator, right.Type())}
	default:
		return &object.Error{Message: fmt.Sprintf("unknown operator: %s %s %s", left.Type(), operator, right.Type())}
	}
}

func evalIntegerInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.Integer).Value
	rightVal := right.(*object.Integer).Value
	switch operator {
	case "+":
		return &object.Integer{Value: leftVal + rightVal}
	case "-":
		return &object.Integer{Value: leftVal - rightVal}
	case "*":
		return &object.Integer{Value: leftVal * rightVal}
	case "/":
		return &object.Integer{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	case "%":
		return &object.Integer{Value: leftVal % rightVal}
	default:
		return &object.Error{Message: fmt.Sprintf("unknown operator: INTEGER %s INTEGER", operator)}
	}
}

func evalStringInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := left.(*object.String).Value
	rightVal := right.(*object.String).Value

	switch operator {
	case "+":
		return &object.String{Value: leftVal + rightVal}
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return &object.Error{Message: fmt.Sprintf("unknown operator: STRING %s STRING", operator)}
	}
}

func evalIdentifier(node *ast.Identifier, env *object.Environment) object.Object {
	if val, ok := env.Get(node.Value); ok {
		return val
	}
	if builtin, ok := builtins[node.Value]; ok {
		return builtin
	}
	return &object.Error{Message: "identifier not found: " + node.Value}
}

func evalIndexExpression(left, index object.Object) object.Object {
	switch {
	case left.Type() == object.ARRAY_OBJ && index.Type() == object.INTEGER_OBJ:
		return evalArrayIndexExpression(left, index)
	case left.Type() == object.HASH_OBJ:
		return evalHashIndexExpression(left, index)
	default:
		return &object.Error{Message: fmt.Sprintf("index operator not supported: %s", left.Type())}
	}
}

func evalArrayIndexExpression(array, index object.Object) object.Object {
	arrayObject := array.(*object.Array)
	idx := index.(*object.Integer).Value
	max := int64(len(arrayObject.Elements) - 1)
	if idx < 0 || idx > max {
		return NULL
	}
	return arrayObject.Elements[idx]
}

func evalHashIndexExpression(hash, index object.Object) object.Object {
	hashObject := hash.(*object.Hash)
	key, ok := index.(object.Hashable)
	if !ok {
		return &object.Error{Message: fmt.Sprintf("unusable as hash key: %s", index.Type())}
	}
	pair, ok := hashObject.Pairs[key.HashKey()]
	if !ok {
		return NULL
	}
	return pair.Value
}

func evalHashLiteral(node *ast.HashLiteral, env *object.Environment) object.Object {
	pairs := make(map[object.HashKey]object.HashPair)
	for keyNode, valueNode := range node.Pairs {
		key := Eval(keyNode, env)
		if isError(key) {
			return key
		}

		hashKey, ok := key.(object.Hashable)
		if !ok {
			return &object.Error{Message: fmt.Sprintf("unusable as hash key: %s", key.Type())}
		}

		value := Eval(valueNode, env)
		if isError(value) {
			return value
		}

		pairs[hashKey.HashKey()] = object.HashPair{Key: key, Value: value}
	}
	return &object.Hash{Pairs: pairs}
}

func evalExpressions(exps []ast.Expression, env *object.Environment) []object.Object {
	var result []object.Object
	for _, e := range exps {
		evaluated := Eval(e, env)
		if isError(evaluated) {
			return []object.Object{evaluated}
		}
		result = append(result, evaluated)
	}
	return result
}

func applyFunction(fn object.Object, args []object.Object) object.Object {
	switch function := fn.(type) {
	case *object.Function:
		extendedEnv := extendFunctionEnv(function, args)
		evaluated := Eval(function.Body, extendedEnv)
		return unwrapReturnValue(evaluated)
	case *object.Builtin:
		return function.Fn(args...)
	default:
		return &object.Error{Message: fmt.Sprintf("not a function: %s", fn.Type())}
	}
}

func extendFunctionEnv(fn *object.Function, args []object.Object) *object.Environment {
	env := object.NewEnclosedEnvironment(fn.Env)
	for i, param := range fn.Parameters {
		if i < len(args) {
			env.Set(param.Value, args[i])
		}
	}
	return env
}

func unwrapReturnValue(obj object.Object) object.Object {
	if returnValue, ok := obj.(*object.ReturnValue); ok {
		return returnValue.Value
	}
	return obj
}

func isTruthy(obj object.Object) bool {
	switch obj {
	case NULL:
		return false
	case TRUE:
		return true
	case FALSE:
		return false
	default:
		if intObj, ok := obj.(*object.Integer); ok {
			return intObj.Value != 0
		}
		if floatObj, ok := obj.(*object.Float); ok {
			return floatObj.Value != 0.0
		}
		return true
	}
}

func isError(obj object.Object) bool {
	if obj != nil {
		return obj.Type() == object.ERROR_OBJ
	}
	return false
}

func evalForStatement(fs *ast.ForStatement, env *object.Environment) object.Object {
	childEnv := object.NewEnclosedEnvironment(env)
	if fs.Init != nil {
		initRes := Eval(fs.Init, childEnv)
		if isError(initRes) {
			return initRes
		}
	}

	for {
		if fs.Condition != nil {
			condition := Eval(fs.Condition, childEnv)
			if isError(condition) {
				return condition
			}
			if !isTruthy(condition) {
				break
			}
		}

		result := Eval(fs.Body, childEnv)
		if result != nil {
			rt := result.Type()
			if rt == object.RETURN_VALUE_OBJ || rt == object.ERROR_OBJ {
				return result
			}
		}

		if fs.Update != nil {
			updateRes := Eval(fs.Update, childEnv)
			if isError(updateRes) {
				return updateRes
			}
		}
	}
	return NULL
}

func evalPostfixExpression(pe *ast.PostfixExpression, env *object.Environment) object.Object {
	ident, ok := pe.Left.(*ast.Identifier)
	if !ok {
		return &object.Error{Message: "postfix operator can only be applied to identifiers"}
	}

	val, ok := env.Get(ident.Value)
	if !ok {
		return &object.Error{Message: fmt.Sprintf("identifier not found: %s", ident.Value)}
	}

	switch pe.Operator {
	case "++":
		switch v := val.(type) {
		case *object.Integer:
			newVal := &object.Integer{Value: v.Value + 1}
			env.Set(ident.Value, newVal)
			return v // Return old value for postfix?
			// In many C-like langs, x++ returns the value before increment.
		case *object.Float:
			newVal := &object.Float{Value: v.Value + 1.0}
			env.Set(ident.Value, newVal)
			return v
		}
	case "--":
		switch v := val.(type) {
		case *object.Integer:
			newVal := &object.Integer{Value: v.Value - 1}
			env.Set(ident.Value, newVal)
			return v
		case *object.Float:
			newVal := &object.Float{Value: v.Value - 1.0}
			env.Set(ident.Value, newVal)
			return v
		}
	}
	return &object.Error{Message: fmt.Sprintf("unknown postfix operator: %s", pe.Operator)}
}

func evalMemberExpression(me *ast.MemberExpression, env *object.Environment) object.Object {
	obj := Eval(me.Object, env)
	if isError(obj) {
		return obj
	}

	memberName := me.Member.Value

	switch obj.Type() {
	case object.MODULE_OBJ:
		m := obj.(*object.Module)
		val, ok := m.Env.Get(memberName)
		if !ok {
			return &object.Error{Message: fmt.Sprintf("identifier %s not found in module %s", memberName, m.Name)}
		}
		return val

	case object.ARRAY_OBJ:
		arr := obj.(*object.Array)
		switch memberName {
		case "len":
			return &object.Builtin{Fn: func(args ...object.Object) object.Object {
				return &object.Integer{Value: int64(len(arr.Elements))}
			}}
		case "push":
			return &object.Builtin{Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.Error{Message: "array.push() expects 1 argument"}
				}
				arr.Elements = append(arr.Elements, args[0])
				return arr
			}}
		}

	case object.STRING_OBJ:
		s := obj.(*object.String)
		switch memberName {
		case "len":
			return &object.Builtin{Fn: func(args ...object.Object) object.Object {
				return &object.Integer{Value: int64(len(s.Value))}
			}}
		}
	case object.HASH_OBJ:
		h := obj.(*object.Hash)
		key := &object.String{Value: memberName}
		child, ok := h.Pairs[key.HashKey()]
		if !ok {
			return &object.Error{Message: fmt.Sprintf("key %s not found in hash", memberName)}
		}
		return child.Value
	}

	return &object.Error{Message: fmt.Sprintf("member %s not found on type %s", memberName, obj.Type())}
}

func evalFloatInfixExpression(operator string, left, right object.Object) object.Object {
	leftVal := getFloatVal(left)
	rightVal := getFloatVal(right)

	switch operator {
	case "+":
		return &object.Float{Value: leftVal + rightVal}
	case "-":
		return &object.Float{Value: leftVal - rightVal}
	case "*":
		return &object.Float{Value: leftVal * rightVal}
	case "/":
		return &object.Float{Value: leftVal / rightVal}
	case "<":
		return nativeBoolToBooleanObject(leftVal < rightVal)
	case ">":
		return nativeBoolToBooleanObject(leftVal > rightVal)
	case "==":
		return nativeBoolToBooleanObject(leftVal == rightVal)
	case "!=":
		return nativeBoolToBooleanObject(leftVal != rightVal)
	default:
		return &object.Error{Message: fmt.Sprintf("unknown operator: FLOAT %s FLOAT", operator)}
	}
}

func getFloatVal(obj object.Object) float64 {
	switch v := obj.(type) {
	case *object.Integer:
		return float64(v.Value)
	case *object.Float:
		return v.Value
	default:
		return 0.0
	}
}

func evalInterpolatedString(node *ast.InterpolatedString, env *object.Environment) object.Object {
	var out bytes.Buffer
	for _, part := range node.Parts {
		evaluated := Eval(part, env)
		if isError(evaluated) {
			return evaluated
		}
		out.WriteString(evaluated.Inspect())
	}
	return &object.String{Value: out.String()}
}

func evalMatchExpression(node *ast.MatchExpression, env *object.Environment) object.Object {
	val := Eval(node.Value, env)
	if isError(val) {
		return val
	}

	for _, c := range node.Cases {
		// Special case for wildcard _
		if ident, ok := c.Pattern.(*ast.Identifier); ok && ident.Value == "_" {
			return evalBlock(c.Body, env)
		}

		patternVal := Eval(c.Pattern, env)
		if isError(patternVal) {
			return patternVal
		}

		if objectsEqual(val, patternVal) {
			return evalBlock(c.Body, env)
		}
	}

	return NULL
}

func evalBlock(block *ast.BlockStatement, env *object.Environment) object.Object {
	return evalBlockStatement(block, env)
}

func evalSpawnStatement(node *ast.SpawnStatement, env *object.Environment) object.Object {
	go func() {
		Eval(node.Call, env)
	}()
	return NULL
}

func evalTryExpression(te *ast.TryExpression, env *object.Environment) object.Object {
	res := Eval(te.Block, env)
	if isError(res) {
		errObj := res.(*object.Error)
		childEnv := object.NewEnclosedEnvironment(env)
		if te.CatchParameter != nil {
			childEnv.Set(te.CatchParameter.Value, &object.String{Value: errObj.Message})
		}
		return Eval(te.CatchBlock, childEnv)
	}
	return res
}

func objectsEqual(left, right object.Object) bool {
	if left.Type() != right.Type() {
		return false
	}
	return left.Inspect() == right.Inspect()
}

func evalPrefixExpression(operator string, right object.Object) object.Object {
	switch operator {
	case "!":
		return evalBangOperatorExpression(right)
	case "-":
		return evalMinusPrefixOperatorExpression(right)
	default:
		return &object.Error{Message: fmt.Sprintf("unknown operator: %s%s", operator, right.Type())}
	}
}

func evalBangOperatorExpression(right object.Object) object.Object {
	if isTruthy(right) {
		return FALSE
	}
	return TRUE
}

func evalMinusPrefixOperatorExpression(right object.Object) object.Object {
	if right.Type() == object.INTEGER_OBJ {
		value := right.(*object.Integer).Value
		return &object.Integer{Value: -value}
	}
	if right.Type() == object.FLOAT_OBJ {
		value := right.(*object.Float).Value
		return &object.Float{Value: -value}
	}
	return &object.Error{Message: fmt.Sprintf("unknown operator: -%s", right.Type())}
}

func evalPipeExpression(node *ast.PipeExpression, env *object.Environment) object.Object {
	left := Eval(node.Left, env)
	if isError(left) {
		return left
	}

	switch right := node.Right.(type) {
	case *ast.CallExpression:
		function := Eval(right.Function, env)
		if isError(function) {
			return function
		}
		args := evalExpressions(right.Arguments, env)
		if len(args) == 1 && isError(args[0]) {
			return args[0]
		}
		// Prepend left evaluated value to args
		newArgs := append([]object.Object{left}, args...)
		return applyFunction(function, newArgs)

	case *ast.Identifier:
		function := Eval(right, env)
		if isError(function) {
			return function
		}
		return applyFunction(function, []object.Object{left})

	default:
		return &object.Error{Message: fmt.Sprintf("pipeline operator right side must be a function call or identifier, got %T", node.Right)}
	}
}

func objToRaw(obj object.Object) interface{} {
	switch o := obj.(type) {
	case *object.Integer:
		return o.Value
	case *object.Float:
		return o.Value
	case *object.Boolean:
		return o.Value
	case *object.String:
		return o.Value
	case *object.Array:
		res := make([]interface{}, len(o.Elements))
		for i, el := range o.Elements {
			res[i] = objToRaw(el)
		}
		return res
	case *object.Hash:
		res := make(map[string]interface{})
		for _, pair := range o.Pairs {
			// Removing quotes from Inspect() for keys
			keyStr := pair.Key.Inspect()
			if strings.HasPrefix(keyStr, "\"") && strings.HasSuffix(keyStr, "\"") {
				keyStr = keyStr[1 : len(keyStr)-1]
			}
			res[keyStr] = objToRaw(pair.Value)
		}
		return res
	default:
		return nil
	}
}

func rawToObj(raw interface{}) object.Object {
	switch v := raw.(type) {
	case float64:
		// JSON unmarshals all numbers as float64
		// We could try to cast back to int if it's whole, but float is safer
		if v == float64(int64(v)) {
			return &object.Integer{Value: int64(v)}
		}
		return &object.Float{Value: v}
	case bool:
		if v {
			return TRUE
		}
		return FALSE
	case string:
		return &object.String{Value: v}
	case []interface{}:
		elements := make([]object.Object, len(v))
		for i, el := range v {
			elements[i] = rawToObj(el)
		}
		return &object.Array{Elements: elements}
	case map[string]interface{}:
		pairs := make(map[object.HashKey]object.HashPair)
		for k, val := range v {
			key := &object.String{Value: k}
			hashKey := key.HashKey()
			pairs[hashKey] = object.HashPair{Key: key, Value: rawToObj(val)}
		}
		return &object.Hash{Pairs: pairs}
	default:
		return NULL
	}
}
