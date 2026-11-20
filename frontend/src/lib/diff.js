// Small line-based diff for the "review changes before apply" step. No dependency: manifests are
// a few hundred lines, so a classic LCS over the lines between the common prefix and suffix is
// plenty fast, and anything pathologically large degrades to "replace the middle" instead of
// freezing the tab.

const MAX_CELLS = 4_000_000;

/** Splits text into lines without a phantom trailing empty line. */
function toLines(text) {
    if (!text) return [];
    const lines = String(text).replace(/\r\n/g, "\n").split("\n");
    if (lines[lines.length - 1] === "") lines.pop();
    return lines;
}

/**
 * Returns an array of { type: "same" | "add" | "del", text } in display order for the change
 * from `before` to `after`.
 */
export function diffLines(before, after) {
    const a = toLines(before);
    const b = toLines(after);

    let start = 0;
    while (start < a.length && start < b.length && a[start] === b[start]) start++;
    let endA = a.length;
    let endB = b.length;
    while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
        endA--;
        endB--;
    }

    const out = [];
    for (let i = 0; i < start; i++) out.push({ type: "same", text: a[i] });

    const midA = a.slice(start, endA);
    const midB = b.slice(start, endB);
    if (midA.length * midB.length > MAX_CELLS) {
        midA.forEach((text) => out.push({ type: "del", text }));
        midB.forEach((text) => out.push({ type: "add", text }));
    } else {
        // LCS table over the differing middle
        const n = midA.length;
        const m = midB.length;
        const lcs = Array.from({ length: n + 1 }, () => new Uint32Array(m + 1));
        for (let i = n - 1; i >= 0; i--) {
            for (let j = m - 1; j >= 0; j--) {
                lcs[i][j] = midA[i] === midB[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
            }
        }
        let i = 0;
        let j = 0;
        while (i < n && j < m) {
            if (midA[i] === midB[j]) {
                out.push({ type: "same", text: midA[i] });
                i++;
                j++;
            } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
                out.push({ type: "del", text: midA[i++] });
            } else {
                out.push({ type: "add", text: midB[j++] });
            }
        }
        while (i < n) out.push({ type: "del", text: midA[i++] });
        while (j < m) out.push({ type: "add", text: midB[j++] });
    }

    for (let i = endA; i < a.length; i++) out.push({ type: "same", text: a[i] });
    return out;
}

export function diffStats(lines) {
    let added = 0;
    let removed = 0;
    for (const l of lines) {
        if (l.type === "add") added++;
        else if (l.type === "del") removed++;
    }
    return { added, removed, changed: added + removed > 0 };
}

/**
 * Collapses long unchanged runs, keeping `context` lines around each change. Returns display
 * rows: { type, text } or { type: "skip", count } for a collapsed run.
 */
export function withContext(lines, context = 3) {
    const keep = new Array(lines.length).fill(false);
    lines.forEach((l, idx) => {
        if (l.type === "same") return;
        for (let k = Math.max(0, idx - context); k <= Math.min(lines.length - 1, idx + context); k++) keep[k] = true;
    });
    // nothing changed: show the head of the object rather than nothing
    if (!keep.some(Boolean)) return lines.slice(0, Math.min(lines.length, context * 4));
    const rows = [];
    let skipped = 0;
    lines.forEach((l, idx) => {
        if (keep[idx]) {
            if (skipped) rows.push({ type: "skip", count: skipped });
            skipped = 0;
            rows.push(l);
        } else {
            skipped++;
        }
    });
    if (skipped) rows.push({ type: "skip", count: skipped });
    return rows;
}
