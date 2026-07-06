package cmd

import "strings"

// diffLines returns an LCS-based line diff of a -> b. Unchanged lines are
// prefixed with two spaces, removed lines with "- ", added lines with "+ ".
func diffLines(a, b []string) []string {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, "  "+a[i])
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, "- "+a[i])
			i++
		default:
			out = append(out, "+ "+b[j])
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, "- "+a[i])
	}
	for ; j < m; j++ {
		out = append(out, "+ "+b[j])
	}
	return out
}

// renderDiff compares two texts line by line and returns whether they differ
// and a compact diff showing only the changed (+/-) lines.
func renderDiff(oldText, newText string) (changed bool, diff string) {
	oldLines := splitLinesNormalized(oldText)
	newLines := splitLinesNormalized(newText)
	var b strings.Builder
	for _, line := range diffLines(oldLines, newLines) {
		if strings.HasPrefix(line, "+ ") || strings.HasPrefix(line, "- ") {
			changed = true
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return changed, b.String()
}

func splitLinesNormalized(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
