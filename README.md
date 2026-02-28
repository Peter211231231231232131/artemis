# 🧬 Exon Language

Exon is a high-performance, concurrent scripting language designed for **automation, systems programming, and rapid tool development**. 

Built with an optimized **Bytecode VM**, Exon features modern functional primitives, first-class closures, and native multi-threading.

## ✨ Features

- **🚀 High Performance**: Optimized Bytecode VM written in Go.
- **🧵 Concurrency**: Native `spawn` keyword for effortless multi-threading.
- **🔒 Stateful Closures**: Capture and mutate lexical variables with ease.
- **🧩 Functional Power**: Pipeline operators (`|>`), `map`, `filter`, and `reduce`.
- **🛠️ Automation**: Control mouse, keyboard, and screen natively.
- **📂 Modern Tooling**: Built-in disassembler (`-d`) and standalone compiler.
- **🎨 Editor Support**: Dedicated [VS Code Extension](../exon-vscode/) for syntax highlighting.

## 🚀 Quick Start

### Install (add to PATH)

**Windows (PowerShell):**
```powershell
.\install.ps1
```
Installs `xn.exe` to `%LOCALAPPDATA%\Exon` and adds it to your user PATH. Use a new terminal, then run `xn` or `xn script.xn`.

Custom install directory:
```powershell
.\install.ps1 -InstallDir "C:\Tools\Exon"
```

Uninstall:
```powershell
.\install.ps1 -Uninstall
```

### Build from source

1. **Build**:
   ```bash
   go build -o xn.exe main.go
   ```

2. **Run a script**:
   ```bash
   ./xn.exe script.xn
   ```

3. **Interactive Mode (REPL)**:
   ```bash
   ./xn.exe
   ```

## 📜 Example: Stateful Closures

```exon
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

```exon
set worker = fn(id) {
    out "Worker ${id} starting...";
    sleep(1000);
    out "Worker ${id} finished.";
};

spawn worker(1);
spawn worker(2);
out "Main thread continuing...";
```

## 🛠️ Built-in Modules

- `std`: Arrays, Functional primitives.
- `os`: Automation (Mouse, Keyboard, Alerts).
- `fs`: File System operations.
- `http`: Native Web requests.
- `json`: Seamless JSON encoding/decoding.

---
*Created with 🧬 Exon. Happy Scripting!*
