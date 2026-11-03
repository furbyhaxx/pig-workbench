package hashline

import (
	"fmt"
	"sort"
)

const (
	lfMask   = 1
	crlfMask = 2
)

type endingPositions struct {
	lf, crlf []int
}

func endingMask(ending, dominant string) int {
	if ending == "" {
		ending = dominant
	}
	if ending == "\r\n" {
		return crlfMask
	}
	return lfMask
}

func assignEnding(line sourceLine, mask int, endings []string) error {
	if line.ending == "" {
		return nil
	}
	switch mask {
	case lfMask:
		endings[line.outputLine-1] = "\n"
	case crlfMask:
		endings[line.outputLine-1] = "\r\n"
	case lfMask | crlfMask:
		return &AmbiguousEndingError{Line: line.outputLine, Candidates: []string{"\n", "\r\n"}}
	default:
		return fmt.Errorf("cannot determine the ending of line %d", line.outputLine)
	}
	return nil
}

func positionsWithin(positions []int, low, high int) bool {
	i := sort.SearchInts(positions, low)
	return i < len(positions) && positions[i] <= high
}

func (p *endingPositions) mask(low, high int) int {
	if p == nil || low > high {
		return 0
	}
	mask := 0
	if positionsWithin(p.lf, low, high) {
		mask |= lfMask
	}
	if positionsWithin(p.crlf, low, high) {
		mask |= crlfMask
	}
	return mask
}

func subsequence(shorter, longer []sourceLine) bool {
	i := 0
	for _, line := range longer {
		if i < len(shorter) && shorter[i].text == line.text {
			i++
		}
	}
	return i == len(shorter)
}

func alignEndings(original, edited []sourceLine, dominant string, endings []string) error {
	mixed := false
	for _, line := range original {
		if line.ending != "" && line.ending != dominant {
			mixed = true
			break
		}
	}
	if !mixed {
		return nil
	}

	originalText := make(map[string]bool, len(original))
	editedText := make(map[string]bool, len(edited))
	for _, line := range original {
		originalText[line.text] = true
	}
	for _, line := range edited {
		editedText[line.text] = true
	}
	var commonOriginal, commonEdited []sourceLine
	positions := make(map[string]*endingPositions)
	for _, line := range original {
		if !editedText[line.text] {
			continue
		}
		i := len(commonOriginal)
		commonOriginal = append(commonOriginal, line)
		p := positions[line.text]
		if p == nil {
			p = &endingPositions{}
			positions[line.text] = p
		}
		if endingMask(line.ending, dominant) == lfMask {
			p.lf = append(p.lf, i)
		} else {
			p.crlf = append(p.crlf, i)
		}
	}
	for _, line := range edited {
		if originalText[line.text] {
			commonEdited = append(commonEdited, line)
		}
	}
	if subsequence(commonOriginal, commonEdited) {
		return alignInsertions(commonOriginal, commonEdited, dominant, endings, positions)
	}
	if subsequence(commonEdited, commonOriginal) {
		return alignDeletions(commonOriginal, commonEdited, endings, positions)
	}
	return alignLCS(commonOriginal, commonEdited, dominant, endings, positions)
}

func alignInsertions(original, edited []sourceLine, dominant string, endings []string, positions map[string]*endingPositions) error {
	prefix := make([]int, len(edited)+1)
	for j, line := range edited {
		prefix[j+1] = prefix[j]
		if prefix[j] < len(original) && original[prefix[j]].text == line.text {
			prefix[j+1]++
		}
	}
	suffix := make([]int, len(edited)+1)
	suffix[len(edited)] = len(original)
	for j := len(edited) - 1; j >= 0; j-- {
		suffix[j] = suffix[j+1]
		if suffix[j] > 0 && original[suffix[j]-1].text == edited[j].text {
			suffix[j]--
		}
	}
	for j, line := range edited {
		mask := positions[line.text].mask(max(0, suffix[j+1]-1), min(len(original)-1, prefix[j]))
		if prefix[j] >= suffix[j+1] {
			mask |= endingMask("", dominant)
		}
		if err := assignEnding(line, mask, endings); err != nil {
			return err
		}
	}
	return nil
}

func alignDeletions(original, edited []sourceLine, endings []string, positions map[string]*endingPositions) error {
	low := make([]int, len(edited))
	high := make([]int, len(edited))
	for i, j := 0, 0; j < len(edited); i++ {
		if original[i].text == edited[j].text {
			low[j] = i
			j++
		}
	}
	for i, j := len(original)-1, len(edited)-1; j >= 0; i-- {
		if original[i].text == edited[j].text {
			high[j] = i
			j--
		}
	}
	for j, line := range edited {
		if err := assignEnding(line, positions[line.text].mask(low[j], high[j]), endings); err != nil {
			return err
		}
	}
	return nil
}

func forwardLCS(state []int, original, edited []sourceLine) {
	for _, line := range edited {
		diagonal := state[0]
		for i, old := range original {
			previous := state[i+1]
			if old.text == line.text {
				state[i+1] = diagonal + 1
			} else {
				state[i+1] = max(state[i], state[i+1])
			}
			diagonal = previous
		}
	}
}

func backwardLCS(state []int, original, edited []sourceLine) {
	for j := len(edited) - 1; j >= 0; j-- {
		diagonal := state[len(original)]
		for i := len(original) - 1; i >= 0; i-- {
			previous := state[i]
			if original[i].text == edited[j].text {
				state[i] = diagonal + 1
			} else {
				state[i] = max(state[i], state[i+1])
			}
			diagonal = previous
		}
	}
}

func alignLCS(original, edited []sourceLine, dominant string, endings []string, positions map[string]*endingPositions) error {
	targets := make([]int, len(edited)+1)
	for j, line := range edited {
		targets[j+1] = targets[j]
		p := positions[line.text]
		if line.ending != "" && p.mask(0, len(original)-1)&^endingMask("", dominant) != 0 {
			targets[j+1]++
		}
	}
	if targets[len(edited)] == 0 {
		return nil
	}
	whole := make([]int, len(original)+1)
	forwardLCS(whole, original, edited)
	optimal := whole[len(original)]
	var visit func(int, int, []int, []int) error
	visit = func(start, end int, prefix, suffix []int) error {
		if targets[start] == targets[end] {
			return nil
		}
		if end-start == 1 {
			line := edited[start]
			mask := 0
			for i := 0; i <= len(original); i++ {
				if prefix[i]+suffix[i] == optimal {
					mask |= endingMask("", dominant)
				}
				if i < len(original) && original[i].text == line.text && prefix[i]+suffix[i+1]+1 == optimal {
					mask |= endingMask(original[i].ending, dominant)
				}
			}
			return assignEnding(line, mask, endings)
		}
		mid := start + (end-start)/2
		if targets[start] != targets[mid] {
			leftSuffix := append([]int(nil), suffix...)
			backwardLCS(leftSuffix, original, edited[mid:end])
			if err := visit(start, mid, prefix, leftSuffix); err != nil {
				return err
			}
		}
		if targets[mid] != targets[end] {
			rightPrefix := append([]int(nil), prefix...)
			forwardLCS(rightPrefix, original, edited[start:mid])
			if err := visit(mid, end, rightPrefix, suffix); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(0, len(edited), make([]int, len(original)+1), make([]int, len(original)+1))
}
