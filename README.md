# 🏹 Artemis Language

Artemis is a high-level, interpreted scripting language designed for **automation, visual sensing, and rapid system scripting**. 

## ✨ Features

- **🚀 Performance**: Fast tree-walking interpreter written in Go.
- **👁️ Visual Sensing**: Read screen pixels and colors natively (`os.pixel`).
- **🖱️ Automation**: Control mouse and keyboard with simple commands.
- **🌐 Networking**: Built-in HTTP client for web automation.
- **📂 Self-Hosting**: Standard library written in Artemis itself (`std/core.artms`).
- **🧩 Modern Syntax**: Pipeline operators (`>>`), pattern matching, and string interpolation.

## 🚀 Quick Start

1. **Build from source**:
   ```bash
   go build -o artemis.exe main.go
   ```

2. **Run a script**:
   ```bash
   ./artemis.exe my_script.artms
   ```

3. **Interactive Mode (REPL)**:
   ```bash
   ./artemis.exe
   ```

## 📜 Example Code

```artms
// Smart Automation Example
set target_color = "#FF0000";

if (os.pixel(100, 100) == target_color) {
    os.move_mouse(100, 100);
    os.click();
    out "Operation Successful!";
}
```

## 🛠️ Built-in Modules

- `std`: Arrays, Functional primitives (map, filter).
- `os`: Mouse, Keyboard, Alerts, Pixels.
- `fs`: File reading and writing.
- `http`: Web requests.
- `str`: String manipulation.
- `math`: Randomness and math constants.

## 📦 Project Structure

- `lexer/`, `parser/`, `ast/`: Core language implementation.
- `evaluator/`: Interpreter logic and system built-ins.
- `evaluator/std/`: The Artemis standard library (embedded in the binary).

---
*Created with Artemis. Happy Scripting!*
