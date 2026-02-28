# 🧬 Xon

Xon is a high-performance, concurrent scripting language designed for **automation, systems programming, and rapid tool development**. 

Built with an optimized **Bytecode VM**, Xon features modern functional primitives, first-class closures, and native multi-threading.

## ✨ Features

- **🚀 High Performance**: Optimized Bytecode VM written in Go.
- **🧵 Concurrency**: Native `spawn` keyword for effortless multi-threading.
- **🔒 Stateful Closures**: Capture and mutate lexical variables with ease.
- **🧩 Functional Power**: Pipeline operators (`|>`), `map`, `filter`, and `reduce`.
- **🛠️ Automation**: Control mouse, keyboard, and screen natively.
- **🖼️ GUI Maker**: Native Windows GUI with `gui.run()` — labels, buttons, inputs, callbacks.
- **📂 Modern Tooling**: Built-in disassembler (`-d`) and standalone compiler.
- **🎨 Editor Support**: Dedicated [VS Code Extension](../xon-vscode/) for syntax highlighting.

## 🚀 Quick Start

### Installing Xon (Windows)

Build and install (requires Go):

```powershell
.\install.ps1
```
Builds `xon.exe` and installs to `%LOCALAPPDATA%\Xon`. Custom directory: `.\install.ps1 -InstallDir "C:\Tools\Xon"`. Uninstall: `.\install.ps1 -Uninstall`.

### Build from source (manual)

1. **Build**:
   ```bash
   go build -o xon.exe .
   ```

2. **Run a script**:
   ```bash
   ./xon.exe script.xn
   ```

3. **Interactive Mode (REPL)**:
   ```bash
   ./xon.exe
   ```

## 📜 Example: Stateful Closures

```xon
set makeCounter = fn() {
    set c = 0;
    return fn() {
        c = c + 1;
        return c;
    };
};

set count = makeCounter();
out count(); // 1
out count(); // 2
```

## 📜 Example: Concurrency

```xon
set worker = fn(id) {
    out "Worker ${id} starting...";
    sleep(1000);
    out "Worker ${id} finished.";
};

spawn worker(1);
spawn worker(2);
out "Main thread continuing...";
```

## 📜 Example: GUI Maker

Native **Windows GUI** (labels, inputs, buttons, callbacks).

```xon
gui.runWindow("Hello from Xon!", 400, 200, [
    gui.label("Enter your name:"),
    gui.input("nameInput", ""),
    gui.button("Say Hello", fn() {
        set name = gui.get("nameInput");
        if (name == "") { name = "World"; }
        os.alert("Hello", "Hello, " + name + "!");
    })
]);
```

Widgets: `gui.label(text)`, `gui.button(text, onClick)`, `gui.input(id, default)`, `gui.textarea(id, default)`. Use `gui.get(id)` to read input values. Click **Quit** to close.

## 🛠️ Built-in Modules

- `std`: Arrays, Functional primitives.
- `os`: Automation (Mouse, Keyboard, Alerts).
- `gui`: GUI Maker — windows, labels, buttons, inputs (Windows only).
- `fs`: File System operations.
- `http`: Native Web requests.
- `json`: Seamless JSON encoding/decoding.

---
*Created with 🧬 Xon. Happy Scripting!*
