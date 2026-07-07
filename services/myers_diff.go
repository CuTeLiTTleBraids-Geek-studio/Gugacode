package services

import (
	"bytes"
	"fmt"
	"strings"
)

// myersDiff computes a unified diff between oldText and newText using the
// Myers diff algorithm (Plan 60 / N-27). This produces much cleaner diffs
// than the previous naive line-by-line comparison, which treated every
// changed line as a delete+insert pair even when a small edit was made
// within a line.
//
// The output follows the standard unified diff format:
//   diff --git a/<path> b/<path>
//   --- a/<path>
//   +++ b/<path>
//    context line
//   -removed line
//   +added line
//
// The algorithm is the classic O(ND) Myers diff with backtracking to
// recover the edit script. See: Eugene W. Myers, "An O(ND) Difference
// Algorithm and Its Variations" (1986).
func myersDiff(filePath string, oldText string, newText string) string {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	diff := computeDiff(oldLines, newLines)

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", filePath, filePath))
	buf.WriteString(fmt.Sprintf("--- a/%s\n", filePath))
	buf.WriteString(fmt.Sprintf("+++ b/%s\n", filePath))

	// Generate hunks with 3 lines of context.
	writeHunks(&buf, diff, oldLines, newLines, 3)

	return buf.String()
}

// diffOp represents a single edit operation in the diff script.
type diffOp struct {
	kind   diffKind
	oldIdx int // index into oldLines (for equal and delete)
	newIdx int // index into newLines (for equal and insert)
}

type diffKind int

const (
	diffEqual  diffKind = iota
	diffDelete          // line present in old, not in new
	diffInsert          // line present in new, not in old
)

// computeDiff runs the Myers algorithm and returns the edit script as a
// sequence of equal/delete/insert operations.
func computeDiff(oldLines, newLines []string) []diffOp {
	n := len(oldLines)
	m := len(newLines)
	max := n + m

	if max == 0 {
		return nil
	}

	// trace holds the V arrays for each round. V[k] stores the furthest
	// x-value reached on diagonal k. We use the standard Myers
	// formulation with 1-based indexing offset (V[1] = 0 for the
	// initial state).
	traces := make([][]int, 0, max+1)
	v := make([]int, 2*max+1)
	v[1+max] = 0 // v[1] = 0 with offset

	var trace []int
	for d := 0; d <= max; d++ {
		trace = make([]int, len(v))
		copy(trace, v)
		traces = append(traces, trace)

		found := false
		for k := -d; k <= d; k += 2 {
			var x int
			idx := k + max // offset index
			if k == -d || (k != d && v[idx-1] < v[idx+1]) {
				x = v[idx+1] // move down (insert)
			} else {
				x = v[idx-1] + 1 // move right (delete)
			}
			y := x - k
			// Follow diagonal (equal lines).
			for x < n && y < m && oldLines[x] == newLines[y] {
				x++
				y++
			}
			v[idx] = x
			if x >= n && y >= m {
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	// Backtrack through the traces to build the edit script.
	return backtrack(traces, oldLines, newLines, max)
}

// backtrack recovers the edit script by walking backwards through the
// trace history from the end point (n, m) to the start (0, 0).
func backtrack(traces [][]int, oldLines, newLines []string, max int) []diffOp {
	n := len(oldLines)
	m := len(newLines)

	var ops []diffOp
	x, y := n, m

	for d := len(traces) - 1; d > 0; d-- {
		trace := traces[d]
		k := x - y
		idx := k + max

		var prevK int
		if k == -d || (k != d && trace[idx-1] < trace[idx+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := trace[prevK+max]
		prevY := prevX - prevK

		// Follow diagonal (equal lines) back from (x, y) to (prevX, prevY).
		for x > prevX && y > prevY {
			ops = append(ops, diffOp{kind: diffEqual, oldIdx: x - 1, newIdx: y - 1})
			x--
			y--
		}

		if d > 0 {
			if x == prevX {
				// Insert: line came from new.
				ops = append(ops, diffOp{kind: diffInsert, oldIdx: -1, newIdx: y - 1})
			} else {
				// Delete: line was in old.
				ops = append(ops, diffOp{kind: diffDelete, oldIdx: x - 1, newIdx: -1})
			}
		}

		x = prevX
		y = prevY
	}

	// Follow the initial diagonal from (x, y) back to (0, 0). This
	// captures the equal lines at the beginning of the diff that were
	// matched during the d=0 forward pass. Without this, leading context
	// lines are lost (the main loop only processes d >= 1 edits).
	for x > 0 && y > 0 {
		ops = append(ops, diffOp{kind: diffEqual, oldIdx: x - 1, newIdx: y - 1})
		x--
		y--
	}

	// The ops are in reverse order (end to start). Reverse them.
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}

	return ops
}

// writeHunks groups the edit operations into unified-diff hunks with the
// given number of context lines, and writes them to buf.
func writeHunks(buf *bytes.Buffer, ops []diffOp, oldLines, newLines []string, context int) {
	if len(ops) == 0 {
		return
	}

	i := 0
	for i < len(ops) {
		// Find the next change (non-equal op).
		for i < len(ops) && ops[i].kind == diffEqual {
			i++
		}
		if i >= len(ops) {
			break
		}

		// Start of a hunk: go back `context` lines.
		hunkStart := i - context
		if hunkStart < 0 {
			hunkStart = 0
		}

		// Find the end of the hunk: scan forward until we've seen `context`
		// consecutive equal lines after the last change.
		j := i
		lastChange := i
		for j < len(ops) {
			if ops[j].kind != diffEqual {
				lastChange = j
				j++
			} else {
				// Count consecutive equal lines.
				eqStart := j
				for j < len(ops) && ops[j].kind == diffEqual {
					j++
				}
				eqCount := j - eqStart
				if eqCount >= context*2 || j >= len(ops) {
					// Enough context to split, or end of ops.
					break
				}
			}
		}

		// End of hunk: go forward `context` lines after the last change.
		hunkEnd := lastChange + context + 1
		if hunkEnd > len(ops) {
			hunkEnd = len(ops)
		}

		// Compute the hunk header (old start, old count, new start, new count).
		oldStart, oldCount, newStart, newCount := computeHunkBounds(ops, hunkStart, hunkEnd, oldLines, newLines)
		buf.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))

		// Write the hunk lines.
		for k := hunkStart; k < hunkEnd; k++ {
			op := ops[k]
			switch op.kind {
			case diffEqual:
				buf.WriteString(" " + lineAt(oldLines, newLines, op) + "\n")
			case diffDelete:
				buf.WriteString("-" + oldLines[op.oldIdx] + "\n")
			case diffInsert:
				buf.WriteString("+" + newLines[op.newIdx] + "\n")
			}
		}

		i = hunkEnd
	}
}

// computeHunkBounds calculates the (oldStart, oldCount, newStart, newCount)
// for a hunk header, where starts are 1-based.
func computeHunkBounds(ops []diffOp, hunkStart, hunkEnd int, oldLines, newLines []string) (int, int, int, int) {
	oldStart, newStart := 0, 0
	if hunkStart < len(ops) {
		op := ops[hunkStart]
		if op.kind == diffEqual {
			oldStart = op.oldIdx
			newStart = op.newIdx
		} else if op.kind == diffDelete {
			oldStart = op.oldIdx
			// For delete at hunkStart, newStart is the newIdx of the
			// next equal or insert op, or 0 if none.
			newStart = findNewStart(ops, hunkStart)
		} else {
			newStart = op.newIdx
			oldStart = findOldStart(ops, hunkStart)
		}
	}
	if oldStart < 0 {
		oldStart = 0
	}
	if newStart < 0 {
		newStart = 0
	}

	oldCount, newCount := 0, 0
	for k := hunkStart; k < hunkEnd; k++ {
		op := ops[k]
		switch op.kind {
		case diffEqual:
			oldCount++
			newCount++
		case diffDelete:
			oldCount++
		case diffInsert:
			newCount++
		}
	}

	// 1-based starts.
	return oldStart + 1, oldCount, newStart + 1, newCount
}

func findNewStart(ops []diffOp, from int) int {
	for i := from; i < len(ops); i++ {
		if ops[i].kind == diffEqual || ops[i].kind == diffInsert {
			return ops[i].newIdx
		}
	}
	return 0
}

func findOldStart(ops []diffOp, from int) int {
	for i := from; i < len(ops); i++ {
		if ops[i].kind == diffEqual || ops[i].kind == diffDelete {
			return ops[i].oldIdx
		}
	}
	return 0
}

// lineAt returns the line content for an equal op (same in old and new).
func lineAt(oldLines, newLines []string, op diffOp) string {
	if op.oldIdx >= 0 && op.oldIdx < len(oldLines) {
		return oldLines[op.oldIdx]
	}
	if op.newIdx >= 0 && op.newIdx < len(newLines) {
		return newLines[op.newIdx]
	}
	return ""
}

// splitLines splits text into lines, preserving the content without the
// trailing newline (the diff writer re-adds "\n" for each line).
func splitLines(text string) []string {
	if text == "" {
		return []string{}
	}
	lines := strings.Split(text, "\n")
	// Remove the trailing empty string if the text ends with "\n".
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
