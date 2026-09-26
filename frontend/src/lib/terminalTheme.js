// xterm.js needs literal colour strings, so resolve the --terminal-* / --tok-*
// tokens (styles/tokens.css) at terminal creation time.
const cssVar = (name) =>
    getComputedStyle(document.documentElement).getPropertyValue(name).trim();

export function buildTerminalTheme() {
    const bg = cssVar("--terminal-bg");
    const plain = cssVar("--tok-plain");
    const green = cssVar("--tok-key");
    const blue = cssVar("--tok-str");
    const yellow = cssVar("--tok-num");
    const magenta = cssVar("--tok-kw");
    const cyan = cssVar("--tok-punct");
    const red = cssVar("--terminal-dot-close");
    return {
        background: bg,
        foreground: plain,
        cursor: cssVar("--terminal-caret"),
        cursorAccent: bg,
        selectionBackground: cssVar("--terminal-selection"),
        black: bg,
        red,
        green,
        yellow,
        blue,
        magenta,
        cyan,
        white: plain,
        brightBlack: cssVar("--terminal-text-dim"),
        brightRed: red,
        brightGreen: green,
        brightYellow: yellow,
        brightBlue: blue,
        brightMagenta: magenta,
        brightCyan: cyan,
        brightWhite: plain,
    };
}
