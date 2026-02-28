package builtins

import (
	"bufio"
	"embed"
	"encoding/json"
	"xon/object"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	VMConstants []object.Object
	VMGlobals   []object.Object
	VMGlobalsMu *sync.RWMutex
)

var RunClosureCallback func(cl *object.Closure, args []object.Object) object.Object

func SetVMContext(constants []object.Object, globals []object.Object, mu *sync.RWMutex) {
	VMConstants = constants
	VMGlobals = globals
	VMGlobalsMu = mu
}

//go:embed all:std
var embeddedStd embed.FS

const StdBltinsFallback = `
set help = fn() {
    out "Xon Language - Core Primitives Only";
    out "Standard library (std/core.xn) not found.";
};
`

// LoadStdLib loads the standard library source code.
func LoadStdLib() (string, error) {
	stdPath := "builtins/std/core.xn"
	content, err := ioutil.ReadFile(stdPath)
	if err == nil {
		return string(content), nil
	}
	// Fallback to embedded
	embeddedContent, err := embeddedStd.ReadFile(stdPath)
	if err == nil {
		return string(embeddedContent), nil
	}
	return StdBltinsFallback, nil
}

var (
	NULL         = &object.Null{}
	TRUE         = &object.Boolean{Value: true}
	FALSE        = &object.Boolean{Value: false}
	stdinScanner = bufio.NewScanner(os.Stdin)
)

func isTruthyBuiltin(obj object.Object) bool {
	if obj == NULL {
		return false
	}
	if b, ok := obj.(*object.Boolean); ok {
		return b.Value
	}
	return true
}

func boolToObj(b bool) object.Object {
	if b {
		return TRUE
	}
	return FALSE
}

var builtinsMap = map[string]*object.Builtin{
	"type": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return &object.String{Value: string(args[0].Type())}
		},
	},
	"math_sqrt": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			val := float64(0)
			switch arg := args[0].(type) {
			case *object.Integer:
				val = float64(arg.Value)
			case *object.Float:
				val = arg.Value
			default:
				return &object.Error{Message: "argument to sqrt must be NUMBER"}
			}
			return &object.Float{Value: math.Sqrt(val)}
		},
	},
	"math_pow": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			base := float64(0)
			exp := float64(0)
			if b, ok := args[0].(*object.Integer); ok {
				base = float64(b.Value)
			} else if b, ok := args[0].(*object.Float); ok {
				base = b.Value
			}
			if e, ok := args[1].(*object.Integer); ok {
				exp = float64(e.Value)
			} else if e, ok := args[1].(*object.Float); ok {
				exp = e.Value
			}
			return &object.Float{Value: math.Pow(base, exp)}
		},
	},
	"str_split": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			s, ok1 := args[0].(*object.String)
			sep, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return &object.Error{Message: "arguments to split must be STRING"}
			}
			parts := strings.Split(s.Value, sep.Value)
			elements := make([]object.Object, len(parts))
			for i, p := range parts {
				elements[i] = &object.String{Value: p}
			}
			return &object.Array{Elements: elements}
		},
	},
	"str_contains": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			s, ok1 := args[0].(*object.String)
			sub, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return &object.Error{Message: "arguments to contains must be STRING"}
			}
			if strings.Contains(s.Value, sub.Value) {
				return TRUE
			}
			return FALSE
		},
	},
	"http_serve": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=2"}
			}
			port, ok1 := args[0].(*object.Integer)
			handler, ok2 := args[1].(*object.Closure)
			if !ok1 || !ok2 {
				return &object.Error{Message: "arguments to http_serve must be (INTEGER, FUNCTION)"}
			}

			addr := ":" + fmt.Sprint(port.Value)
			fmt.Printf("Xon Server starting on %s...\n", addr)

			server := &http.Server{Addr: addr}
			http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				// Create sub-VM for request
				// We need a dummy VM or just a way to run the closure.
				// Since we don't have a direct "RunClosure" helper, we'll implement a tiny one or use VM logic.

				// Prepare request object
				reqHash := make(map[object.HashKey]object.HashPair)
				reqHash[(&object.String{Value: "method"}).HashKey()] = object.HashPair{Key: &object.String{Value: "method"}, Value: &object.String{Value: r.Method}}
				reqHash[(&object.String{Value: "path"}).HashKey()] = object.HashPair{Key: &object.String{Value: "path"}, Value: &object.String{Value: r.URL.Path}}

				// For simplicity, we just pass method and path for now.
				// In a full implementation, we'd add headers, body, etc.

				// Need a way to run this. We actually need a circular dependency or a helper.
				// Let's assume we have a way to run a closure.

				// Since we can't easily import 'vm' here without circular deps,
				// we'll use a hack or a callback.
				if RunClosureCallback == nil {
					http.Error(w, "Server engine not initialized", 500)
					return
				}

				res := RunClosureCallback(handler, []object.Object{&object.Hash{Pairs: reqHash}})
				if res.Type() == object.ERROR_OBJ {
					http.Error(w, res.Inspect(), 500)
					return
				}

				fmt.Fprintf(w, "%s", res.Inspect())
			})

			go server.ListenAndServe()
			return &object.String{Value: "Server running on " + addr}
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
	"os_compile": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return &object.Error{Message: "wrong number of arguments. got=" + fmt.Sprint(len(args)) + ", want=2"}
			}
			scriptPath, ok1 := args[0].(*object.String)
			outputExe, ok2 := args[1].(*object.String)
			if !ok1 || !ok2 {
				return &object.Error{Message: "arguments to compile must be STRING"}
			}

			scriptContent, err := ioutil.ReadFile(scriptPath.Value)
			if err != nil {
				return &object.Error{Message: "failed to read script: " + err.Error()}
			}

			escaped := strings.ReplaceAll(string(scriptContent), "'", "''")
			ldflags := fmt.Sprintf("-X 'main.EmbeddedScript=%s'", escaped)

			cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", outputExe.Value, "main.go")
			out, err := cmd.CombinedOutput()
			if err != nil {
				return &object.Error{Message: "build failed: " + string(out) + " " + err.Error()}
			}

			return &object.String{Value: "Successfully built " + outputExe.Value}
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
			case *object.Float:
				return &object.Integer{Value: int64(arg.Value)}
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
	"float": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			switch arg := args[0].(type) {
			case *object.Integer:
				return &object.Float{Value: float64(arg.Value)}
			case *object.Float:
				return arg
			case *object.String:
				cleanVal := strings.TrimSpace(arg.Value)
				val, err := strconv.ParseFloat(cleanVal, 64)
				if err != nil {
					return &object.Error{Message: fmt.Sprintf("could not parse string '%s' as float: %v", cleanVal, err)}
				}
				return &object.Float{Value: val}
			default:
				return &object.Error{Message: "cannot convert to float"}
			}
		},
	},
	"bool": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: "wrong number of arguments"}
			}
			return boolToObj(isTruthyBuiltin(args[0]))
		},
	},
	"typeof": &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return &object.Error{Message: fmt.Sprintf("wrong number of arguments. got=%d, want=1", len(args))}
			}
			return &object.String{Value: string(args[0].Type())}
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

func objToRaw(obj object.Object) interface{} {
	switch obj := obj.(type) {
	case *object.Integer:
		return obj.Value
	case *object.Float:
		return obj.Value
	case *object.String:
		return obj.Value
	case *object.Boolean:
		return obj.Value
	case *object.Array:
		res := make([]interface{}, len(obj.Elements))
		for i, el := range obj.Elements {
			res[i] = objToRaw(el)
		}
		return res
	case *object.Hash:
		res := make(map[string]interface{})
		for _, pair := range obj.Pairs {
			res[pair.Key.Inspect()] = objToRaw(pair.Value)
		}
		return res
	default:
		return nil
	}
}

func rawToObj(val interface{}) object.Object {
	switch val := val.(type) {
	case float64:
		if val == float64(int64(val)) {
			return &object.Integer{Value: int64(val)}
		}
		return &object.Float{Value: val}
	case string:
		return &object.String{Value: val}
	case bool:
		return &object.Boolean{Value: val}
	case []interface{}:
		elements := make([]object.Object, len(val))
		for i, el := range val {
			elements[i] = rawToObj(el)
		}
		return &object.Array{Elements: elements}
	case map[string]interface{}:
		pairs := make(map[object.HashKey]object.HashPair)
		for k, v := range val {
			key := &object.String{Value: k}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: rawToObj(v)}
		}
		return &object.Hash{Pairs: pairs}
	default:
		return NULL
	}
}
