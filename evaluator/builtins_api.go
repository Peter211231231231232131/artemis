package evaluator

import "artemis/object"

// BuiltinNames returns all builtin function names in a stable order.
// This order is critical for the compiler to match names to indices.
var BuiltinNames = []string{
	"type", "len", "push", "first", "last", "pop",
	"readFile", "writeFile",
	"toUpperCase", "toLowerCase",
	"now", "sleep",
	"json_encode", "json_decode",
	"fs_remove", "fs_exists",
	"os_mouse_move", "os_mouse_click", "os_key_tap", "os_exec",
	"os_mouse_get_pos", "os_alert", "os_compile", "os_keyboard_type",
	"math_random", "http_get",
	"input", "int", "str",
	"copy", "paste",
}

// GetBuiltinByName returns a builtin function by name.
func GetBuiltinByName(name string) *object.Builtin {
	b, ok := builtins[name]
	if !ok {
		return nil
	}
	return b
}

// GetBuiltinByIndex returns a builtin by its index in BuiltinNames.
func GetBuiltinByIndex(index int) *object.Builtin {
	if index < 0 || index >= len(BuiltinNames) {
		return nil
	}
	return GetBuiltinByName(BuiltinNames[index])
}
